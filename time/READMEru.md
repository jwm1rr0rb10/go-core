# time

[English version](README.md) · [← go-core](../README.md)

Дополнения к стандартному пакету `time`: Unix-метки с заранее неизвестной
единицей измерения, кэшированная загрузка часовых поясов, фиксированные
смещения с минутами (+5:30, +5:45), компактный вывод длительностей и замер
времени операций через `log/slog`.

```bash
go get github.com/jwm1rr0rb10/go-core
```

Имя пакета совпадает со стандартным, поэтому импортируйте его с алиасом:

```go
import coretime "github.com/jwm1rr0rb10/go-core/time"
```

## Возможности

- `UnixToTime` по количеству цифр определяет, что пришло: секунды,
  миллисекунды, микросекунды или наносекунды. Отрицательные метки (до 1970
  года) тоже поддерживаются.
- `LoadLocation` кэширует `time.LoadLocation`: ≈10 нс вместо ≈4,5 мкс и
  24 аллокаций на вызов. Все функции для часовых поясов работают через него.
- `LocationByOffsetMinutes` создаёт фиксированные пояса вида `UTC+5:30` или
  `UTC-9:30`.
- `FormatDuration` выводит `850ns`, `12µs`, `340ms`, `1.50s`, `1h2m3s`;
  у отрицательных длительностей появляется минус.
- `Track` замеряет операцию и пишет результат в `slog` с типизированным уровнем.
- `CountDigitsInNumber` корректен на всём диапазоне `int64`.

## API

| Функция | Описание |
|---------|----------|
| `UnixToTime(ts) time.Time` | Метка с неизвестной единицей → `time.Time` |
| `SecondsSince(ts) int` / `SecondsUntil(ts) int` | Округлённые секунды с метки / до метки |
| `UnixMilli() int64` | Текущее Unix-время в миллисекундах |
| `CountDigitsInNumber(n) int` | Количество десятичных цифр без учёта знака |
| `FormatDuration(d) string` | Компактная длительность в одной единице |
| `Track(logger, level, name) func() time.Duration` | Начать замер; вызов результата пишет лог и возвращает длительность |
| `LoadLocation(name)` | `time.LoadLocation` с кэшем |
| `ConvertToTimezone(t, tz)` / `NowInTimezone(tz)` | Время в IANA-поясе |
| `ParseInTimezone(value, layout, tz)` | `time.ParseInLocation` по имени пояса |
| `FormatWithTimezone(t, layout, tz)` | Перевести в пояс и отформатировать |
| `LocationByOffset(hours)` | Фиксированный пояс `UTC±H`, часы в [-12, 14] |
| `LocationByOffsetMinutes(minutes)` | Фиксированный пояс `UTC±H[:MM]`, минуты в [-720, 840] |
| `TimeTrack(start, name, level...)` | **Устарела**, используйте `Track` |

## Примеры

```go
// Метки от разных источников.
t1 := coretime.UnixToTime(1_767_225_600)     // секунды
t2 := coretime.UnixToTime(1_767_225_600_000) // миллисекунды
fmt.Println(t1.Equal(t2)) // true

// Часовые пояса: загружаются один раз, дальше берутся из кэша.
s, err := coretime.FormatWithTimezone(time.Now(), time.DateTime, "Asia/Tokyo")

// Смещения с минутами.
nepal, _ := coretime.LocationByOffsetMinutes(5*60 + 45) // UTC+5:45

// Замер операции.
func loadUsers(ctx context.Context) error {
	defer coretime.Track(logger, slog.LevelDebug, "load users")()
	// ...
}
```

## Конкурентность и производительность

Все функции потокобезопасны. Замеры на тестовой машине:

| Бенчмарк | нс/оп | Б/оп | аллокаций/оп |
|----------|------:|-----:|-------------:|
| `time.LoadLocation("Europe/Moscow")` | 4493 | 3664 | 24 |
| `coretime.LoadLocation("Europe/Moscow")` | 10 | 0 | 0 |
| `UnixToTime` | 14 | 0 | 0 |
| `FormatDuration(1.5s)` | 64 | 16 | 2 |

В кэш попадают только успешно загруженные пояса, поэтому его размер ограничен
базой tz, даже если имена поясов приходят от пользователя.

`UnixToTime` работает эвристически: маленькое значение в мелких единицах
(например, 5 000 мс от начала эпохи) будет прочитано как секунды. Если единица
известна заранее, используйте `time.Unix`, `time.UnixMilli` или
`time.UnixMicro`.

## Миграция

- `CountDigitsInNumber(math.MinInt64)` теперь возвращает 19, а не мусор из-за
  переполнения.
- `UnixToTime` для отрицательных меток в мс/мкс/нс выбирает единицу по модулю
  значения.
- `FormatDuration` для отрицательных длительностей возвращает `-1.50s`, а не
  `-1500000000ns`.
- Ошибки часовых поясов теперь оборачивают исходную ошибку `time.LoadLocation`
  (сообщение по-прежнему начинается с `timeutil: invalid timezone "<имя>"`).
- `TimeTrack` устарела, вместо неё `Track`: пишет через `slog` и принимает
  `slog.Level` вместо строки.
