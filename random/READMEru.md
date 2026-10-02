# random

[← go-core](../READMEru.md) · [English version](README.md)

Случайные числа, строки, даты, IP-адреса и выбор из вариантов — в двух чётко разделённых
вариантах: **быстрые** функции поверх `math/rand/v2` для тестов, выборок, джиттера и нагрузочного
тестирования и **криптостойкие** поверх `crypto/rand` для токенов, паролей и секретов.

```go
import "github.com/jwm1rr0rb10/go-core/random"
```

## Возможности

- Потокобезопасно по умолчанию: передайте `nil` вместо источника — будут использованы глобальные
  генераторы `math/rand/v2`, работающие без блокировок
- Воспроизводимо, когда нужно: передайте `*rand.Rand` с фиксированным сидом
- Диапазоны работают на всей числовой оси (`RandInt64(nil, math.MinInt64, math.MaxInt64)` не паникует)
- Строки без смещения распределения (rejection sampling), одна аллокация на строку, 8 символов
  из одного 64-битного числа
- Семейство на `crypto/rand`: `SecureInt`, `SecureString`, `SecureToken`, `SecureText`, `Secure()`
- Помощники для `netip.Addr`: случайные IPv4/IPv6, адрес внутри CIDR-префикса
- Обобщённый `Pick[T]`
- Сигнальные ошибки (sentinel): `ErrInvalidRange`, `ErrNegativeCount`, `ErrEmpty`

## API

| Быстрые (`r *rand.Rand` может быть nil) | Криптостойкие (crypto/rand) | Описание |
|---|---|---|
| `RandInt(r, min, max)` / `RandInt64` | `SecureInt(min, max)` / `SecureInt64` | Целое в `[min, max)` |
| `RandFloat64(r, min, max)` | — | Дробное в `[min, max)` |
| `RandString(r, n, set)` | `SecureString(n, set)` | `n` байт из `set` (по умолчанию A–Z a–z 0–9) |
| — | `SecureToken(n)` | `n` случайных байт в hex |
| — | `SecureText()` | 26 символов base32, 128 бит (`crypto/rand.Text`) |
| `Pick(r, values...)` | `Pick(random.Secure(), ...)` | Равновероятный выбор |
| `RandomBool(r)` | — | Подбрасывание монетки |
| `RandomTime(r, min, max)` | — | Момент в `[min, max]` с точностью до наносекунд |
| `RandomDate(r, min, max)` | — | Момент в `[min, max]` со сдвигом на целые секунды |
| `RandIPv4(r)`, `RandIPv6(r)` | — | Случайный `netip.Addr` |
| `RandAddrInPrefix(r, prefix)` | — | Адрес внутри, например, `10.0.0.0/8` |
| — | `Secure() *rand.Rand` | Потокобезопасный `*rand.Rand` на crypto/rand для любой функции выше |

Устаревшие (оставлены для совместимости): `RandomCase` → `Pick`, `RandIP` → `RandIPv4`.

## Примеры

```go
// Быстро и потокобезопасно
n, _ := random.RandInt(nil, 1, 7)
jitter := time.Duration(n) * 10 * time.Millisecond

// Воспроизводимые тестовые данные
r := rand.New(rand.NewPCG(42, 42))
name, _ := random.RandString(r, 8, nil)

// Секреты
apiKey, _ := random.SecureToken(32)      // 64 hex-символа
password, _ := random.SecureString(20, []byte("abcdefghjkmnpqrstuvwxyz23456789"))
sessionID := random.SecureText()

// Тестовые сетевые данные
ip, _ := random.RandAddrInPrefix(nil, netip.MustParsePrefix("192.168.0.0/16"))
```

## Конкурентность и производительность

Функции с источником `nil` и все `Secure*` безопасны для конкурентного вызова. Созданный вами
`*rand.Rand` — **нет**: заведите отдельный на каждую горутину. Исключение — `Secure()`: у него
нет состояния, его можно разделять.

Intel Core Ultra 5 225H (14 потоков), Go 1.27:

| Бенчмарк | нс/оп | аллокаций |
|---|---|---|
| `RandInt` | 7,3 | 0 |
| `RandInt` параллельно | 0,9 | 0 |
| `RandString(32)` | 124 | 1 |
| `RandString(32)` параллельно | 20 | 1 |
| `SecureString(32)` | 229 | 1 |
| `SecureInt` | 48 | 0 |

## Безопасность

Для всего, что злоумышленник не должен предсказать, подходят только функции `Secure*` и
`Secure()`. Быстрые функции статистически хороши, но не криптостойки.

## Миграция

- **Исправлена потокобезопасность**: общий источник пакета был `rand.New(rand.NewPCG(...))`,
  который нельзя использовать из нескольких горутин; теперь `nil` означает потокобезопасные
  глобальные генераторы.
- `RandInt`/`RandInt64` больше не паникуют при переполнении `max-min`.
- `RandFloat64` отвергает границы NaN/±Inf.
- Ошибки стали сигнальными значениями (`ErrInvalidRange`, `ErrNegativeCount`, `ErrEmpty`);
  проверяйте через `errors.Is`.
- При том же сиде вывод отличается от прежней версии (`RandString` берёт 8 символов из одного
  64-битного числа).
- `RandomCase` и `RandIP` помечены устаревшими, используйте `Pick` и `RandIPv4`.
- Новое: `RandomTime`, `RandIPv6`, `RandAddrInPrefix`, `Secure*`, `Pick`.
