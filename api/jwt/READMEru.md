# jwt

[← go-core](../../README.md) · [English version](README.md)

Пары access/refresh-токенов на HS256 и middleware аутентификации для `net/http`
и gRPC. В токенах стандартные claims плюс id роли и тип токена. Тип проверяется
всегда: refresh-токен не пройдёт там, где ждут access, и наоборот.

```go
import "github.com/jwm1rr0rb10/go-core/api/jwt"
```

## Возможности

- Типизированные `Claims`: `sub`, `iss`, `aud`, `iat`, `nbf`, `exp`, `jti` + `role_id`, `typ`
- Проверка типа токена (`access` / `refresh`) при каждом разборе
- Принимается только HS256; `none`, HS512 и любые другие алгоритмы отклоняются
- Секрет короче 32 байт отклоняется ещё при создании хелпера
- Настройки: время жизни токенов, issuer, audience, leeway, часы, параметры cookie
- Ротация refresh-токенов с обнаружением повторного использования через
  подключаемый `RefreshStore` (in-memory реализация в комплекте)
- HTTP middleware: `Authorization: Bearer` и/или cookie, фильтр по ролям,
  прозрачное обновление по cookie (опционально), ошибки в JSON, свой обработчик ошибок
- gRPC: unary- и stream-перехватчики, карта «метод → роли», публичные методы,
  политика для неперечисленных методов, обновление через metadata (опционально)
- Claims кладутся в контекст под неэкспортируемым типизированным ключом,
  коллизий не бывает
- Нет зависимости от `grpc-ecosystem/go-grpc-middleware`

## Обзор API

| Идентификатор | Назначение |
|---|---|
| `NewHelper(secret, ...Option) (*Helper, error)` / `MustNewHelper` | Создать переиспользуемый потокобезопасный хелпер |
| `WithAccessTTL`, `WithRefreshTTL`, `WithIssuer`, `WithAudience`, `WithLeeway`, `WithClock`, `WithCookieConfig`, `WithRefreshStore` | Опции хелпера |
| `(*Helper).GeneratePair(userID, roleID)` | Выпустить access + refresh |
| `(*Helper).ParseAccess` / `ParseRefresh` / `Parse(tok, typ)` | Проверить токен ожидаемого типа |
| `(*Helper).Refresh(ctx, refresh)` | Обменять (и погасить) refresh-токен на новую пару |
| `(*Helper).Cookies` / `SetCookies` / `ClearCookies` | HttpOnly-cookie для пары, выход из системы |
| `(*Helper).HTTPMiddleware(...HTTPOption)` | `func(http.Handler) http.Handler` |
| `WithRoles`, `WithTokenSources`, `WithCookieRefresh`, `WithErrorHandler` | Опции HTTP |
| `NewAuthInterceptor(h, ...GRPCOption)` | Аутентификатор для gRPC |
| `UnaryServerInterceptor()` / `StreamServerInterceptor()` / `Authorize(ctx, method)` | Интеграция с gRPC |
| `WithMethodRoles`, `WithPublicMethods`, `WithUnlistedPolicy`, `WithMetadataRefresh` | Опции gRPC |
| `ClaimsFromContext`, `GetUserID`, `GetRoleID`, `ContextWithClaims` | Доступ к контексту |
| `RefreshStore`, `NewMemoryRefreshStore` | Ротация refresh-токенов |
| `ErrBadToken`, `ErrTokenExpired`, `ErrWrongTokenType`, `ErrTokenRevoked`, `ErrNoToken`, `ErrForbidden`, `ErrWeakSecret`, `ErrNoContext` | Ошибки для `errors.Is` |

## Использование

### Выпуск и проверка

```go
h, err := jwt.NewHelper(secret, // не меньше 32 случайных байт
	jwt.WithIssuer("auth-service"),
	jwt.WithAudience("api"),
	jwt.WithAccessTTL(10*time.Minute),
	jwt.WithRefreshTTL(7*24*time.Hour),
	jwt.WithLeeway(30*time.Second),
	jwt.WithRefreshStore(jwt.NewMemoryRefreshStore()),
)
if err != nil {
	log.Fatal(err)
}

pair, err := h.GeneratePair("user-42", roleAdmin)
claims, err := h.ParseAccess(pair.AccessToken)   // ок
_, err = h.ParseAccess(pair.RefreshToken)         // errors.Is(err, jwt.ErrWrongTokenType)
next, err := h.Refresh(ctx, pair.RefreshToken)    // новая пара
_, err = h.Refresh(ctx, pair.RefreshToken)        // errors.Is(err, jwt.ErrTokenRevoked)
```

### HTTP

```go
auth := h.HTTPMiddleware()                                      // любой аутентифицированный
admin := h.HTTPMiddleware(jwt.WithRoles(roleAdmin))             // только админы
web := h.HTTPMiddleware(jwt.WithCookieRefresh(),                // браузер: cookie + тихое обновление
	jwt.WithTokenSources(jwt.SourceCookie))

mux.Handle("/api/", auth(apiHandler))
mux.Handle("/admin/", admin(adminHandler))
mux.Handle("/app/", web(appHandler))

func apiHandler(w http.ResponseWriter, r *http.Request) {
	userID, _ := jwt.GetUserID(r.Context())
	roleID, _ := jwt.GetRoleID(r.Context()) // uint64
	// ...
}

// Вход / выход
h.SetCookies(w, pair)
h.ClearCookies(w)
```

Ответы об ошибках по умолчанию: `401` с заголовком `WWW-Authenticate: Bearer` и
телом `{"error":"no token"|"token expired"|"unauthorized"}` либо `403` с
`{"error":"forbidden"}`.

