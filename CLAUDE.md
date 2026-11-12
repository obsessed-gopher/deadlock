# deadlock

Личная аналитика матчей Deadlock: история матчей и соли берутся у deadlock-api.com, файлы `.meta` — с CDN Valve, всё складывается в Postgres и отдаётся через gRPC + REST-gateway; веб-интерфейс — Next.js в `web/`. Go 1.27, pgx, grpc-gateway; Node 26, Next 16, TypeScript. Один разработчик, Windows + GoLand, прод — Docker (linux).

Подробности — в `docs/`. Читай нужный файл **до** того, как менять соответствующую область:

| Файл | Когда читать |
|---|---|
| [docs/architecture.md](docs/architecture.md) | перед изменением структуры пакетов, зависимостей, схемы БД, API |
| [docs/development.md](docs/development.md) | пайплайны: проверки, тесты, кодогенерация, миграции, changelog, коммиты |
| [docs/code-style.md](docs/code-style.md) | правила написания кода (Go и TypeScript) и тестов в этом проекте |
| [docs/frontend-testing.md](docs/frontend-testing.md) | перед любым изменением `web/`: юнит, e2e, мок-данные, сверка через Playwright MCP |
| [docs/roadmap.md](docs/roadmap.md) | план развития: какие части в работе и в каком порядке |
| [docs/problems.md](docs/problems.md) | грабли и известные проблемы — смотри сюда первым делом, когда что-то «странно» |

## Команды

Единая точка входа — `go run ./tools/dev <команда>` (make нет, работает одинаково на Windows, Linux и в CI):

```sh
go run ./tools/dev check     # ОБЯЗАТЕЛЬНО перед завершением задачи: tidy, формат, сборка, линт, тесты
go run ./tools/dev fmt       # отформатировать (gofumpt + goimports)
go run ./tools/dev lint      # golangci-lint; аргументы пробрасываются (--fix)
go run ./tools/dev test      # юнит-тесты; аргументы пробрасываются: test -run TestX
go run ./tools/dev test-db   # интеграционные тесты БД (сам поднимает postgres из compose)
go run ./tools/dev test-linux  # юнит-тесты в Linux-контейнере с -race — то, что гоняет CI
go run ./tools/dev gen       # кодогенерация из .proto (Go, OpenAPI и TS-типы web/src/api/schema.gen.ts)
go run ./tools/dev web       # фронт: npm ci (если нет node_modules), typecheck, юнит-тесты, сборка
go run ./tools/dev e2e       # фронт: e2e (Playwright, desktop + mobile) против мок-API
go run ./tools/dev vuln      # govulncheck
go run ./tools/dev setup     # один раз на новой машине: инструменты в .tools/bin + git-хук
```

Один тест одного пакета: `go test ./internal/syncer -run TestProcessMeta -v`.

Стеки (подробно — README):

```sh
docker compose up -d --build --wait                              # прод: postgres + server + web, :3000 / :8080
docker compose -f docker-compose.dev.yml up -d --build --wait    # dev на мок-данных: web + mock, :3001 / :8090
cd web && npm run dev:mock                                       # hot reload на мок-данных, :3002
```

## Определение «готово»

Задача не закончена, пока не выполнено всё:

1. `go run ./tools/dev check` проходит. Не «должно проходить» — запусти и посмотри вывод.
2. Менялся `internal/db` (SQL, миграции) → прошёл `go run ./tools/dev test-db`.
   Менялась работа с путями, файлами или горутинами → прошёл `go run ./tools/dev test-linux` (локальный `check` на Windows таких ошибок не видит).
   Менялся `web/` или API → прошли `go run ./tools/dev web` и `go run ./tools/dev e2e`.
3. Менялись `.proto` → выполнен `go run ./tools/dev gen`, сгенерированные файлы в том же коммите.
4. Новое поведение покрыто тестом; исправленный баг — тестом, который без исправления падает.
   Фронт: логика — юнит-тестом (`node:test`), пользовательский сценарий — e2e-тестом Playwright; изменился API — обновлены мок (`web/mock/api.ts`) и при необходимости фикстуры.
   Фронт: весь текст — через словари в **обеих** локалях (ru/en); имена героев/предметов — через `i18n.name` (по умолчанию английские); у каждой таблицы — сортировка и фильтры, длинные сворачиваются; ошибки — локализованные, без 500 (`docs/code-style.md`, раздел «Фронтенд»).
   Видимое изменение UI — **сверено глазами через Playwright MCP** на dev-стеке с мок-данными: desktop и mobile, без ошибок в консоли (порядок — `docs/frontend-testing.md`).
5. Видимое пользователю изменение записано в `CHANGELOG.md` → `[Unreleased]`.
6. Наткнулся на неочевидную проблему или оставил долг → запись в `docs/problems.md`.
7. Изменилась структура, зависимости между пакетами, конфигурация или команды → обновлён соответствующий файл в `docs/` и/или `README.md`.

## Жёсткие правила

- **Не править руками** сгенерированное и чужое: `internal/pb/`, `gen/`, `web/src/api/schema.gen.ts`, `internal/zstd/` (копия из stdlib Go), `proto/deadlock/` и `proto/third_party/` (вендорные протобафы), `internal/gamedata/snapshot/gamedata.json` (пересобирается `tools/gamedata`).
- **Применённую миграцию не менять.** Любое изменение схемы — новый файл `internal/db/migrations/NNNN_name.sql`.
- **`matches.meta_raw` не удалять и не пережимать**: Valve стирает реплеи, это единственная копия, из неё пересчитываются все производные таблицы.
- **Границы пакетов** (проверяет depguard, см. architecture.md): pgx и SQL — только в `internal/db`; gRPC и типы API — только в `internal/server`, `cmd/server` и модели чтения `internal/db`; `server` и `syncer` зависят от БД через собственные интерфейсы `Store`.
- **Не глушить линтер** ради зелёной сборки. `//nolint` — только с именем линтера и причиной; ослаблять `.golangci.yml` — только осознанно и с комментарием «почему».
- **Новые зависимости** — только по необходимости и с согласия владельца: проект намеренно держит их минимум (даже zstd скопирован из stdlib).
- **Вежливость к чужим сервисам**: не убирать и не уменьшать паузу между загрузками (`META_INTERVAL`), не ходить в сеть из тестов. E2E и dev-стек работают только на мок-API; `web/mock/capture.ts` запускается только руками против своего сервера.
- **Никаких упоминаний Claude/ИИ в git**: ни `Co-Authored-By`, ни «Generated with…», ни иных пометок в сообщениях коммитов, описаниях PR/MR, тегах, changelog и комментариях к коду. Автор — только владелец репозитория. (Продублировано в `.claude/settings.json` → `attribution`.)
- Коммитить и пушить — только по просьбе. Работа ведётся в feature-ветках (`feat/...`, `fix/...`), не в `main`.

## Язык

- Код, комментарии, сообщения логов и ошибок, `.proto`, сообщения коммитов — **английский**.
- `README.md`, `CLAUDE.md`, `docs/`, `CHANGELOG.md`, общение с владельцем — **русский**.
