# array

[← go-core](../READMEru.md) · [English version](README.md)

Обобщённые функции для поиска, преобразования, объединения и сортировки слайсов, а также
ленивые варианты на `iter.Seq`. Функции никогда не меняют входные данные, делегируют
`slices`/`maps`, если в стандартной библиотеке уже есть нужная операция, и обходятся без
рефлексии и лишних аллокаций.

```go
import "github.com/jwm1rr0rb10/go-core/array"
```

## Соглашения

- **Входной слайс не меняется.** Результат — новый слайс, кроме `Chunk`, `Split`, `Window` и
  `PartitionBy`: они возвращают подслайсы, разделяющие память со входом (без копирования).
  Ёмкость таких подслайсов обрезана, поэтому `append` к ним не затрёт соседние данные, но
  присваивание элементу изменит исходный слайс.
- Пустой результат — не `nil` (`[]T{}`), поэтому в JSON он превращается в `[]`, а не в `null`.
- Ниже указаны аналоги из стандартной библиотеки; в новом коде используйте их напрямую, если
  семантика подходит.

## API

| Группа | Функции | Аналог в stdlib |
|---|---|---|
| Преобразование | `Map`, `MapIndex`, `FilterMap`, `FlatMap`, `Filter`, `Reduce`, `Sum`, `SumBy` | — |
| Дубликаты / очистка | `Uniq`, `UniqBy`, `Compact` (убирает нулевые значения), `CompactBy`, `RemoveAll` | `slices.Compact` убирает только *соседние* дубликаты |
| Поиск | `Contains`, `IndexOf`, `Find`, `FindLast`, `Count`, `Every`, `Some`, `None` | `slices.Contains`, `slices.Index`, `slices.IndexFunc`, `slices.ContainsFunc` |
| Сравнение | `AreIdentical` (равенство мультимножеств), `DistinctCount` | `slices.Equal` (с учётом порядка) |
| Изменение (копия) | `RemoveByValue`, `RemoveByIndex`, `Take`, `Drop`, `DropRight`, `TakeWhile`, `DropWhile`, `Fill` | `slices.Delete` (меняет исходный) |
| Объединение | `Concat`, `Flatten`, `Interleave`, `Zip`, `Unzip`, `ZipWith`, `ZipWithIndex` | `slices.Concat` |
| Группировка | `GroupBy`, `KeyBy`, `Partition`, `PartitionBy` | — |
| Разбиение | `Chunk`, `Split`, `Window` | `slices.Chunk` (итератор) |
| Порядок (копия) | `Sort`, `SortStable`, `SortBy`, `Reverse`, `Rotate`, `Shuffle`, `ShuffleWith`, `MinMax` | `slices.SortFunc`, `slices.Reverse`, `slices.Min/Max` (меняют исходный / два прохода) |
| Строки | `Join` (как `%v`), `JoinString` (`fmt.Stringer`) | `strings.Join` для `[]string` |
| Рефлексия | `FlattenDeep` (любая вложенность, защита от циклов) | — |
| Итераторы | `MapSeq`, `FilterSeq`, `UniqSeq`, `TakeSeq` | `slices.Values`, `slices.Collect` |
| Типы | `Pair[T, U]`, `Number` | — |

Ошибки: `ErrInvalidSize`, `ErrIndexOutOfRange`, `ErrNegativeCount`, `ErrEmpty`.

## Примеры

```go
squares := array.Map([]int{1, 2, 3}, func(v int) int { return v * v }) // [1 4 9]

even, odd := array.Partition([]int{1, 2, 3, 4, 5}, func(v int) bool { return v%2 == 0 })

byAge := array.SortBy(users, func(u User) int { return u.Age }) // стабильная сортировка
byName := array.Sort(users, func(a, b User) int { return cmp.Compare(a.Name, b.Name) })

for _, batch := range array.Chunk(ids, 500) {
	_ = repo.DeleteBatch(ctx, batch)
}

csv := array.Join([]any{1, "two", 3.5}, ",") // "1,two,3.5"

// Ленивый конвейер без промежуточных слайсов
top := slices.Collect(array.TakeSeq(
	array.FilterSeq(slices.Values(events), func(e Event) bool { return e.Critical }), 10))
```

## Производительность

Intel Core Ultra 5 225H, Go 1.27, 10 000 `int`, если не указано иное:

| Бенчмарк | нс/оп | Б/оп | аллокаций |
|---|---|---|---|
| `Map` | 19 400 | 81 920 | 1 |
| `Filter` | 23 900 | 81 920 | 1 |
| `Partition` | 42 100 | 81 920 | 1 (обе половины в одном буфере) |
| `Uniq` (1 000 различных) | 194 000 | 377 475 | 34 |
| `Uniq` (10 элементов, без map) | 128 | 160 | 2 |
| `Chunk(100)` | 920 | 2 688 | 1 |
| `Join` 1 000 int | 14 150 (было 57 300) | 9 472 | 2 (было 1 735) |
| `Join` 1 000 строк | 8 190 (было 65 000) | 6 144 | 1 (было 2 001) |

Все функции чистые: их можно вызывать конкурентно, если входной слайс в это время никто не
изменяет.

## Миграция

- `Sort(s, less func(a, b T) bool)` → `Sort(s, cmp func(a, b T) int)` (быстрее, как
  `slices.SortFunc`). Быстрая замена: `array.Sort(s, cmp.Compare[int])`; старое поведение
  доступно под устаревшим именем `SortLess`.
- Ёмкость результатов `Chunk`, `Split`, `Window` теперь обрезана: `append` к чанку больше не
  затирает следующий чанк и исходный слайс.
- Группы `PartitionBy` теперь разделяют память со входом, а не копируются.
- `Fill` при `end < start` больше не заполняет хвост слайса — не заполняет ничего.
- `FlattenDeep` больше не зависает на слайсах, ссылающихся сами на себя, и ограничивает
  глубину 512 уровнями.
- Ошибки стали сигнальными значениями; проверяйте через `errors.Is`.
- Новое: `MapIndex`, `FilterMap`, `FlatMap`, `UniqBy`, `RemoveAll`, `FindLast`, `Count`, `None`,
  `Unzip`, `KeyBy`, `SortStable`, `SortBy`, `ShuffleWith`, `Sum`, `SumBy`, `MapSeq`,
  `FilterSeq`, `UniqSeq`, `TakeSeq`.
