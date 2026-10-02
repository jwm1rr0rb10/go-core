# validator

[English version](README.md) · [← go-core](../README.md)

Валидация входных данных: два стиля и один тип ошибки. Можно проверять
структуры по тегам (`Engine` поверх
[go-playground/validator](https://github.com/go-playground/validator)) или
собирать проверки отдельных значений из маленьких блоков. Любая ошибка — это
`ValidationError` с мапой «поле → сообщение», которую можно сразу отдать
клиенту API.

```bash
go get github.com/jwm1rr0rb10/go-core
```

```go
import "github.com/jwm1rr0rb10/go-core/validator"
```

## Возможности

- `Engine` потокобезопасен и кэширует метаданные структур: создайте его один
  раз при старте и переиспользуйте. Глобальное состояние не обязательно.
- Встроенный тег `date` с настраиваемым форматом; свои теги — через
  `WithValidation`.
- `WithJSONFieldNames()` выдаёт в ошибках `email` вместо `Email`; вложенные
  поля называются по пути (`address.city`).
- `ChainValidator` останавливается на первой ошибке, `CollectAll` собирает все
  невалидные поля сразу.
- Проверки значений: `EmailValidator` (только «голый» адрес, без отображаемого
  имени), `PhoneValidator` (без аллокаций), `UUIDValidator` (только
  каноническая форма), `TimeValidator`.
- Текст ошибки детерминирован: поля отсортированы по имени.
- Ошибки, не связанные с валидацией (например, в `Struct` передали не
  структуру), возвращаются как есть — так плохие данные легко отличить от
  ошибки программиста.

## API

| Элемент | Описание |
|---------|----------|
| `NewEngine(opts...) (*Engine, error)` | Создать движок |
| `WithDateLayout(layout)` | Формат для тега `date` (по умолчанию `time.DateOnly`) |
| `WithJSONFieldNames()` | Имена полей из json-тегов |
| `WithValidation(tag, fn)` | Зарегистрировать свой тег |
| `(*Engine).Struct(s) error` | Проверить структуру |
| `(*Engine).Var(field, value, tag) error` | Проверить одно значение по выражению тегов |
| `(*Engine).StructValidator(s) Validator` | `Struct` в виде `Validator` |
| `Default()` / `SetDefault(e)` | Движок уровня пакета (создаётся лениво) |
| `New(dateLayout) error` | Перенастроить движок уровня пакета |
| `StructValidator(s) Validator` | Проверка движком уровня пакета |
| `EmailValidator`, `PhoneValidator`, `UUIDValidator(field, value)` | Проверки значений |
| `NewTimeValidator(field, value, layout)` | Проверка разбора времени; `Value()` / `Parsed()` возвращают результат |
| `ChainValidator(vs...)` / `CollectAll(vs...)` | Первая ошибка / все ошибки вместе |
| `ValidatorFunc` | Обёртка над `func() error` |
| `ValidationError`, `ErrorFields` | Тип ошибки: `Fields map[string]string` |
| `AsValidationError(err)` | Достать `ValidationError` (значение или указатель) из цепочки |

## Примеры

```go
type CreateUser struct {
	Email    string  `json:"email" validate:"required,email"`
	Birthday string  `json:"birthday" validate:"required,date"`
	Address  Address `json:"address"`
}

var engine, _ = validator.NewEngine(validator.WithJSONFieldNames())

func handle(w http.ResponseWriter, req CreateUser) {
	if err := engine.Struct(req); err != nil {
		if vErr, ok := validator.AsValidationError(err); ok {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(vErr.Fields) // {"email": "...", "address.city": "..."}
			return
		}
		panic(err) // передали не структуру — ошибка в коде
	}
}
```

Комбинирование проверок:

```go
err := validator.CollectAll(
	validator.EmailValidator("email", in.Email),
	validator.PhoneValidator("phone", in.Phone),
	validator.UUIDValidator("id", in.ID),
).Validate()
// validation failed: email: must be a valid email address; phone: must be a valid phone number
```

## Производительность

Замеры на тестовой машине:

| Бенчмарк | нс/оп | Б/оп | аллокаций/оп |
|----------|------:|-----:|-------------:|
| `Engine.Struct`, валидная структура (4 поля, вложенная) | 771 | 97 | 5 |
| `Engine.Struct`, 4 ошибки | 1918 | 1657 | 32 |
| `EmailValidator` | 230 | 96 | 5 |
| `PhoneValidator` | 22 | 0 | 0 (раньше regexp + builder: 188 нс, 2 аллокации) |

## Миграция

- `StructValidator` больше не паникует, если не вызвали `New`: движок по
  умолчанию с тегом `date` (`2006-01-02`) создаётся лениво. `New` можно
  вызывать повторно, чтобы сменить формат (раньше учитывался только первый
  вызов).
- Движки создаются с `WithRequiredStructEnabled`, поэтому `required` на поле
  типа «структура» теперь не пропускает пустую структуру (так по умолчанию
  ведёт себя go-playground v11).
- Вложенные поля называются по пути (`Address.City`), а не только по имени
  листа (`City`). Имена полей верхнего уровня не изменились.
- В сообщение добавляется параметр тега, если он есть:
  `failed on the 'gte' tag (18)`.
- `ValidationError.Error()` теперь выглядит как
  `validation failed: a: msg; b: msg` вместо `map[a:msg b:msg]`.
- `EmailValidator` отклоняет `Name <a@b.c>`, пробелы по краям, комментарии и
  адреса длиннее 254 символов.
- `UUIDValidator` принимает только каноническую 36-символьную форму (без
  фигурных скобок, префикса `urn:uuid:` и 32-символьной записи без дефисов).
- `TimeValidator.Value()` разбирает значение сам, если `Validate` ещё не
  вызывали; `Parsed()` сообщает, валидно ли оно.
- `ChainValidator` пропускает `nil`-валидаторы вместо паники.
