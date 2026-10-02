# go-core

[English version](README.md)

Общие строительные блоки для бэкенд-сервисов на Go: аутентификация, повторы запросов,
graceful shutdown, защита горутин от паник, нагруженный TCP, UUID, валидация и небольшие
дженерик-хелперы. Каждый пакет проверен тестами с `-race`, покрыт бенчмарками и
задокументирован на русском и английском.

```bash
go get github.com/jwm1rr0rb10/go-core@latest
```

Нужен Go 1.27+. Подключайте только нужные пакеты — они независимы друг от друга, а тяжёлые
зависимости (gRPC, websocket, go-playground/validator) подтягивают только те пакеты,
которые их используют.

---

## Пакеты

| Пакет | Назначение | Документация |
|---|---|---|
| [`api/jwt`](api/jwt) | Пары access/refresh-токенов на HS256 с обязательной проверкой типа токена, одноразовые refresh-токены, middleware для `net/http`, unary/stream-интерсепторы для gRPC | [RU](api/jwt/READMEru.md) · [EN](api/jwt/README.md) |
| [`array`](array) | Дженерик-функции для слайсов (map/filter/group/zip/chunk/sort…) и ленивые варианты на `iter.Seq`; входные данные не изменяются | [RU](array/READMEru.md) · [EN](array/README.md) |
| [`bytes`](bytes) | JSON ↔ Go-значения: дженерик `Marshal`/`Unmarshal`, хелперы для `map[string]any` без потери точности больших чисел | [RU](bytes/READMEru.md) · [EN](bytes/README.md) |
| [`clock`](clock) | Интерфейс `Clock` по образцу `time` (таймеры, тикеры, `AfterFunc`) и детерминированный `Mock` для тестов | [RU](clock/READMEru.md) · [EN](clock/README.md) |
| [`closer`](closer) | Graceful shutdown: закрытие ресурсов в порядке LIFO с таймаутами, перехватом паник и обработкой сигналов | [RU](closer/READMEru.md) · [EN](closer/README.md) |
| [`healthcheck`](healthcheck) | Сервер `grpc.health.v1`, статус которого зависит от состояния зависимостей и периодических проверок | [RU](healthcheck/READMEru.md) · [EN](healthcheck/README.md) |
| [`pointer`](pointer) | Хелперы для необязательных значений в виде указателей (`ToPointer`, `ValueOr`, `Coalesce`, `Equal`…) | [RU](pointer/READMEru.md) · [EN](pointer/README.md) |
| [`random`](random) | Быстрые (`math/rand/v2`) и криптостойкие (`crypto/rand`) случайные числа, строки, токены, даты, IP | [RU](random/READMEru.md) · [EN](random/README.md) |
| [`repeat`](repeat) | Повторы с экспоненциальной задержкой и джиттером, `http.RoundTripper` с повторами, дозвон до WebSocket | [RU](repeat/READMEru.md) · [EN](repeat/README.md) |
| [`safe`](safe) | Превращает паники в типизированные ошибки `*PanicError`; безопасные горутины и вызовы | [RU](safe/READMEru.md) · [EN](safe/README.md) |
| [`safe/errorgroup`](safe/errorgroup) | `errgroup`, устойчивый к паникам, с лимитом параллелизма и режимами «первая ошибка» / «все ошибки» | [RU](safe/errorgroup/READMEru.md) · [EN](safe/errorgroup/README.md) |
| [`safe/waitgroup`](safe/waitgroup) | `WaitGroup` без блокировок, который сообщает о неправильном использовании и умеет ждать с контекстом или таймаутом | [RU](safe/waitgroup/READMEru.md) · [EN](safe/waitgroup/README.md) |
| [`tcp`](tcp) | TCP-сервер для высокой нагрузки, клиент с контекстом, фреймингом и повторами, пул с проверкой живости соединений, шардированный rate limiter, proof-of-work | [RU](tcp/READMEru.md) · [EN](tcp/README.md) |
| [`time`](time) | Unix-метки с неизвестной единицей, кэш часовых поясов, смещения с минутами, форматирование длительностей, замер времени через `slog` | [RU](time/READMEru.md) · [EN](time/README.md) |
| [`uuid`](uuid) | Тип UUID по RFC 9562: быстрый разбор и форматирование, поддержка JSON/SQL, генераторы v4 и монотонного v7 без блокировок | [RU](uuid/READMEru.md) · [EN](uuid/README.md) |
| [`uuid/db`](uuid/db) | Криптостойкие v4 и монотонные v7 для ключей в базе данных | [RU](uuid/db/READMEru.md) · [EN](uuid/db/README.md) |
| [`uuid/network`](uuid/network) | Очень быстрые несекретные UUID для ID запросов, трейсов и сообщений | [RU](uuid/network/READMEru.md) · [EN](uuid/network/README.md) |
| [`uuid/google_uuid`](uuid/google_uuid) | Генераторы строковых ID за одним интерфейсом: google/uuid v4/v7 и монотонный ULID | [RU](uuid/google_uuid/READMEru.md) · [EN](uuid/google_uuid/README.md) |
| [`validator`](validator) | `Engine` для struct-тегов поверх go-playground/validator и составные проверки значений с единым типом ошибки | [RU](validator/READMEru.md) · [EN](validator/README.md) |

