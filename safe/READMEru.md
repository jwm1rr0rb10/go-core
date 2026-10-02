# safe

[English version](README.md) · [← go-core](../README.md)

Превращает паники в ошибки. Паника в запущенной вами горутине роняет весь
процесс. `safe` перехватывает её, передаёт обработчику и возвращает как
типизированную ошибку `*PanicError`: со значением паники и стеком, снятым в
месте паники.

```go
import "github.com/jwm1rr0rb10/go-core/safe"
```

Подпакеты:

- [`safe/errorgroup`](errorgroup/READMEru.md): `errgroup` с защитой от паник и
  сбором ошибок.
- [`safe/waitgroup`](waitgroup/READMEru.md): `sync.WaitGroup`, который не
  ломается от неправильного использования и умеет ждать с таймаутом.

## Возможности

- `*PanicError{Value, Stack}` работает с `errors.As`. `panic(err)`
  разворачивается в `err`, так что работает и `errors.Is`.
- Стек снимается **один раз**, в кадре, который перехватил панику, поэтому в
  нём видна функция, где она произошла.
- Сменный `RecoverFunc`:
  - `DefaultRecover` пишет в `slog.Default()`;
  - `IgnoreRecover` молчит.
- Без паники вызов стоит ~7 нс и не делает ни одной аллокации.

## API

| Функция | Назначение |
|---|---|
| `Go(ctx, fn, recoverFn) <-chan error` | Запустить `fn` в горутине; в канал придёт её ошибка или `*PanicError` |
| `Call(fn, recoverFn) error` | Выполнить `func() error` на месте, перехватив панику |
| `CallCtx(ctx, fn, recoverFn) error` | То же для `func(context.Context) error` |
| `Recover(&err, recoverFn)` | Вызвать через `defer` в своей функции, чтобы превратить панику в `err` |
| `Func(fn, recoverFn)`, `CtxFunc(fn, recoverFn)` | Обернуть функцию так, чтобы её вызов никогда не паниковал |
| `NewPanicError(v)` | Собрать `*PanicError` из значения, полученного через `recover` |
| `DefaultRecover`, `IgnoreRecover` | Готовые `RecoverFunc` (`nil` означает `DefaultRecover`) |

## Использование

```go
// Фоновый воркер, который не должен ронять сервис.
errc := safe.Go(ctx, func(ctx context.Context) error {
	return consumer.Run(ctx)
}, nil) // nil — паники пишутся в slog.Default()

if err := <-errc; err != nil {
	var pe *safe.PanicError
	if errors.As(err, &pe) {
		metrics.Panics.Inc()
	}
}

// Внутри своей функции.
func (h *Handler) process(msg Message) (err error) {
	defer safe.Recover(&err, func(p *safe.PanicError) {
		h.log.Error("panic", "value", p.Value, "stack", string(p.Stack))
	})
	return h.handle(msg)
}
```

## Производительность

Intel Core Ultra 5 225H, Go 1.27:

| Бенчмарк | нс/оп | Б/оп | аллокаций/оп |
|---|---:|---:|---:|
| `Call` без паники | 6,6 | 0 | 0 |
| `Call` с паникой (снятие стека) | 10 100 | 3120 | 3 |
| `Go` + чтение из канала | 446 | 176 | 3 |

## Миграция (с 1.3.x)

| Было | Стало |
|---|---|
| `RecoverFunc func(r any)` | `RecoverFunc func(p *PanicError)`: обработчик получает значение **и** стек |
| Ошибка паники — `fmt.Errorf("panic: %v\n<стек>")`, строка, которую нельзя разобрать | `*PanicError`. `Error()` возвращает `"panic: <значение>"`, стек лежит в поле `Stack` |
| Стек снимался дважды (в обработчике и в ошибке) | Снимается один раз |
| `SafeGo`, `SafeFunc`, `SafeCtxFunc` | Переименованы в `Go`, `Func`, `CtxFunc`; старые имена оставлены как устаревшие синонимы |
| не было | `Call`, `CallCtx`, `Recover`, `IgnoreRecover`, `NewPanicError` |
