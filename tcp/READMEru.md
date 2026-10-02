# tcp

[← go-core](../README.md) · [English version](README.md)

Набор компонентов для нагруженных TCP-сервисов: сервер с лимитом соединений,
idle-таймаутами, middleware и плавной остановкой; клиент с контекстами, TLS,
фреймингом и ретраями; пул клиентов с честной проверкой живости соединения;
шардированный rate limiter по IP с адаптивным proof-of-work. Используются
только стандартная библиотека и [`go-errors`](https://github.com/jwm1rr0rb10/go-errors),
горячие пути не аллоцируют память.

```bash
go get github.com/jwm1rr0rb10/go-core
```

```go
import "github.com/jwm1rr0rb10/go-core/tcp"
```

## Возможности

- **Server.** Точный лимит соединений, который можно менять на лету, и
  backoff при ошибках `Accept` (5 мс → 1 с, при EMFILE сервер не крутит CPU
  вхолостую). Кроме того:
  - скользящий idle-таймаут;
  - перехват паник в обработчиках;
  - цепочка middleware;
  - TLS;
  - `Serve(listener)` / `ListenAndServe` / `Start`;
  - `Shutdown(ctx)`: закрывает листенеры, отменяет контекст обработчиков,
    ждёт их завершения и принудительно закрывает зависшие;
  - статистика без блокировок на каждом соединении.
- **Client.** Все операции принимают `context.Context` (и дедлайн, и отмену).
  Также есть:
  - TLS через `tls.Dialer`;
  - буферизованное чтение и `ReadInto` без аллокаций;
  - фреймы с префиксом длины;
  - `Do`, который повторяет весь обмен запрос/ответ с экспоненциальным
    backoff и full jitter.

  `Close` окончательный: закрытый клиент больше не «оживает».
- **Pool.** LIFO-переиспользование с лимитами активных (`Get(ctx)` ждёт
  свободный слот) и простаивающих клиентов. Поддерживаются idle timeout и
  max lifetime. **Неблокирующая проверка через `MSG_PEEK`** отсеивает
  соединения, которые пир закрыл, сбросил или на которых остались
  непрочитанные данные. `Put` после `Close` безопасен, для сломанных клиентов
  есть `Discard`.
- **RateLimiter.** Два token bucket на каждый IP: выше мягкого лимита нужно
  решить proof-of-work, выше жёсткого — бан. Сложность адаптивная (+1 бит за
  каждый вызов, со временем снижается). IPv6 группируется по /64, память
  ограничена. Состояние разбито на 64 шарда, в map нет указателей (GC их не
  сканирует), часы можно подменить в тестах.
- **Proof-of-work.** Задачи на SHA-256 со сложностью в ведущих нулевых битах.
  Каждая задача содержит 128-битный случайный префикс и срок действия, так
  что повторно использовать решение нельзя. Проверка без аллокаций.
  `SolvePoW` задействует все ядра, `AnswerPoW` реализует клиентскую сторону.
- **Фрейминг.** `WriteFrame` / `ReadFrame` с 4-байтовым big-endian префиксом
  длины. Запись идёт одним вызовом с объединением заголовка и данных, буферы
  переиспользуются, длина проверяется до выделения памяти.

## Обзор API

| Область | API |
|---|---|
| Сервер | `NewServer(addr, HandlerFunc, ...ServerOption)`, `Start`, `ListenAndServe`, `Serve(net.Listener)`, `Shutdown(ctx)`, `Close`, `Stats`, `SetMaxConnections`, `Addr` |
| Опции сервера | `WithMaxConnections`, `WithIdleTimeout`, `WithMiddleware`, `WithServerTLS`, `WithServerLogger`, `WithListenConfig`, `WithBaseContext` |
| Клиент | `NewClient`, `Dial`, `Connect`, `Read`, `ReadInto`, `ReadFull`, `Write`, `ReadFrame`, `WriteFrame`, `Do`, `WriteWithRetry`, `ReadWithRetry`, `Reconnect`, `Close`, `Stats` |
| Опции клиента | `WithTimeouts`, `WithDialTimeout`, `WithDialer`, `WithBufferSize`, `WithMaxFrameSize`, `WithTLSClientConfig`, `WithRetryPolicy`, `WithClientLogger` |
| Пул | `NewPool(Factory, ...PoolOption)`, `Get(ctx)`, `Put`, `Discard`, `Prune`, `Close`, `Stats` |
| Опции пула | `WithMaxActive`, `WithMaxIdle`, `WithPoolIdleTimeout`, `WithPoolMaxLifetime`, `WithPoolHealthCheck`, `WithPoolLogger` |
| Rate limiter | `NewRateLimiter(...RateLimiterOption)`, `Check(netip.Addr)`, `Middleware()`, `Cleanup`, `Len`, `Stop` |
| Опции лимитера | `WithConnectionRate`, `WithBanRate`, `WithBanDuration`, `WithPoW`, `WithPoWHandshake`, `WithPoWDifficulty`, `WithPoWTimeout`, `WithDifficultyDecay`, `WithIPv6PrefixLen`, `WithMaxTrackedPeers`, `WithCleanupInterval`, `WithClock`, `WithRateLimiterLogger` |
| Proof-of-work | `NewPoWChallenge`, `(*PoWChallenge).Verify`, `SolvePoW`, `AnswerPoW`, `WritePoWChallenge`, `ParsePoWChallenge`, `WritePoWSolution`, `ReadPoWSolution` |
| Фрейминг | `WriteFrame`, `ReadFrame`, `FrameHeaderSize`, `DefaultMaxFrameSize` |
| Ошибки | `IsRetryable`, `ConnectionError`, `ErrConnectionClosed`, `ErrTimeout`, `ErrClientClosed`, `ErrServerClosed`, `ErrPoolClosed`, `ErrFrameTooLarge`, `ErrBanned`, `ErrRateLimited`, `ErrPoWFailed` |
| TLS | `ServerTLSConfig(cert, key)`, `ClientTLSConfig(insecure)` |

## Примеры

### Сервер с плавной остановкой

```go
srv, err := tcp.NewServer(":9000", func(ctx context.Context, conn net.Conn) {
	var buf []byte
	for {
		var err error
		if buf, err = tcp.ReadFrame(conn, buf[:0], 1<<20); err != nil {
			return // EOF, idle-таймаут или остановка сервера
		}
		if err := tcp.WriteFrame(conn, buf, 1<<20); err != nil {
			return
		}
	}
},
	tcp.WithMaxConnections(50_000),
	tcp.WithIdleTimeout(2*time.Minute),
	tcp.WithServerLogger(slog.Default()),
)
if err != nil {
	return err
}
go func() {
	if err := srv.ListenAndServe(); !errors.Is(err, tcp.ErrServerClosed) {
		slog.Error("serve", "err", err)
	}
}()

<-ctx.Done() // например, signal.NotifyContext
shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
defer cancel()
_ = srv.Shutdown(shutdownCtx) // ждёт обработчики, через 15 с закрывает принудительно
```

### Клиент с ретраями

```go
client, err := tcp.Dial(ctx, "10.0.0.5:9000",
	tcp.WithTimeouts(2*time.Second, 2*time.Second),
	tcp.WithRetryPolicy(tcp.RetryPolicy{MaxAttempts: 4, BaseDelay: 50 * time.Millisecond, MaxDelay: time.Second}),
)
if err != nil {
	return err
}
defer client.Close()

var resp []byte
err = client.Do(ctx, func(ctx context.Context, c *tcp.Client) error {
	if err := c.WriteFrame(ctx, req); err != nil {
		return err
	}
	var err error
	resp, err = c.ReadFrame(ctx, resp[:0])
	return err
})
```

### Пул

```go
pool, _ := tcp.NewPool(func(ctx context.Context) (*tcp.Client, error) {
	return tcp.Dial(ctx, addr)
}, tcp.WithMaxActive(256), tcp.WithMaxIdle(64),
	tcp.WithPoolIdleTimeout(time.Minute), tcp.WithPoolMaxLifetime(30*time.Minute))
defer pool.Close()

c, err := pool.Get(ctx) // ждёт слот, если заняты все 256 клиентов
if err != nil {
	return err
}
if err := c.WriteFrame(ctx, req); err != nil {
	pool.Discard(c) // соединение сломано: закрываем и освобождаем слот
	return err
}
pool.Put(c)
```

### Rate limiting с proof-of-work

```go
rl := tcp.NewRateLimiter(
	tcp.WithConnectionRate(20, 40), // 20/с на IP, всплеск до 40, дальше — PoW
	tcp.WithBanRate(100, 200),      // ещё выше — бан на минуту
	tcp.WithPoWDifficulty(16, 24),
	tcp.WithPoWHandshake(true),     // клиенты вызывают tcp.AnswerPoW
)
defer rl.Stop()

srv, _ := tcp.NewServer(":9000", handler, tcp.WithMiddleware(rl.Middleware()))
go srv.ListenAndServe()

// на стороне клиента
conn, _ := net.Dial("tcp", "server:9000")
if err := tcp.AnswerPoW(ctx, conn); err != nil { // "OK" или решение задачи + "OK"
	return err
}
```

Протокол (текстовые строки, не длиннее 256 байт):

```
server → POW prefix=<32 hex> difficulty=<биты> expires=<unix-секунды>
client → SOLUTION nonce=<1..64 символа>        # у SHA-256(prefix+nonce) не меньше <биты> ведущих нулей
server → OK                                     # только с WithPoWHandshake
```

Без `WithPoWHandshake` соединения в пределах лимита не получают никаких
лишних байтов, поэтому существующие протоколы не ломаются. Данные, которые
клиент отправил сразу после решения, не теряются и достаются обработчику.

## Конкурентность и производительность

- Все экспортируемые типы безопасны для конкурентного использования.
  `Client`, как и `net.Conn`, допускает одновременно одного читателя и одного
  писателя.
- На горячих путях ничего не логируется. По умолчанию логгер —
  `slog.DiscardHandler`.
- Счётчики байтов и активности сервера живут на каждом соединении: занятые
  соединения не делят одну кэш-линию, а `Stats()` суммирует их по запросу.
- Скользящий idle-дедлайн обновляется не чаще раза в `idle/16`. Если
  обработчик ставит свой дедлайн, автообновление для этого направления
  выключается, пока дедлайн не сбросят нулевым временем.
- У лимитера 64 шарда с отдельными мьютексами, выровненных по кэш-линиям.
  В map нет указателей.

Бенчмарки (Go 1.27.1, linux/amd64, Intel Core Ultra 5 225H, 14 потоков; `go test -bench . -benchmem`):

| Бенчмарк | нс/оп | Б/оп | аллокаций/оп |
|---|---:|---:|---:|
| Эхо фреймами Client ↔ Server через loopback (128 Б) | 7 600 – 10 300 | 0 | 0 |
| `WriteFrame` + `ReadFrame` (512 Б, в памяти) | 43 – 48 | 0 | 0 |
| Обёртка серверного соединения, на один `Read` | 92 – 116 | 0 | 0 |
| `RateLimiter.Check`, один активный пир | 110 – 120 | 0 | 0 |
| `RateLimiter.Check`, параллельно, много пиров | 37 – 40 | 0 | 0 |
| `PoWChallenge.Verify` | 99 – 106 | 0 | 0 |
| `SolvePoW`, 16 бит (все ядра) | ~1,3 мс | — | — |
| `IsRetryable` | 4,8 | 0 | 0 |

Каждый дополнительный бит PoW удваивает ожидаемую работу клиента: 16 бит —
около 65 тыс. хешей, 24 бита — около 16,7 млн (несколько секунд на одном
ядре). Проверка на сервере всегда стоит один хеш.

## Безопасность

- **Прокси и балансировщики.** Лимитер считает по `conn.RemoteAddr()`. За
  L4-балансировщиком это адрес балансировщика, и все клиенты попадут в один
  bucket. Включите на балансировщике PROXY protocol и оберните листенер
  (например, `github.com/pires/go-proxyproto`) перед `Serve`.
- **Соединения без IP.** Лимитер отклоняет соединения, у которых удалённый
  адрес не IP (например, Unix-сокеты).
- **Переполнение таблицы.** Когда таблица пиров заполнена
  (`WithMaxTrackedPeers`), новые пиры решают PoW максимальной сложности.
  Если PoW выключен, их отклоняют.
- **Повтор решений.** Задачи выдаются на каждое соединение, привязаны к
  128 случайным битам и истекают через `WithPoWTimeout`, так что повторно
  использовать решение нельзя. Обмен PoW идёт под отдельным дедлайном, это
  защищает от slow-loris.
- **Размер фреймов.** `ReadFrame` проверяет длину до выделения памяти, так
  что «длина 4 ГиБ» от злоумышленника ничего не стоит.
- **TLS.** `ClientTLSConfig(true)` отключает проверку сертификата, это
  только для тестов. Оба хелпера требуют TLS 1.2+.
- **Ретраи.** Семантика «at least once»: `Do` и `WriteWithRetry` подходят
  только для идемпотентных запросов.

## Миграция с 1.x

| Было | Стало |
|---|---|
| `NewServer(addr, func(net.Conn), tlsCfg, opts...)` | `NewServer(addr, func(ctx, net.Conn), opts...)` + `WithServerTLS(cfg)`; обработчик получает контекст, который отменяется при остановке |
| `WithServerLogger(*log.Logger)` (и у клиента/пула) | `*slog.Logger`; по умолчанию логи отбрасываются, а не идут в `log.Default()` |
| `WithMiddleware(func(net.Conn) bool)`, `ApplyMiddleware` | `WithMiddleware(...Middleware)`, где `Middleware func(ctx, net.Conn) (net.Conn, error)`; отклонённые соединения закрывает сервер; `ApplyMiddleware` удалён |
| `Stop()`, `StopWithTimeout(d)` | `Shutdown(ctx)` / `Close()`; старые методы оставлены как deprecated-обёртки и больше не текут горутинами. Перезапустить сервер нельзя |
| `WithServerTimeout(d)` (абсолютный дедлайн) | `WithIdleTimeout(d)`, скользящий idle-таймаут (старое имя оставлено как deprecated-алиас) |
| по умолчанию максимум 65101 соединение | по умолчанию без лимита; задайте `WithMaxConnections` |
| `NewClient(addr, tlsCfg, opts...)` | `NewClient(addr, opts...)` + `WithTLSClientConfig(cfg)`; также есть `Dial(ctx, addr, opts...)` |
| `Connect()`, `Read()`, `Write(b)`, `Reconnect()` | первым аргументом принимают `ctx`. Повторный `Connect` ничего не делает; `Close` окончательный, `Reconnect` после него возвращает `ErrClientClosed` |
| `WriteWithRetry(data, n, backoff)`, `ReadWithRetry(n, backoff)` | `WriteWithRetry(ctx, data)`, `ReadWithRetry(ctx)` + `WithRetryPolicy`; лучше использовать `Do` |
| `ConnectionStats.RetryCount` (сбрасывался при успехе) | общее число ретраев; новое поле `Reconnects` |
| буфер чтения по умолчанию 1024 | 4096 (`WithBufferSize`) |
| `NewConnectionPool(func() (*Client, error), size)`, `ConnectionPool`, `Get()` | `NewPool(func(ctx) (*Client, error), opts...)`, `Pool`, `Get(ctx)`; новые `Discard`, `Prune`, `Stats`; `Close` возвращает ошибку; `WithPoolPingTimeout` удалён (проверка не блокируется) |
| `NewRateLimiter(*log.Logger)`, фиксированное окно в 1 секунду | `NewRateLimiter(opts...)` на token bucket; `RateLimitMiddleware(rl)` deprecated, используйте `rl.Middleware()` |
| сложность PoW в **hex-символах** (по умолчанию 4..8) | сложность в **битах** (по умолчанию 16..24; старые 4 символа = 16 бит) |
| `GeneratePoWChallenge(d)`, `PoWSolution`, `ValidatePoWSolution` | `NewPoWChallenge(bits, ttl)`, `(*PoWChallenge).Verify(nonce)`; `ReadPoWSolution(*bufio.Reader) (string, error)`; в строке задачи появилось поле `expires=` |
| экспортируемые константы `TCP`, `Read`, `Write` | удалены |
| тексты ошибок вида `"connection closed"` | теперь с префиксом `"tcp: "`; сравнивайте через `errors.Is`, а не по строке |