> Пакеты `time` и `bytes` называются так же, как стандартные. Импортируйте их с алиасом:
> `coretime "github.com/jwm1rr0rb10/go-core/time"`.

---

## Быстрый старт

### Аутентификация (HTTP + gRPC)

```go
auth, err := jwt.NewHelper([]byte(os.Getenv("JWT_SECRET")), // не меньше 32 байт
	jwt.WithIssuer("billing"),
	jwt.WithAccessTTL(5*time.Minute),
	jwt.WithRefreshStore(jwt.NewMemoryRefreshStore()), // одноразовые refresh-токены
)
if err != nil {
	log.Fatal(err)
}

mux.Handle("/api/", auth.HTTPMiddleware()(handler))
mux.Handle("/admin/", auth.HTTPMiddleware(jwt.WithRoles(1, 2))(adminAPI))

interceptor := jwt.NewAuthInterceptor(auth,
	jwt.WithMethodRoles(map[string][]uint64{"/billing.v1.Billing/Refund": {1}}),
	jwt.WithPublicMethods("/billing.v1.Billing/Ping"),
)
srv := grpc.NewServer(
	grpc.UnaryInterceptor(interceptor.UnaryServerInterceptor()),
	grpc.StreamInterceptor(interceptor.StreamServerInterceptor()),
)
```

### Повторы

```go
err := repeat.Exec(ctx, func(ctx context.Context, attempt int) error {
	return doWork(ctx)
}, repeat.WithMaxAttempts(5), repeat.WithBackoff(100*time.Millisecond, 5*time.Second))

body, err := repeat.Do(ctx, fetch, repeat.WithMaxElapsed(30*time.Second))

client := repeat.NewClient(nil) // повторяет идемпотентные запросы, учитывает Retry-After
```

### Параллельная работа без падений

```go
g, ctx := errorgroup.WithContext(ctx, errorgroup.WithLimit(16))
for _, u := range urls {
	g.Go(func(ctx context.Context) error { return download(ctx, u) })
}
err := g.Wait() // паника в задаче вернётся как *safe.PanicError
```

### TCP-сервер и graceful shutdown

