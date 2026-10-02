# pointer

[English version](README.md) · [← go-core](../README.md)

Дженерик-хелперы для необязательных значений в виде указателей: создать
указатель из литерала, прочитать значение с запасным вариантом, сравнить,
скопировать. Пригодится для PATCH-запросов, nullable-колонок в базе и
необязательных полей конфигурации.

```bash
go get github.com/jwm1rr0rb10/go-core
```

```go
import "github.com/jwm1rr0rb10/go-core/pointer"
```

## API

| Функция | Описание |
|---------|----------|
| `ToPointer(v) *T` | Указатель на копию `v` (в Go 1.26+ то же самое делает `new(v)`) |
| `ToPointerOrNil(v) *T` | `nil` для нулевого значения, иначе указатель на копию |
| `FromPointer(p) (T, bool)` | Значение и признак, что `p` не `nil` |
| `Deref(p) T` | Значение или нулевое значение типа |
| `ValueOr(p, fallback) T` | Значение или `fallback` (`FromPointerOr` — то же самое) |
| `Coalesce(ps...) *T` | Первый ненулевой указатель |
| `Equal(a, b) bool` | Оба `nil` либо оба не `nil` и значения равны |
| `Clone(p) *T` | Поверхностная копия `*p` в новой памяти; `nil` остаётся `nil` |
| `Set(p, v) bool` | Присвоить, если `p` не `nil` |
| `Swap(a, b)` | Обменять значения; паникует на `nil` |
| `Copy(p) (*T, error)` | **Устарела**, используйте `Clone` |
| `IsNil(p) bool` | **Устарела**, пишите `p == nil` |

## Примеры

```go
type UpdateUser struct {
	Name *string `json:"name,omitempty"`
	Age  *int    `json:"age,omitempty"`
}

// В обновление попадут только непустые значения из формы.
req := UpdateUser{
	Name: pointer.ToPointerOrNil(form.Name),
	Age:  pointer.ToPointerOrNil(form.Age),
}

limit := pointer.ValueOr(cfg.Limit, 100)
env := pointer.Coalesce(flagEnv, fileEnv, pointer.ToPointer("dev"))

if !pointer.Equal(old.Name, req.Name) {
	// имя изменилось
}
```

## Замечания

- `Clone` копирует только само значение. Срезы, мапы и указатели внутри него
  остаются общими с оригиналом.
- Рефлексии нет; аллоцируют память только `ToPointer`, `ToPointerOrNil` и `Clone`, возвращающие новый указатель.

## Миграция

- `Copy` больше не использует рефлексию и не отказывает несравнимым типам
  (срезам, мапам и структурам с ними); ошибка всегда `nil`. Используйте
  `Clone` — он возвращает только указатель.
- `IsNil` помечена устаревшей.
- Новое: `ToPointerOrNil`, `Deref`, `ValueOr`, `Coalesce`, `Equal`, `Clone`.
