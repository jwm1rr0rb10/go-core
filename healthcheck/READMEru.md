# healthcheck

[← go-core](../README.md) · [English version](README.md)

Сервер `grpc.health.v1`, статус которого зависит от состояния ваших
зависимостей. Состояние можно сообщать самому (`SetStatus`) или зарегистрировать
проверки, которые опрашиваются по интервалу (`AddChecker`). Общий статус —
`SERVING`, только пока здоровы все зависимости; отдельные gRPC-сервисы можно
привязать к части зависимостей.

```go
import "github.com/jwm1rr0rb10/go-core/healthcheck"
```

## Возможности

- Готовый `grpc_health_v1.HealthServer` (встраивает `*health.Server`: `Check`, `List`, `Watch`)
- Два способа сообщать о зависимостях: push (`SetStatus`) и pull (`AddChecker`)
- Изменения статуса применяются сразу, а не на следующем тике
- Статус по сервисам через `BindService`
- Проверки идут параллельно, у каждой свой таймаут; паника перехватывается и
  считается ошибкой
- `Start` и `Stop` идемпотентны; `Shutdown` переводит всё в `NOT_SERVING` для
  плавного вывода из балансировки, `Resume` возвращает обратно
- Подписчики `Watch` получают уведомления только при реальной смене статуса
- Логирование переходов через `slog` (по умолчанию выключено)
- Без гонок: всё состояние под одним мьютексом, кроме цикла опроса горутин нет

## Обзор API

| Идентификатор | Назначение |
|---|---|
| `NewGRPCHealthServer(...Option)` | Создать сервер (`SERVING`, пока нет зависимостей) |
| `WithInterval`, `WithCheckTimeout`, `WithLogger` | Опции (по умолчанию 10 с, 2 с, без логов) |
| `SetStatus(dep, healthy)` | Сообщить статус зависимости |
| `AddChecker(dep, CheckFunc)` | Зарегистрировать проверку (до первого успеха зависимость нездорова) |
| `RemoveDependency(dep)` | Удалить зависимость и её проверку |
| `BindService(service, deps...)` | Статус сервиса = все перечисленные зависимости здоровы |
| `Start(ctx)` / `Stop()` | Запустить / остановить цикл опроса |
| `CheckNow(ctx)` | Синхронно выполнить все проверки один раз |
| `Shutdown()` / `Resume()` | Принудительно `NOT_SERVING` на время остановки / вернуть |
| `Statuses()`, `Errors()` | Снимки состояния для логов и debug-эндпоинтов |
| `HealthCheck(ctx, interval)` | Устарел: используйте `WithInterval` + `Start` |

## Использование

```go
hs := healthcheck.NewGRPCHealthServer(
	healthcheck.WithInterval(5*time.Second),
	healthcheck.WithCheckTimeout(time.Second),
	healthcheck.WithLogger(slog.Default()),
)

hs.AddChecker("postgres", db.PingContext)
hs.AddChecker("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() })
hs.BindService("orders.Orders", "postgres")

nc.SetDisconnectErrHandler(func(*nats.Conn, error) { hs.SetStatus("nats", false) })
nc.SetReconnectHandler(func(*nats.Conn) { hs.SetStatus("nats", true) })

srv := grpc.NewServer()
grpc_health_v1.RegisterHealthServer(srv, hs)

hs.Start(ctx)

// плавная остановка
hs.Shutdown()                 // балансировщик перестаёт слать новый трафик
time.Sleep(drainDelay)
srv.GracefulStop()
```

Проверка из консоли: `grpc_health_probe -addr=:50051 -service=orders.Orders`.

## Производительность и конкурентность

Замеры на Intel Core Ultra 5 225H, Go 1.27.1:

| Бенчмарк | нс/оп | Б/оп | аллокаций/оп |
|---|---:|---:|---:|
| SetStatus (20 зависимостей, статус не меняется) | 169 | 0 | 0 |
| Check | 52 | 48 | 1 |

Все методы потокобезопасны. `SetStatus` стоит O(зависимости + привязанные
сервисы) и обращается к `health.Server`, только когда статус действительно
меняется. `Check` / `Watch` обслуживает встроенный `health.Server`, блокировка
этого пакета там не участвует.

## Миграция (с 1.3.x)

- `NewGRPCHealthServer()` теперь принимает опции; старые вызовы компилируются как раньше.
- `SetStatus` меняет отдаваемый статус **сразу**; раньше изменение было видно
  только после следующего тика.
- `HealthCheck(ctx, interval)` помечен устаревшим, но работает. Повторный вызов
  больше не запускает вторую горутину.
- Без зависимостей сервер сразу отдаёт `SERVING` (раньше `SERVING` выставлял
  `health.NewServer`, и до первого тика статус не пересчитывался).
- Исправлено: гонка данных (флаг статуса писался под read-lock) и дублирующиеся
  горутины опроса.