```go
srv, err := tcp.NewServer(":9000",
	func(ctx context.Context, conn net.Conn) { _, _ = io.Copy(conn, conn) },
	tcp.WithMaxConnections(50_000),
	tcp.WithIdleTimeout(2*time.Minute),
	tcp.WithMiddleware(tcp.RateLimitMiddleware(tcp.NewRateLimiter())),
)
if err != nil {
	log.Fatal(err)
}
go srv.ListenAndServe()

lc := closer.NewLIFOCloser(closer.WithTimeout(30 * time.Second))
lc.AddFunc("tcp server", srv.Shutdown)
lc.AddFunc("http server", httpSrv.Shutdown)
if err := closer.CloseOnSignal(lc, syscall.SIGINT, syscall.SIGTERM); err != nil {
	log.Printf("shutdown: %v", err)
}
```

### Идентификаторы

```go
id := uuid.NewV7()                   // упорядочен по времени, строго монотонный, 0 аллокаций
fast := network.NewV4()              // для ID запросов и трейсов, не для секретов
token, err := random.SecureToken(32) // 32 байта из crypto/rand в виде 64 hex-символов
```

---

## Принципы

- **Безопасно по умолчанию.** Конечное число повторов, обязательная проверка типа токена,
  проверка длины секрета, криптостойкая случайность там, где это важно, никаких паник при
  неправильном использовании.
- **Никакого скрытого глобального состояния.** Всё настраиваемое живёт в экземплярах,
  которые создаются через функциональные опции; функции уровня пакета используют
  потокобезопасные значения по умолчанию.
- **Библиотеки не шумят.** На горячих путях нет логирования; компоненты, которым есть что
  сказать, принимают необязательный `*slog.Logger` и по умолчанию ничего не выводят.
- **Производительность измерена.** У горячих путей есть бенчмарки (`make bench`), большинство
  работают без аллокаций.
- **Совместимость со стандартной библиотекой.** Ошибки работают с `errors.Is` / `errors.As`,
  `context` учитывается везде, типы реализуют интерфейсы `encoding` и `database/sql`.

## Разработка

```bash
make check   # gofmt + go vet + go test -race
make cover   # тесты с -race и покрытием
make bench   # бенчмарки с -benchmem
make fuzz    # фаззинг (array, jwt, uuid)
make lint    # golangci-lint (настройки в .golangci.yml)
```

CI (`.github/workflows/ci.yml`) запускает gofmt, vet, тесты с `-race` и покрытием, а также
golangci-lint. Покрытие тестами — 92–100% по пакетам.

## История изменений

### Unreleased — большая переработка

Несовместимые изменения почти во всех пакетах. В README каждого пакета есть раздел
**Миграция** со списком изменений и инструкцией по обновлению.

- Путь модуля теперь `github.com/jwm1rr0rb10/go-core`, приватные импорты `kalipso/...` удалены.
- `api/jwt`: тип токена проверяется (refresh-токен больше не работает как access),
  стандартные claims, одноразовые refresh-токены, проверка длины секрета, gRPC-интерсепторы,
  правильные коды статусов gRPC. **Токены, выпущенные старыми версиями, отклоняются.**
- Исправлены гонки данных в `uuid/network`, `uuid/google_uuid`, `random`, `healthcheck`, `tcp`.
- `repeat`: по умолчанию конечное число повторов, корректные backoff и джиттер, тело запроса
  сохраняется при повторе, поддержка `Retry-After`.
- `tcp`: graceful `Shutdown(ctx)`, пауза при ошибках accept, реальная проверка живости
  соединений в пуле, шардированный rate limiter на token bucket, proof-of-work с защитой от
  повторного использования, фрейминг без аллокаций.
- `closer`: настоящий порядок LIFO, идемпотентное закрытие, таймауты и перехват паник для
  каждого ресурса.
- `clock`: API повторяет `time`, `Mock` — полноценные фейковые часы.
- `uuid`: единый тип `UUID`, монотонный v7 по RFC 9562, разбор строк, поддержка SQL/JSON.
- Удалены зависимости `grpc-ecosystem/go-grpc-middleware` и `golang.org/x/sync`.
- Добавлены Makefile, CI и конфигурация golangci-lint.

### 1.3.1 и ранее

Выпускались под старым путём модуля; подробности — в истории git.
