# deadlock

Личная аналитика матчей Deadlock (в духе Leetify/STRATZ) на основе метаданных матчей с CDN Valve.

## Как устроено

```
deadlock-api.com ──(история матчей, соль URL)──┐
                                              ▼
replay*.valve.net ──(.meta, zstd)──► syncer ──► Postgres ◄── gRPC :9090 ◄── gateway REST :8080 ◄── web (Next.js :3000)
```

- `cmd/server` — gRPC + REST-gateway + фоновый воркер синхронизации в одном процессе
- `cmd/dlstat` — CLI для отладки (`history`, `match <id>`, `meta <file>`)
- `proto/api` — наш API (`deadlock.v1.StatsService`), `gen/` — сгенерированный код и OpenAPI
- `proto/deadlock` — протобафы Deadlock (копия SteamDatabase/Protobufs), `internal/pb` — сгенерированный код
- `internal/db` — Postgres (pgx), миграции в `internal/db/migrations`, применяются при старте
- `internal/syncer` — синхронизация истории и загрузка метаданных
- `internal/source` — провайдеры данных за интерфейсами (сейчас deadlock-api)
- `internal/zstd` — декодер zstd, скопирован из Go stdlib (BSD)
- `tools/dev` — раннер задач разработки (проверки, тесты, кодогенерация)
- `web` — веб-интерфейс (Next.js + TypeScript): профиль игрока, история матчей, разбор матча. Данные запрашивает на сервере у REST-gateway, типы API генерируются из OpenAPI
- `internal/gamedata` — герои, предметы и способности (названия EN/RU, слот, тир). Снимок `snapshot/gamedata.json` встроен в бинарник; собственные парсеры VPK (`vpk`), ресурсов Source 2 (`resource`) и бинарного KV3 (`kv3`). `item_id` в данных матча = MurmurHash2(lower(имя), 0x31415926)

В базе хранится сырой `.meta` (`matches.meta_raw`), поэтому таблицы можно пересчитать без повторной загрузки: Valve со временем удаляет старые реплеи.

## Запуск

```sh
cp .env.example .env          # SYNC_ACCOUNTS = твой account id
go run ./tools/gamedata -icons data/icons   # иконки из установленной игры (необязательно)
docker compose up -d --build
curl -X POST localhost:8080/v1/players/1517505834:sync
curl localhost:8080/v1/players/1517505834
curl localhost:8080/v1/players/1517505834/matches?pageSize=5
curl localhost:8080/v1/matches/134969900/players/1517505834
```

Веб-интерфейс: http://localhost:3000. Вместо account id везде можно указать SteamID64.

### Dev на мок-данных

Без Postgres, Go-сервера и сети — мок-API на снятых фикстурах (`web/mock`):

```sh
docker compose -f docker-compose.dev.yml up -d --build --wait   # http://localhost:3001
cd web && npm ci && npm run dev:mock                           # с hot reload: http://localhost:3002
```

Фронт против настоящего API без Docker: `cd web && npm run dev` (API_URL по умолчанию http://localhost:8080).

gRPC: `grpcurl -plaintext localhost:9090 list` (reflection включён).

Без Docker: `DATABASE_URL=postgres://... go run ./cmd/server`.

## Разработка

```sh
go run ./tools/dev setup      # один раз: линтер и плагины protoc в .tools/bin, git-хук
go run ./tools/dev check      # перед коммитом: tidy, формат, сборка, линт, тесты
go run ./tools/dev test-db    # интеграционные тесты БД (поднимает postgres из compose)
go run ./tools/dev web        # проверки фронта: typecheck, юнит-тесты, сборка
go run ./tools/dev e2e        # e2e фронта (Playwright) на мок-данных

# после изменения .proto или патча игры (обновить proto/deadlock):
go run ./tools/dev gen

# после патча игры: пересобрать снимок героев/предметов из локальной установки
go run ./tools/gamedata -check internal/meta/testdata/134969900.meta.zst

# посмотреть файлы игры
go run ./tools/vpkdump -grep scripts/ "<...>/Deadlock/game/citadel/pak01_dir.vpk"
go run ./tools/vpkdump -kv scripts/heroes.vdata_c -key hero_haze "<...>/pak01_dir.vpk"
```

Для кодогенерации нужен `protoc`; плагины нужных версий ставит `tools/dev`.

Документация для разработки:

- [docs/architecture.md](docs/architecture.md) — пакеты, границы, принятые решения, схема БД
- [docs/development.md](docs/development.md) — проверки, тесты, линтеры, кодогенерация, миграции, changelog, git
- [docs/code-style.md](docs/code-style.md) — правила написания кода и тестов
- [docs/frontend-testing.md](docs/frontend-testing.md) — тестирование фронта: юнит, e2e, мок, Playwright MCP
- [docs/problems.md](docs/problems.md) — грабли и известные проблемы
- [CHANGELOG.md](CHANGELOG.md)
