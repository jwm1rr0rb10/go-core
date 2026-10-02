# closer

[English version](README.md) · [← go-core](../README.md)

Помощник для graceful shutdown. Регистрируйте ресурсы по мере открытия и
закрывайте их все в обратном порядке одним вызовом: вручную или по
SIGINT/SIGTERM. Каждый ресурс закрывается ровно один раз, и вызов
завершается, даже если какой-то closer упал с ошибкой, запаниковал или
завис.

```go
import "github.com/jwm1rr0rb10/go-core/closer"
```

## Возможности

- **Один LIFO-список** для ресурсов любого вида:
  - `io.Closer`;
  - `Close()` без ошибки;
  - `func(ctx) error`, например `http.Server.Shutdown`.

  Закрываются строго в обратном порядке регистрации.
- **Ограниченное по времени завершение:**
  - `WithTimeout` ограничивает весь `Close`;
  - `WithCloserTimeout` ограничивает каждый closer.

  Closer, не уложившийся во время, попадает в отчёт и доделывает работу в
  фоне, а следующий запускается сразу.
- **Паника в closer'е перехватывается** и возвращается как `*safe.PanicError`.
  Остальные ресурсы всё равно закрываются.
- **Идемпотентный** `Close`: параллельные и повторные вызовы получают
  результат первого.
- **Ошибки с именами:** `close db: connection reset`, все объединены через
  `errors.Join`.
- **Поздняя регистрация:** ресурс, добавленный после начала `Close`,
  закрывается сразу и не утекает.
- **Сигналы:** `CloseOnSignal` ждёт SIGINT/SIGTERM. После первого сигнала
  восстанавливается стандартная обработка, поэтому **второй Ctrl+C убивает
  зависшее завершение**.
- По умолчанию ничего не пишет в лог; `*slog.Logger` подключается опцией.

## API

| Идентификатор | Назначение |
|---|---|
| `NewLIFOCloser(opts...)` / `var lc LIFOCloser` | Создание |
| `Add(...io.Closer)`, `AddNamed(name, io.Closer)` | Зарегистрировать closer, возвращающий ошибку |
| `AddNoErr(...NoErrCloser)` | Зарегистрировать `Close()` без ошибки |
| `AddFunc(name, func(ctx) error)` | Зарегистрировать функцию закрытия, принимающую контекст |
| `Close() error`, `CloseContext(ctx) error` | Закрыть всё (LIFO), вернуть объединённые ошибки |
| `Len() int` | Сколько ресурсов ещё зарегистрировано |
| `WithTimeout(d)`, `WithCloserTimeout(d)`, `WithLogger(l)` | Опции |
| `CloseOnSignal(lc, sigs...)` | Ждать сигнал, затем закрыть |
| `CloseOnSignalWithContext(ctx, lc, sigs...)` | То же, либо когда `ctx` завершён |
| `CloseOnSignalContext(lc, sigs...)` | Возвращает `func(ctx) error` для run-групп |
| `DefaultSignals` | `os.Interrupt`, `syscall.SIGTERM` |
| `CloserFunc`, `NoErrCloserFunc` | Адаптеры функций |

## Использование

```go
func main() {
	lc := closer.NewLIFOCloser(
		closer.WithTimeout(20*time.Second),
		closer.WithCloserTimeout(5*time.Second),
		closer.WithLogger(slog.Default()),
	)

	db := mustOpenDB()
	lc.AddNamed("postgres", db)

	nc := mustConnectNATS()
	lc.AddNoErr(nc) // (*nats.Conn).Close()

	srv := &http.Server{Addr: ":8080", Handler: router}
	lc.AddFunc("http", srv.Shutdown) // остановится первым: последним пришёл, первым ушёл
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http", "err", err)
		}
	}()

	if err := closer.CloseOnSignal(lc); err != nil {
		slog.Error("shutdown finished with errors", "err", err)
		os.Exit(1)
	}
}
```

## Замечания

- `ctx` в `CloseOnSignalWithContext` только **запускает** завершение. Сколько
  времени даётся на закрытие, решает `WithTimeout`, поэтому отменённый
  родительский контекст не обрывает shutdown.
- Closer не должен вызывать `Close` у того же `LIFOCloser`: такой вызов будет
  ждать сам себя. Вызывать `Add*` можно.
- Если ни один таймаут не задан, closer'ы выполняются прямо в вызывающей
  горутине, без дополнительных горутин.

Бенчмарк (Intel Core Ultra 5 225H, Go 1.27): регистрация и закрытие 8
ресурсов — 0,96 мкс, 1032 Б, 21 аллокация. Это не горячий путь.

## Миграция (с 1.3.x)

| Было | Стало |
|---|---|
| Closer'ы с ошибкой и без хранились в двух разных списках, и реальный порядок не был LIFO | Один список, строгий LIFO для всех видов |
| `Close` мог выполниться дважды и закрыть ресурсы повторно | Идемпотентен; повторные вызовы возвращают первую ошибку |
| Таймаутов не было: один зависший closer блокировал завершение навсегда | `WithTimeout`, `WithCloserTimeout`, `CloseContext(ctx)` |
| Паника в closer'е роняла shutdown | Перехватывается и возвращается как `*safe.PanicError` |
| Ошибки вида `close error: <err>` без имени ресурса | `close <name>: <err>` |
| `CloseOnSignal` писал в `log.Printf` и делал `defer close(ch)` для канала, зарегистрированного в `signal.Notify` | Без глобального логирования (`WithLogger`), на основе `signal.NotifyContext`; второй сигнал завершает процесс |
| Без списка сигналов функция ждала вечно | Без списка сигналов используются `DefaultSignals` (SIGINT, SIGTERM) |
| `NewLIFOCloser()` | `NewLIFOCloser(opts...)`; нулевое значение тоже работает |
| не было | `AddNamed`, `AddFunc`, `CloseContext`, `Len` |
