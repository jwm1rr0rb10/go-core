# bytes

[English version](README.md) · [← go-core](../README.md)

Преобразования между JSON и Go-значениями: дженерик `Marshal`/`Unmarshal` с
опциями декодирования и хелперы для произвольных документов
`map[string]any`, которые умеют не терять точность больших целых чисел.

```bash
go get github.com/jwm1rr0rb10/go-core
```

Имя пакета совпадает со стандартным, поэтому импортируйте его с алиасом:

```go
import corebytes "github.com/jwm1rr0rb10/go-core/bytes"
```

## API

| Элемент | Описание |
|---------|----------|
| `Marshal[T](v) ([]byte, error)` | `json.Marshal` с типизированным параметром |
| `Unmarshal[T](data, opts...) (T, error)` | Декодировать в новое значение `T` |
| `MapToJSON(m) ([]byte, error)` | Закодировать мапу; `nil` → `null` |
| `JSONToMap(data, opts...) (map[string]any, error)` | Декодировать объект; `null` → `nil`-мапа |
| `UseNumber()` | Числа внутри `any` становятся `json.Number`, а не `float64` |
| `DisallowUnknownFields()` | Ошибка, если в JSON есть ключ без соответствующего поля структуры |
| `ErrTrailingData` | Во входных данных больше одного JSON-значения |
| `MapToByteArr`, `ByteArrToMap` | **Устаревшие** имена `MapToJSON` и `JSONToMap` |

## Примеры

```go
type Event struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
}

ev, err := corebytes.Unmarshal[Event](body, corebytes.DisallowUnknownFields())

// Большие идентификаторы не искажаются при декодировании в мапу.
m, err := corebytes.JSONToMap([]byte(`{"order_id": 9007199254740993}`), corebytes.UseNumber())
id, _ := m["order_id"].(json.Number).Int64() // 9007199254740993, а не ...992
```

## Замечания

- Без опций декодирование — ровно `json.Unmarshal`. С опциями используется
  `json.Decoder`, но данные после первого значения всё равно отклоняются,
  так что `{"a":1} garbage` даёт ошибку в обоих режимах.
- Бенчмарки на небольшом документе: `JSONToMap` ≈1,7 мкс / 19 аллокаций,
  с `UseNumber` ≈2,3 мкс / 27 аллокаций.

## Миграция

- `ByteArrToMap(string)` → `JSONToMap([]byte, ...DecodeOption)`.
- `MapToByteArr` → `MapToJSON`.
- Старые имена продолжают работать и помечены устаревшими.
