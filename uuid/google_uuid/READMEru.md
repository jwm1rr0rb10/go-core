# uuid/google_uuid

[← go-core](../../READMEru.md) · [← uuid](../READMEru.md) · [English version](README.md)

Генераторы строковых ID за одним интерфейсом `IDGenerator`: UUID v4 и v7 на основе
[google/uuid](https://github.com/google/uuid) и монотонные [ULID](https://github.com/ulid/spec)
на основе [oklog/ulid](https://github.com/oklog/ulid). Пригодится, когда компонент должен
зависеть от «чего-то, что выдаёт строковые ID», а конкретный формат выбирается при сборке
приложения.

Имя каталога по историческим причинам содержит подчёркивание, поэтому импортируйте пакет с
алиасом:

```go
import idgen "github.com/jwm1rr0rb10/go-core/uuid/google_uuid"
```

## API

| Идентификатор | Описание |
|---|---|
| `type IDGenerator interface { GenerateID() string }` | Общий контракт |
| `NewGoogleUUIDGenerator()` | Строки UUIDv4 |
| `NewGoogleUUIDv7Generator()` | Строки UUIDv7 |
| `NewULIDGenerator()` | Монотонные ULID, 26 символов, сортируются лексикографически |
| `(*ULIDGenerator).New() ulid.ULID` | Типизированный ULID |

Все генераторы безопасны для конкурентного вызова и не возвращают ошибок.

## Пример

```go
type OrderService struct{ ids idgen.IDGenerator }

svc := OrderService{ids: idgen.NewULIDGenerator()}
id := svc.ids.GenerateID() // "01J9Z3K8V6Q2N4R7T1W5X8Y0C3"
```

## Подробности про ULID

Энтропия — ChaCha8 (криптостойкий ГПСЧ) с сидом из `crypto/rand`, обёрнутый в
`ulid.Monotonic`. ID одного генератора строго возрастают, даже внутри одной миллисекунды и
даже если системные часы отступили назад; если 80 бит энтропии на миллисекунду исчерпаны,
метка времени сдвигается на 1 мс вперёд. Вызовы сериализуются мьютексом.

## Производительность

Intel Core Ultra 5 225H, Go 1.27:

| Бенчмарк | нс/оп | Б/оп | аллокаций |
|---|---|---|---|
| ULID | 128 | 48 | 2 |
| ULID параллельно (14 горутин) | 326 | 48 | 2 |
| google UUIDv4 | 129 | 64 | 2 |
| google UUIDv7 | 178 | 64 | 2 |

Если нужны типизированные значения или ноль аллокаций, используйте корневой пакет
[`uuid`](../READMEru.md).

## Миграция

- **Исправлена гонка данных** в `ULIDGenerator`: состояние ChaCha8 использовалось из
  нескольких горутин без блокировки.
- `NewULIDGenerator()` теперь возвращает `*ULIDGenerator` без ошибки.
- `(*ULIDGenerator).GenerateID()` теперь возвращает `string` без ошибки — и `ULIDGenerator`
  наконец реализует `IDGenerator`.
- ULID теперь монотонные.
- Новое: `NewGoogleUUIDv7Generator`, `(*ULIDGenerator).New`.
