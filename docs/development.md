# Разработка: пайплайны и процессы

## Первый запуск на новой машине

Нужны: Go (версия из `go.mod`), Docker, `protoc` (только для кодогенерации; на Windows — `winget install Google.Protobuf`), Node.js 26 (только для фронта; `winget install OpenJS.NodeJS.LTS`).

```sh
go run ./tools/dev setup      # golangci-lint, govulncheck, плагины protoc -> .tools/bin; включает git-хук
cp .env.example .env          # SYNC_ACCOUNTS = свой account id
docker compose up -d --build
```

`setup` выполняет `git config core.hooksPath .githooks`: перед каждым коммитом запускается `dev check`.

## Пайплайн проверок

`go run ./tools/dev check` — единственный критерий «код в порядке». Шаги идут по порядку, первый упавший останавливает прогон:

| Шаг | Что делает | Как чинить |
|---|---|---|
| go mod tidy | `go mod tidy -diff`: `go.mod`/`go.sum` соответствуют импортам | `go mod tidy` |
| format | `golangci-lint fmt --diff` (gofumpt + goimports) | `go run ./tools/dev fmt` |
| build | `go build ./...` | — |
| lint | `golangci-lint run` | `go run ./tools/dev lint --fix`, остальное руками |
| test | `go test -shuffle=on ./...` | — |

Тот же набор выполняется в трёх местах:

- **локально** — вручную перед завершением задачи;
- **pre-commit хук** (`.githooks/pre-commit`) — тот же `dev check`;
- **CI** (`.github/workflows/ci.yml`) — `dev ci` (то же, но тесты с `-race`), затем `dev test-db` на сервисе Postgres и `dev vuln`.

Отдельно, не входит в `check`:

- `go run ./tools/dev test-db` — обязателен при любых изменениях в `internal/db`;
- `go run ./tools/dev test-linux` — юнит-тесты в контейнере `golang` с `-race`: то же, что в CI. Запускать при изменениях в работе с путями, файлами и конкурентностью, и всегда, когда CI упал, а локально зелено;
- `go run ./tools/dev vuln` — при обновлении зависимостей;
- `go run ./tools/dev web` — проверки фронта (`web/`): `npm ci` (если нет `node_modules` или задан `CI`), `tsc --noEmit`, `node --test`, `next build`. Обязателен при изменениях в `web/` и в API. В CI — отдельная джоба `web`;
- `go run ./tools/dev e2e` — Playwright против мок-API (ставит Chromium при первом запуске); аргументы уходят в `playwright test` (`-g sync`, `--headed`). Обязателен там же, где `web`. Подробно — [frontend-testing.md](frontend-testing.md).

Устаревший `web/src/api/schema.gen.ts` ловит обычный `check`: тест `TestSwaggerToTSRealSpec` сравнивает его с результатом генерации.

## Тестирование

Три уровня:

1. **Юнит-тесты** (`go test ./...`) — без сети, без БД, без установленной игры. Быстрые, запускаются всегда. `server` и `syncer` тестируются с фейковыми `Store`; HTTP — подменой `http.RoundTripper`.
2. **Интеграционные тесты БД** (`internal/db`, `dev test-db`) — на настоящем Postgres. Без `TEST_DATABASE_URL` пропускаются (`t.Skip`). Каждый прогон создаёт временную схему `test_<время>` и удаляет её, данные приложения не трогаются. `dev test-db` сам поднимает `postgres` из compose и запускает с `-count=1`.
3. **Тесты на локальной установке игры** (`TestExtractLocalInstall`) — пропускаются, если Deadlock не установлен (путь — `DEADLOCK_GAME_DIR`).

Эталонные данные: `internal/meta/testdata/134969900.meta.zst` — настоящий файл с CDN Valve. На него опираются тесты `meta`, `db`, `syncer`, `gamedata`. Новые эталоны класть в `testdata/` соответствующего пакета.

Что обязательно покрывать тестами:

- любую логику ветвления в `server` (валидация, пагинация, маппинг ошибок) и `syncer` (что считается постоянной ошибкой, что повторяется);
- каждый новый запрос в `internal/db` — в интеграционном тесте, включая повторный вызов (идемпотентность);
- парсеры — на реальных образцах плюс мусор на входе;
- исправленный баг — тестом, который падает без исправления.

Детектор гонок: на Windows `-race` требует gcc, поэтому гонки проверяются в Linux — `go run ./tools/dev test-linux` локально и `dev ci` в CI.