### gRPC

```go
authz := jwt.NewAuthInterceptor(h,
	jwt.WithMethodRoles(map[string][]uint64{
		"/billing.Billing/Refund": {roleAdmin},
		"/billing.Billing/List":   {}, // любой аутентифицированный
	}),
	jwt.WithPublicMethods("/auth.Auth/Login", "/grpc.health.v1.Health/Check"),
	jwt.WithUnlistedPolicy(jwt.UnlistedDeny), // по умолчанию UnlistedAuthenticated
)

srv := grpc.NewServer(
	grpc.ChainUnaryInterceptor(authz.UnaryServerInterceptor()),
	grpc.ChainStreamInterceptor(authz.StreamServerInterceptor()),
)
```

Клиент передаёт `authorization: Bearer <access>`. Если токена нет или он
невалиден, возвращается `codes.Unauthenticated`, если не подходит роль —
`codes.PermissionDenied`. С `WithMetadataRefresh()` клиент может прислать ещё и
`refresh-token: <refresh>`; новая пара вернётся в заголовках ответа
`authorization` и `refresh-token`.

## Производительность

Замеры на Intel Core Ultra 5 225H, Go 1.27.1 (`go test -bench=. -benchmem`):

| Бенчмарк | нс/оп | Б/оп | аллокаций/оп |
|---|---:|---:|---:|
| GeneratePair | 10 947 | 5 672 | 79 |
| ParseAccess | 5 828 | 2 268 | 39 |
| ParseAccess (параллельно, 14 потоков) | 1 340 | 2 287 | 39 |
| HTTPMiddleware (bearer) | 5 886 | 2 637 | 41 |

`Helper` и `AuthInterceptor` после создания не меняются, на горячем пути
блокировок нет, парсер создаётся один раз. Единственная общая блокировка —
внутри `MemoryRefreshStore`, и берётся она только при обновлении токенов.

## Безопасность

- Секрет должен быть случайным и не короче 32 байт (`openssl rand -base64 48`),
  ротируйте его через менеджер секретов. При HS256 любой, кто проверяет токены,
  может их и выпускать. Если это неприемлемо, нужна асимметричная схема
  (RS256/EdDSA + JWKS).
- **Подключите `RefreshStore`.** Без него украденный refresh-токен действует
  весь срок жизни, и обменять его можно сколько угодно раз.
  `MemoryRefreshStore` подходит только для одного инстанса; для нескольких
  реплик реализуйте `Consume` на Redis (`SET jti 1 NX EXAT exp`) или SQL
  (`INSERT ... ON CONFLICT DO NOTHING`).
- Access-токен нельзя отозвать до истечения срока, поэтому `AccessTTL` держите коротким.
- По умолчанию cookie `HttpOnly`, `Secure`, `SameSite=Strict`. Задайте
  `CookieConfig.RefreshPath`, чтобы refresh-cookie уходил только на эндпоинт обновления.
- `Secure: false` — только для локальной разработки по HTTP.

## Миграция (с 1.3.x)

| Было | Стало |
|---|---|
| `NewHelper(secret string) Helper` | `NewHelper(secret []byte, opts...) (*Helper, error)`; секреты короче 32 байт отклоняются |
| `GeneratePair(userID, issuerName, roleID)` | `GeneratePair(userID, roleID)`; issuer задаётся через `WithIssuer` |
| `ParseToken` + `ParseMapClaims` (`jwt.MapClaims`) | `ParseAccess` / `ParseRefresh`, возвращают `*Claims` |
| `CustomClaims{UserID, IssuerName, ExpireAt, IssuedAt, RoleID}` | `Claims{RoleID, Type, jwt.RegisteredClaims}`; id пользователя — `Subject` / `UserID()` |
| Claims `id`, `iss_at` | Стандартные `sub`, `iat`, плюс `typ`, `jti`, `nbf`. **Токены, выпущенные 1.3.x, не принимаются.** |
| `PrepareCookies(pair)` | `Cookies(pair)` / `SetCookies(w, pair)` |
| `AccessTokenDuration`, `RefreshTokenDuration` (int) | `DefaultAccessTTL`, `DefaultRefreshTTL` (`time.Duration`) |
| `Middleware(h, secret, roles...)` | `h.HTTPMiddleware(WithRoles(...), WithCookieRefresh())`. Старая функция осталась (deprecated) и **паникует на секретах короче 32 байт** |
| `GetRoleID(ctx) (int, error)` | `GetRoleID(ctx) (uint64, error)` |
| Строковые ключи контекста, `GetUserIdCtxKey`, `GetRoleIdCtxKey`, grpc_ctxtags | Удалены; используйте `ClaimsFromContext` / `GetUserID` / `GetRoleID` |
| `NewAuthInterceptor(helper, roles)` + `AuthorizeHandler` (grpc_auth) | `NewAuthInterceptor(h, WithMethodRoles(roles), ...)` + `UnaryServerInterceptor` / `StreamServerInterceptor`. `AuthorizeHandler` остался (deprecated) |
| Неперечисленные gRPC-методы были публичными | Теперь требуют валидный токен; старое поведение — `WithUnlistedPolicy(UnlistedPublic)` |
| Обновление через gRPC metadata было всегда включено | Включается явно: `WithMetadataRefresh()` |
| Все ошибки gRPC были `PermissionDenied` | `Unauthenticated` для учётных данных, `PermissionDenied` для ролей |
| Ошибки HTTP были простым текстом | Тело в JSON, заголовок `WWW-Authenticate`; своя логика — через `WithErrorHandler` |
