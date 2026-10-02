# clock

[English version](README.md) · [← go-core](../README.md)

Абстракция времени для кода, который работает с таймерами, тикерами и
таймаутами. Боевой код зависит от интерфейса `Clock` и получает настоящие часы
через `clock.New()`, а тесты подставляют `clock.Mock` — время в нём идёт только
тогда, когда это говорит тест. Никаких `time.Sleep` в тестах и плавающих
таймаутов.

```bash
go get github.com/jwm1rr0rb10/go-core
```

```go
import "github.com/jwm1rr0rb10/go-core/clock"
```

## Возможности

- Повторяет пакет `time`: `Now`, `Since`, `Until`, `Sleep`, `After`,
  `NewTimer`, `AfterFunc`, `NewTicker`, а также `SleepContext`.
- Граничные случаи ведут себя как в `time`: нулевая или отрицательная
  длительность срабатывает сразу, `NewTicker(0)` паникует, а после `Stop` и
  `Reset` в канале не остаётся устаревшего значения (семантика таймеров
  Go 1.23+).
- `Mock` — полноценные фейковые часы: `Advance` запускает все наступившие
  таймеры и тикеры строго по порядку дедлайнов, а `Now()` внутри колбэка
  возвращает дедлайн этого таймера.
- `BlockUntil(n)` ждёт, пока тестируемый код зарегистрирует `n` таймеров или
  уснёт, поэтому сдвигать время можно ровно в нужный момент.
- Всё потокобезопасно; мок не запускает фоновых горутин.

## API

| Элемент | Описание |
|---------|----------|
| `type Clock` | `Now`, `Since`, `Until`, `Sleep`, `SleepContext`, `After`, `NewTimer`, `AfterFunc`, `NewTicker` |
| `type Timer` | `C() <-chan time.Time`, `Stop() bool`, `Reset(d) bool` |
| `type Ticker` | `C() <-chan time.Time`, `Stop()`, `Reset(d)` |
| `New() Clock` | Настоящие часы на базе `time` |
| `NewMock(start) *Mock` | Фейковые часы, начинающие с `start` |
| `(*Mock).Advance(d)` | Сдвинуть вперёд и запустить наступившие таймеры (колбэки `AfterFunc` выполняются синхронно) |
| `(*Mock).Set(t)` | Перейти к `t`; вперёд — со срабатыванием таймеров, назад — только меняется `Now` |
| `(*Mock).BlockUntil(n)` / `BlockUntilContext(ctx, n)` | Дождаться `n` активных ожидающих |
| `(*Mock).Waiters()` | Число активных таймеров, тикеров и спящих горутин |

## Примеры

Боевой код принимает `Clock`:

```go
type Cache struct {
	clock clock.Clock
	ttl   time.Duration
}

func (c *Cache) expired(storedAt time.Time) bool {
	return c.clock.Since(storedAt) > c.ttl
}

cache := &Cache{clock: clock.New(), ttl: time.Minute}
```

Тест управляет временем явно:

```go
func TestWorkerRetriesAfterDelay(t *testing.T) {
	m := clock.NewMock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	done := make(chan struct{})
	go func() {
		m.Sleep(30 * time.Second) // тестируемый код ждёт здесь
		close(done)
	}()

	m.BlockUntil(1)             // горутина уже спит
	m.Advance(30 * time.Second) // и мгновенно просыпается
	<-done
}
```

Тикеры:

```go
tk := m.NewTicker(time.Minute)
defer tk.Stop()

m.Advance(time.Minute)
tick := <-tk.C() // 00:01:00
```

## Конкурентность и производительность

- Настоящие часы — значение нулевого размера; `Now()` стоит столько же, сколько
  `time.Now()` (≈33 нс, 0 аллокаций на тестовой машине).
- Мок хранит ожидающих в отсортированном срезе под одним мьютексом. Создать
  таймер, сдвинуть время и получить значение — ≈360 нс и 5 аллокаций, для
  тестов этого с запасом.
- Как и `time.Ticker`, тикер мока пропускает тики, если получатель не успевает,
  и не блокирует `Advance`.

## Миграция

Раньше интерфейс возвращал ошибки для неположительных длительностей и имел
метод `Tick`. Что изменилось:

| Было | Стало |
|------|-------|
| `After(d) (<-chan time.Time, error)` | `After(d) <-chan time.Time`; при `d <= 0` срабатывает сразу |
| `Sleep(d) error` | `Sleep(d)`; при `d <= 0` возвращается сразу. Для отмены — `SleepContext` |
| `Tick(d) (<-chan time.Time, func(), error)` | `NewTicker(d) Ticker`; вместо функции остановки — `Stop()` |
| `Mock.After` / `Mock.Sleep` сами двигали время | Блокируются до вызова `Advance` в тесте (сначала `BlockUntil`) |
| `Mock.Tick` крутил горутину в холостом цикле | `Mock.NewTicker` срабатывает только на `Advance`, без горутин |