Правила написания тестов — в [code-style.md](code-style.md#тесты).

## Линтеры и форматирование

Конфигурация — `.golangci.yml` (golangci-lint v2, версия закреплена в `tools/dev/main.go`).

- Форматирование: **gofumpt** + **goimports**; импорты в три группы: stdlib, внешние, `github.com/obsessed-gopher/deadlock/...`. В GoLand стоит включить «gofumpt» и «goimports» при сохранении, либо просто запускать `dev fmt`.
- Линтеры: стандартный набор (errcheck, govet, ineffassign, staticcheck, unused) плюс bodyclose, depguard, errorlint, gocritic, gosec, noctx, revive, rowserrcheck, sqlclosecheck, sloglint и др.
- Из проверки исключены: `internal/pb`, `gen` (сгенерировано), `internal/zstd` (копия stdlib).
- `depguard` охраняет границы пакетов — см. [architecture.md](architecture.md#правила-зависимостей).

Порядок действий при замечании линтера:

1. Исправить код.
2. Если это ложное срабатывание в одном месте — `//nolint:<линтер> // причина` на этой строке (`nolintlint` требует и имя, и причину).
3. Если правило системно не подходит проекту — исключение в `.golangci.yml` с комментарием «почему». Не отключать линтер целиком ради одного файла.

Обновление версии линтера: поменять версию в `tools/dev/main.go`, запустить `dev lint`, исправить новые замечания в том же коммите.

## Кодогенерация

```sh
go run ./tools/dev gen
```

Ставит закреплённые плагины в `.tools/bin` и запускает `tools/protogen`:

- `proto/deadlock` (копия [SteamDatabase/Protobufs](https://github.com/SteamDatabase/Protobufs)) → `internal/pb`. Генерируется только замыкание импортов корневых файлов из `gameRoots` в `tools/protogen/main.go`; файлы Valve предварительно очищаются от нестандартных опций.
- `proto/api` → `gen/go` (сообщения, gRPC, gateway) и `gen/openapi`.
- `gen/openapi/api.swagger.json` → `web/src/api/schema.gen.ts` (TS-типы для фронта, `tools/protogen/tsgen.go`). Готовые генераторы (openapi-typescript и др.) больше не читают OpenAPI v2, а спецификация использует лишь объекты, enum-ы, массивы и `$ref`. Форма типов повторяет JSON gateway: поля есть всегда (`EmitUnpopulated`), незаданное сообщение — `null`, 64-битные числа — строки.

Правила:

- Сгенерированные файлы коммитятся вместе с изменением `.proto`, руками не правятся.
- Версии плагинов в `tools/dev/main.go` должны совпадать с версиями библиотек в `go.mod`. Обновляешь `google.golang.org/protobuf`, `grpc` или `grpc-gateway` — обнови и плагин, и перегенерируй.
- Проверка воспроизводимости: после `dev gen` на чистом дереве `git status` не должен показывать изменений в `gen/` и `internal/pb/`.

### После патча игры

1. Обновить `proto/deadlock` из SteamDatabase/Protobufs → `go run ./tools/dev gen`.
2. Пересобрать снимок и иконки: `go run ./tools/gamedata -check internal/meta/testdata/134969900.meta.zst -icons data/icons`.
3. `go run ./tools/dev check`; запись в `CHANGELOG.md`.

## Миграции БД

- Файл `internal/db/migrations/NNNN_short_name.sql`, номер — следующий по порядку, четыре цифры.
- Применяются автоматически при старте сервера, каждая в своей транзакции.
- **Уже применённую миграцию не менять** (раннер запоминает номер, а не содержимое): исправление — новой миграцией.
- Только вперёд. Миграция должна быть совместима со старым кодом на время выката либо выкатываться одновременно с ним (процесс один, так что достаточно второго).
- Производные таблицы можно смело пересоздавать: они восстанавливаются из `matches.meta_raw`. Колонки `matches.meta_*` и таблицы из истории матчей — нет.
- Изменилось то, что `db.derive` выводит из `.meta` (новая колонка, исправление) → **поднять `db.DeriveVersion`**. Сервер при старте сам пересоберёт все матчи из `meta_raw`; миграция только добавляет колонки.
- После изменения схемы: обновить запросы в `internal/db`, интеграционный тест, раздел «Схема БД» в architecture.md, прогнать `dev test-db`.

## Changelog

`CHANGELOG.md` в формате [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/), на русском.

- Каждое изменение, заметное пользователю API/CLI или тому, кто разворачивает сервис, — строка в `[Unreleased]` в том же коммите, что и изменение.
- Разделы: `Добавлено`, `Изменено`, `Исправлено`, `Удалено`, `Безопасность`.
- Писать о результате, а не о процессе: «`/healthz` больше не раскрывает текст ошибки БД», а не «поправил хендлер».
- Не записывать: рефакторинг без изменения поведения, правки тестов и документации, обновление dev-инструментов.
- Ломающие изменения (API, схема без автоматической миграции, переменные окружения) — с пометкой **BREAKING** и инструкцией по переходу.
- Релиз: переименовать `[Unreleased]` в `[X.Y.Z] - ГГГГ-ММ-ДД`, поставить тег `vX.Y.Z`, открыть новый пустой `[Unreleased]`. Версии — SemVer; до `1.0.0` ломающие изменения поднимают минорную версию.

## Git

- Ветки: `feat/<тема>`, `fix/<тема>`, `chore/<тема>`; в основную ветку — через merge/PR после зелёного `check`.
- В сообщениях коммитов и описаниях PR/MR нет упоминаний Claude/ИИ и строк `Co-Authored-By` (см. CLAUDE.md).
- Сообщения коммитов — на английском, в повелительном наклонении, без префиксов и точки в конце: `Add hero/item names from game files`. Первая строка до ~72 символов; «зачем» — в теле, если не очевидно.
- Один коммит — одно логическое изменение; код, тесты, сгенерированные файлы, changelog и документация к нему — вместе.
- `--no-verify` — только чтобы сохранить незаконченную работу в своей ветке; перед слиянием `check` должен пройти.

## Фронтенд

`web/` — Next.js (App Router) + TypeScript, собственный `package.json`; зависимости — только `next`, `react`, `react-dom` и типы. Версии закреплены точно (`--save-exact`), `package-lock.json` в git.

- Страницы — серверные компоненты: данные запрашиваются на сервере Next у REST-gateway (`src/api/client.ts`, адрес — `API_URL`). Браузер в Go API не ходит, CORS не нужен, порт API остаётся на `127.0.0.1`.
- Синхронизация — server action (`src/app/players/[id]/actions.ts`), форма работает и без JS.
- Логика без React (форматирование, разбор покупок, статистика героев) — в `src/lib`, тесты рядом (`*.test.ts`, `node:test`, без зависимостей).
- Тестирование (юнит, e2e, мок-API, сверка через Playwright MCP) — [frontend-testing.md](frontend-testing.md).
- Графики — SVG руками, без библиотек.
- Импорты внутри `src` — с расширением `.ts`, чтобы `node --test` запускал файлы без сборки.

## Стеки: прод и dev

| | Файл / команда | Состав | Порты (на `127.0.0.1`) |
|---|---|---|---|
| прод | `docker compose up -d --build --wait` (`docker-compose.yml`) | postgres, server, web | web 3000, REST 8080, gRPC 9090, postgres 5432 |
| dev на моке | `docker compose -f docker-compose.dev.yml up -d --build --wait` | mock, web (та же production-сборка) | web 3001, mock 8090 |
| dev с hot reload | `cd web && npm run dev:mock` | mock + `next dev` | web 3002, mock 8092 |
| e2e | `go run ./tools/dev e2e` | mock + standalone-сборка | web 3200, mock 8091 |

Порты не пересекаются, стеки можно держать одновременно. Dev-стек — отдельный compose-проект (`name: deadlock-dev`), без Postgres, Go и сети: собирается за минуту на любой машине. Остановить: `docker compose -f docker-compose.dev.yml down`.

Бэкенд на Go dev-стек не проверяет: для него — юнит-тесты, `test-db` и прод-стек.

## Конфигурация

Вся конфигурация сервера — переменные окружения, читаются и валидируются в `cmd/server/config.go`; список с умолчаниями — в комментарии пакета `cmd/server/main.go`. Новая переменная: `config.go` + тест, комментарий в `main.go`, `.env.example`, `docker-compose.yml`, запись в changelog.

Фронт читает `API_URL` (адрес REST-gateway), `SYNC_ACCOUNTS` (ссылки на главной), `TZ` (часовой пояс дат: они рендерятся на сервере) и `ICONS_DIR` (каталог иконок; по умолчанию `../data/icons` от `web/`, в Docker — смонтированный `./data/icons`).

## Иконки

Иконки героев, предметов и способностей — арт из файлов игры, **в git их нет**. Их достаёт из локальной установки `go run ./tools/gamedata -icons data/icons` (`internal/gamedata/vtex` декодирует текстуры `*.vtex_c`, PNG до 96 px, около 5 МБ). Повторять после патча игры.

Отдаёт их фронт: `GET /icons/<heroes|abilities>/<key>` (`web/src/app/icons/...`) читает PNG из `ICONS_DIR`, а если файла нет — отвечает SVG-заглушкой с инициалами (200, не 404). Поэтому CI, e2e, мок и чистая машина работают без иконок и без ошибок в консоли; e2e нарочно запускаются без них (`ICONS_DIR=e2e/no-icons`).

## Работа с проблемами

Когда что-то пошло не так:

1. Посмотреть [problems.md](problems.md) — возможно, это известные грабли.
2. Воспроизвести минимально; для бага в коде — сначала написать падающий тест.
3. Найти причину, а не заглушить симптом (не `t.Skip`, не `//nolint`, не увеличенный таймаут без понимания).
4. Исправить, убедиться, что тест позеленел, прогнать `dev check`.
5. Если причина была неочевидной или может повториться — записать в `problems.md`.
