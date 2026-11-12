# deadlock_ideas

Личная аналитика матчей Deadlock (в духе Leetify/STRATZ) на основе метаданных матчей с CDN Valve.

## Как устроено

```
deadlock-api.com ──(история матчей, соль URL)──┐
                                              ▼
replay*.valve.net ──(.meta, zstd)──► syncer ──► Postgres ◄── gRPC :9090 ◄── gateway REST :8080 ◄── web (Next.js, позже)
```

- `cmd/server` — gRPC + REST-gateway + фоновый воркер синхронизации в одном процессе
- `cmd/dlstat` — CLI для отладки (`history`, `match <id>`, `meta <file>`)
- `proto/api` — наш API (`deadlock.v1.StatsService`), `gen/` — сгенерированный код и OpenAPI
- `proto/deadlock` — протобафы Deadlock (копия SteamDatabase/Protobufs), `internal/pb` — сгенерированный код
- `internal/db` — Postgres (pgx), миграции в `internal/db/migrations`, применяются при старте
- `internal/syncer` — синхронизация истории и загрузка метаданных
- `internal/source` — провайдеры данных за интерфейсами (сейчас deadlock-api)
- `internal/zstd` — декодер zstd, скопирован из Go stdlib (BSD)

В базе хранится сырой `.meta` (`matches.meta_raw`), поэтому таблицы можно пересчитать без повторной загрузки: Valve со временем удаляет старые реплеи.

## Запуск

```sh
cp .env.example .env          # SYNC_ACCOUNTS = твой account id
docker compose up -d --build
curl -X POST localhost:8080/v1/players/1517505834:sync
curl localhost:8080/v1/players/1517505834
curl localhost:8080/v1/players/1517505834/matches?pageSize=5
curl localhost:8080/v1/matches/134969900/players/1517505834
```

gRPC: `grpcurl -plaintext localhost:9090 list` (reflection включён).

Без Docker: `DATABASE_URL=postgres://... go run ./cmd/server`.

## Разработка

```sh
go test ./...
# интеграционный тест БД:
TEST_DATABASE_URL='postgres://deadlock:deadlock@localhost:5432/deadlock?sslmode=disable' go test ./internal/db

# после изменения .proto или патча игры (обновить proto/deadlock):
go run ./tools/protogen
```

Нужны `protoc` и плагины в `$GOPATH/bin`: `protoc-gen-go`, `protoc-gen-go-grpc`, `protoc-gen-grpc-gateway`, `protoc-gen-openapiv2`.
