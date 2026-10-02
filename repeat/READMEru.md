# repeat

[English version](README.md) · [← go-core](../README.md)

Повтор операций с экспоненциальной задержкой, ограничением сверху и
джиттером. В пакете также есть `http.RoundTripper` с повторами и помощник для
подключения по WebSocket. Настройки по умолчанию рассчитаны на продакшен:
небольшое конечное число попыток, рандомизированные экспоненциальные паузы и
никаких новых попыток после отмены контекста.

```go
import "github.com/jwm1rr0rb10/go-core/repeat"
```

## Возможности

- **Конечное число попыток по умолчанию.** 4 попытки, задержка растёт от
  100 мс до 10 с с полным джиттером.
- **Четыре стратегии джиттера:** none, full, equal, decorrelated. Джиттер
  применяется после ограничения, поэтому ограничение его не «съедает».
- **Понятные бюджеты.** `WithMaxAttempts` считает все попытки вместе с первой.
  `WithMaxElapsed` задаёт бюджет по времени. Дедлайн контекста учитывается
  *до* ожидания: если пауза выйдет за дедлайн, пакет не ждёт впустую.
- **Управление повторами.** `Permanent(err)` прекращает повторы сразу,
  `WithRetryIf` / `WithRetryPolicy` фильтруют ошибки, `RetryAfter(err, d)`
  позволяет серверу задать следующую паузу.
- **Хук для наблюдаемости.** `WithOnRetry(func(RetryEvent))` для логов и
  метрик. Сам пакет ничего не логирует.
- **Дженерик** `Do[T]` для операций, которые возвращают значение.
- **Без аллокаций** на горячем пути, если заранее создать `Retrier`. Он
  неизменяемый и безопасен для конкурентного использования.
- **HTTP `Transport`:**
  - по умолчанию повторяет только идемпотентные запросы;
  - перематывает тело запроса через `GetBody`;
  - учитывает `Retry-After`;
  - вычитывает и закрывает отброшенные ответы, чтобы соединения
    переиспользовались.

## API

| Идентификатор | Назначение |
|---|---|
| `Exec(ctx, op, opts...) error` | Выполнить `op(ctx, attempt)` с повторами |
| `Do[T](ctx, fn, opts...) (T, error)` | То же для функций, возвращающих значение |
| `New(opts...) (*Retrier, error)` / `(*Retrier).Exec` / `DoWith` | Проверить настройки один раз и переиспользовать |
| `WithMaxAttempts(n)` / `Unlimited` | Общее число попыток (1 — без повторов) |
| `WithBackoff(base, max)`, `WithBaseDelay`, `WithMaxDelay`, `WithConstantDelay` | Форма задержки |
| `WithJitter(JitterFull \| JitterNone \| JitterEqual \| JitterDecorrelated)` | Рандомизация |
| `WithMaxElapsed(d)` | Общий бюджет времени → `ErrMaxElapsed` |
| `WithRetryIf(fn)`, `WithRetryPolicy(fn)` | Какие ошибки повторять |
| `WithOnRetry(fn)` | Хук перед каждой паузой (`RetryEvent{Attempt, Err, Delay}`) |
| `Permanent(err)`, `IsPermanent(err)` | Прекратить повторы |
| `RetryAfter(err, d)` | Минимальная пауза перед следующей попыткой |
| `*Error{Attempts, Err, Cause}` | Возвращается, когда попытки исчерпаны; разворачивается в `Err` и `Cause` |
| `Backoff(base, limit, attempt)` | `min(limit, base·2ⁿ)` без переполнения |
| `NewTransport(base, opts...)`, `Transport{RetryStatus, RetryNonIdempotent}` | `http.RoundTripper` с повторами |
| `NewClient(*http.Client, opts...) *http.Client` | Копия клиента с транспортом, который делает повторы |
| `DefaultRetryStatus(code)` | 429, 502, 503, 504 |
| `StatusError` | Ошибка попытки с «повторяемым» статусом (видна в хуках) |
| `ConnectWithRetry(ctx, dial, opts...)`, `Dialer(d, url, h)` | Подключение по WebSocket |

## Использование

```go
// Разовый вызов с настройками по умолчанию (4 попытки, backoff с джиттером).
err := repeat.Exec(ctx, func(ctx context.Context, attempt int) error {
	return client.Ping(ctx)
})

// Общий Retrier с политикой, хуком для метрик и возвратом значения.
var retrier, _ = repeat.New(
	repeat.WithMaxAttempts(5),
	repeat.WithBackoff(50*time.Millisecond, 2*time.Second),
	repeat.WithMaxElapsed(10*time.Second),
	repeat.WithRetryIf(func(err error) bool { return !errors.Is(err, ErrNotFound) }),
	repeat.WithOnRetry(func(e repeat.RetryEvent) {
		retries.Inc()
		slog.Debug("retrying", "attempt", e.Attempt, "err", e.Err, "delay", e.Delay)
	}),
)

user, err := repeat.DoWith(ctx, retrier, func(ctx context.Context) (*User, error) {
	u, err := repo.Get(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repeat.Permanent(err) // повторять бессмысленно
	}
	return u, err
})

var re *repeat.Error
if errors.As(err, &re) {
	slog.Warn("gave up", "attempts", re.Attempts, "cause", re.Cause)
}
```

