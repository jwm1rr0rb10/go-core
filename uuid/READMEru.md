# uuid

[← go-core](../READMEru.md) · [English version](README.md)

UUID по RFC 9562: один сравнимый тип `UUID` с быстрым разбором и форматированием,
поддержкой JSON/text/binary/SQL и генераторами без блокировок для версии 4 (случайные)
и версии 7 (упорядочены по времени, строго возрастают).

```bash
go get github.com/jwm1rr0rb10/go-core
```

```go
import "github.com/jwm1rr0rb10/go-core/uuid"
```

## Возможности

- `NewV7()` — UUID, упорядоченные по времени и **строго возрастающие**; лучший выбор для
  первичных ключей (RFC 9562 §6.2, метод 1: 12-битный счётчик, при переполнении переносится
  в метку времени, порядок сохраняется даже если системные часы отступили назад)
- `NewV4()` — случайные UUID из `crypto/rand`, без аллокаций
- `Parse` понимает канонический вид, `{в фигурных скобках}`, `urn:uuid:` и 32 hex-символа,
  регистр не важен, без аллокаций
- Реализует `encoding.TextMarshaler` / `TextAppender` / `BinaryMarshaler`, `sql.Scanner`,
  `driver.Valuer`; `NullUUID` для колонок, допускающих NULL
- `Version()`, `Variant()`, `Time()` (для v7), `Compare`, `IsZero`, `Nil`, `Max`
- `Generator` с подключаемым источником энтропии (`WithReader`, `WithFastRandom`) и часов (`WithClock`)
- Подпакеты [`db`](db/READMEru.md) и [`network`](network/READMEru.md) возвращают тот же тип;
  [`google_uuid`](google_uuid/READMEru.md) — генераторы строковых ID (google/uuid, ULID)

## API

| Функция / метод | Описание |
|---|---|
| `NewV4() UUID` | Случайный UUID, crypto/rand |
| `NewV7() UUID` | UUID по времени, монотонный, crypto/rand |
| `Parse(s) (UUID, error)` / `MustParse(s)` | Разбор текста (`ErrInvalidFormat`, `ErrInvalidLength`) |
| `FromBytes(b) (UUID, error)` | Из 16 байт |
| `Must(u, err) UUID` | Паника при ошибке — для инициализации |
| `u.String()`, `u.URN()`, `u.Bytes()` | Представления |
| `u.Version()`, `u.Variant()`, `u.Time()` | Разбор полей |
| `u.Compare(v)`, `u.IsZero()` | Сравнение (`==` тоже работает) |
| `MarshalText`/`UnmarshalText`, `MarshalBinary`/`UnmarshalBinary`, `AppendText` | интерфейсы encoding |
| `Scan`, `Value`; `NullUUID` | database/sql |
| `NewGenerator(opts...)`, `g.NewV4()`, `g.NewV7()` | Свои генераторы |
| `WithReader(io.Reader)`, `WithFastRandom()`, `WithClock(func() time.Time)` | Опции генератора |

## Примеры

```go
id := uuid.NewV7()
fmt.Println(id)                 // 0192f4c1-9b7a-7d3e-8a41-2f6c0b9e5d17
ts, _ := id.Time()              // время создания с точностью до мс

u, err := uuid.Parse("{0190A3C2-7B1E-7C3A-9F2D-4B6E8A1C0D3F}")
if err != nil { /* errors.Is(err, uuid.ErrInvalidFormat) */ }

// JSON: сериализуется строкой
type User struct {
	ID uuid.UUID `json:"id"`
}

// SQL: можно передавать параметром и сканировать
var userID uuid.UUID
_ = db.QueryRowContext(ctx, "SELECT id FROM users LIMIT 1").Scan(&userID)

// Детерминированные тесты
gen := uuid.NewGenerator(uuid.WithClock(func() time.Time { return fixed }))
a, _ := gen.NewV7()
```

## Конкурентность и производительность

Все функции и методы `Generator` безопасны для конкурентного вызова. Состояние v7 — одно
атомарное слово, обновляемое через CAS, мьютексов нет. Замеры на Intel Core Ultra 5 225H
(14 потоков), Go 1.27:

| Бенчмарк | нс/оп | аллокаций |
|---|---|---|
| `NewV4` | 44 | 0 |
| `NewV7` | 92 | 0 |
| `NewV4` параллельно | 64–70 | 0 |
| `NewV7` параллельно | ~320 | 0 |
| `String` | 51 | 1 (сама строка) |
| `Parse` | 25 | 0 |

Параллельный `NewV7` упирается в конкуренцию за общий монотонный счётчик — это плата за
глобальный порядок. Если нужны десятки миллионов ID в секунду, заведите по `Generator` на
воркер (порядок будет внутри воркера) или используйте `network.NewV4`.

## Безопасность

`NewV4`, `NewV7` и генераторы по умолчанию используют `crypto/rand`: 62 случайных бита v7
и 122 бита v4 непредсказуемы. Учтите, что v7 раскрывает время создания. `WithFastRandom` и
пакет `network` **не подходят** для секретов и токенов.

## Миграция

| Было | Стало |
|---|---|
| `uuid.NewV4() (UUID, error)` | `uuid.NewV4() UUID` (crypto/rand не может вернуть ошибку начиная с Go 1.24) |
| `uuid.NewV7(r *rand.ChaCha8) (UUID, error)` | `uuid.NewV7() UUID`; своя энтропия: `uuid.NewGenerator(uuid.WithReader(r)).NewV7()` |
| `db.UUID`, `network.UUID`, `uuid.UUID` — три разных типа | `db.UUID` и `network.UUID` — алиасы `uuid.UUID` |
| не было разбора и форматирования | `Parse`, `String`, интерфейсы text/binary/SQL |
| v7 не был монотонным, 2 случайных байта терялись | монотонность по RFC 9562: 12-битный счётчик + 62 случайных бита |
