# safe/errorgroup

[English version](README.md) · [← safe](../READMEru.md) · [← go-core](../../READMEru.md)

Замена `golang.org/x/sync/errgroup` без переделки кода. Паника в одной задаче
становится ошибкой, а не роняет процесс, и вы сами выбираете, получать первую
ошибку или все сразу.

```go
import "github.com/jwm1rr0rb10/go-core/safe/errorgroup"
```

## Возможности

- Тот же интерфейс, что у `errgroup`: `Go`, `TryGo`, `SetLimit`, `Wait`,
  `WithContext`; нулевое значение тоже пригодно к работе.
- **Паники** перехватываются один раз, передаются в `safe.RecoverFunc` и
  записываются как `*safe.PanicError`.
- **Первая ошибка** (по умолчанию) или **все ошибки**, объединённые через
  `errors.Join` (`WithCollectAll`). `errors.Is` / `errors.As` видят каждую.
  `Errors()` всегда возвращает полный список.
- **Отмена:** контекст из `WithContext` отменяется на первой ошибке, а
  `context.Cause(ctx)` возвращает эту ошибку. С `WithContinueOnError`
  контекст живёт до возврата из `Wait`, и остальные задачи не прерываются.
- Не зависит от `golang.org/x/sync`.

## API

| Идентификатор | Назначение |
|---|---|
| `WithContext(ctx, opts...) (*Group, context.Context)` | Группа с производным контекстом |
| `New(opts...) *Group` / `var g Group` | Группа без контекста (задачи получают `context.Background()`) |
| `(*Group).Go(fn)` / `TryGo(fn) bool` | Запустить задачу (блокируясь / не блокируясь, когда лимит исчерпан) |
| `(*Group).SetLimit(n)` / `WithLimit(n)` | Максимум одновременных задач (`n < 0` — без лимита) |
| `(*Group).Wait() error` | Дождаться всех задач; вернуть первую ошибку или все вместе |
| `(*Group).Errors() []error` | Все записанные ошибки в порядке завершения задач |
| `WithRecover(fn)` | Обработчик паник (`nil` = `safe.DefaultRecover`, `safe.IgnoreRecover` = молча) |
| `WithCollectAll()` | `Wait` возвращает `errors.Join` всех ошибок |
| `WithContinueOnError()` | Не отменять контекст при ошибке |

## Использование

```go
g, ctx := errorgroup.WithContext(ctx, errorgroup.WithLimit(8))
for _, id := range ids {
	g.Go(func(ctx context.Context) error {
		return sync(ctx, id) // паника здесь станет *safe.PanicError
	})
}
if err := g.Wait(); err != nil {
	return err
}

// Выполнить всё и сообщить обо всех сбоях.
g := errorgroup.New(errorgroup.WithCollectAll(), errorgroup.WithContinueOnError())
g.Go(checkDB)
g.Go(checkCache)
if err := g.Wait(); err != nil {
	for _, e := range g.Errors() {
		slog.Error("health check failed", "err", e)
	}
}
```

## Производительность

Intel Core Ultra 5 225H, Go 1.27. Группа из 8 пустых задач вместе с `Wait`
занимает 3,2 мкс/оп, 384 Б/оп, 11 аллокаций/оп. Основная стоимость — сами
горутины.

## Миграция (с 1.3.x)

| Было | Стало |
|---|---|
| `SafeGroup` | `Group` (`SafeGroup` оставлен как устаревший синоним) |
| Паника записывалась **дважды** (в `Errors()` и ещё раз через `Wait`) | Записывается один раз |
| `Wait` возвращал `fmt.Errorf("safe group errors: %v", errs)`, `errors.Is` не работал | Возвращает саму первую ошибку или `errors.Join` с `WithCollectAll` |
| Каждый вызов `Wait` дописывал ошибку в `Errors()` | `Wait` не меняет состояние, его можно вызывать повторно |
| `RecoverFunc func(r any)`, `DefaultRecover(r any)` | Синонимы `safe.RecoverFunc func(*safe.PanicError)` и `safe.DefaultRecover` |
| не было управления лимитом | `SetLimit`, `TryGo`, `WithLimit` |
| зависимость от `golang.org/x/sync` | только стандартная библиотека |