HTTP:

```go
client := repeat.NewClient(&http.Client{Timeout: 10 * time.Second},
	repeat.WithMaxAttempts(3),
	repeat.WithBackoff(100*time.Millisecond, 2*time.Second),
)
resp, err := client.Get("https://api.example.com/items") // повтор на 429/502/503/504 и сетевых ошибках

// Или подключите транспорт к существующему клиенту или SDK.
tr := repeat.NewTransport(http.DefaultTransport, repeat.WithMaxAttempts(4))
tr.RetryStatus = func(code int) bool { return code == 500 || repeat.DefaultRetryStatus(code) }
sdkClient := &http.Client{Transport: tr}
```

WebSocket:

```go
conn, err := repeat.ConnectWithRetry(ctx,
	repeat.Dialer(nil, "wss://stream.example.com/ws", nil),
	repeat.WithMaxAttempts(repeat.Unlimited),
	repeat.WithMaxElapsed(time.Minute),
)
```

## Тонкости поведения

- **Нумерация попыток.** `attempt` в операции и `RetryEvent.Attempt`
  считаются с нуля. `Error.Attempts` — это количество попыток.
- **Что возвращается.**
  - Отклонённая ошибка (`Permanent`, `RetryIf`) возвращается **как есть**.
    Обёртка `Permanent` снимается, если она на верхнем уровне.
  - Если закончились попытки, время или контекст, результат — `*repeat.Error`.
    `errors.Is` находит и последнюю ошибку, и причину остановки
    (`context.Canceled`, `context.DeadlineExceeded`, `ErrMaxElapsed`).
- **Паузы из `Retry-After`.** Подсказка `RetryAfter` (или заголовок
  `Retry-After`) может быть больше `MaxDelay`, но всё равно ограничена
  `MaxElapsed` и дедлайном контекста.
- **Transport и идемпотентность.**
  - Идемпотентные запросы — это `GET`, `HEAD`, `OPTIONS`, `TRACE`, `PUT`,
    `DELETE` и любой запрос с заголовком `Idempotency-Key` /
    `X-Idempotency-Key`.
  - Запрос, тело которого нельзя перемотать (нет `GetBody`), отправляется
    один раз.
  - Когда повторы кончились, возвращается **последний ответ** с `nil`-ошибкой,
    как того требует контракт `http.RoundTripper`.
- **Таймаут клиента.** `http.Client.Timeout` действует на все попытки вместе.

## Производительность

Замеры на Intel Core Ultra 5 225H, Go 1.27, `-benchmem`:

| Бенчмарк | нс/оп | Б/оп | аллокаций/оп |
|---|---:|---:|---:|
| `Retrier.Exec`, успех | 36,8 | 0 | 0 |
| `Retrier.Exec`, 2 повтора с нулевой паузой | 68,5 | 0 | 0 |
| `Exec(ctx, op, opts...)` (конфиг собирается на каждый вызов) | 74,7 | 64 | 1 |

Случайные числа берутся из глобального источника `math/rand/v2`, который
безопасен для горутин. Между паузами переиспользуется один `time.Timer`.

## Миграция (с 1.3.x)

| Было | Стало |
|---|---|
| По умолчанию бесконечные повторы (до скрытых 24 ч) | По умолчанию **4 попытки**. Бесконечность — явно: `WithMaxAttempts(repeat.Unlimited)` + `WithMaxElapsed` |
| `WithMaxAttempts(n)` означал число *повторов* (n+1 вызов) | Означает **общее число попыток** (n вызовов) |
| `WithMinWait` / `WithMaxWait` (случайная пауза в диапазоне; min перекрывал backoff) | `WithBackoff(base, max)` / `WithConstantDelay`. Старые имена оставлены как устаревшие синонимы base/max |
| `WithExponentialBackoff`, `WithErrorFilter` | Устаревшие синонимы `WithBackoff`, `WithRetryIf` |
| `WithJitter(func(time.Duration) time.Duration)` | `WithJitter(repeat.Jitter)` — константа стратегии |
| Экспортируемый `Config`, `OptionSetter` | Конфиг внутренний; `Option` (`OptionSetter` оставлен как синоним) |
| Ошибки: строковая обёртка `"max attempts (n): err"`; при отмене возвращался голый `ctx.Err()` | `*repeat.Error` с `Attempts`, `Err` и `Cause`; `errors.Is` работает для обеих ошибок |
| `NewClient` возвращал `*ClientWithRetry` | Возвращает `*http.Client`; `client.Do(req)` по-прежнему работает. Повторы делает `Transport` |
| HTTP повторял любой 5xx для любого метода и терял тело запроса при повторе | Повторяет 429/502/503/504 и сетевые ошибки для идемпотентных запросов, перематывает тело, учитывает `Retry-After` |
| `TemporaryError` | `StatusError` (виден только в хуках; `RoundTrip` возвращает последний ответ) |
| `ConnectWithRetry` писал в `log.Printf` | Ничего не пишет. Используйте `WithOnRetry` |
| Константы `MaxTotalDuration`, `DefaultMinWait` и т. п. | `DefaultMaxAttempts`, `DefaultBaseDelay`, `DefaultMaxDelay`, `Unlimited` |
