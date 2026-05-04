# M0_embeddings — генерация векторов

## Цель

Появляется подкоманда `mdx embed`. После её запуска для каждой модели из конфигурации и каждой заметки в таблице `notes`, для которой ещё нет актуального вектора, через внешний embedding API считается embedding и через REST API Qdrant загружается в коллекцию `mdx` как именованный вектор. Локальная таблица `embeddings` фиксирует факт расчёта (path, model, content_hash, embedded_at). Повторный запуск идемпотентен: уже посчитанные точки пропускаются, изменившиеся (по `content_hash`) пересчитываются.

После M0_embeddings корпус заметок векторизован и готов к поиску. На этот фундамент M1_embeddings навешивает подкоманду `mdx search`, M2_embeddings — neovim-команду `:MdxSearch` с Telescope-picker'ом.

## Что входит и что не входит

### Входит

- Подкоманда `mdx embed`, регистрируемая в `cmd/mdx/main.go` рядом со `scan`/`gc`/`lsp`.
- Конфигурация `~/.config/mdx/embedding.yaml`: список моделей, URL Qdrant, имя коллекции; парсер и валидация.
- Миграция схемы БД с v1 до v2: добавляется таблица `embeddings(path, model, content_hash, embedded_at)`.
- HTTP-клиент к Qdrant: создание коллекции, добавление недостающих именованных векторов, batch upsert точек.
- HTTP-клиент к embedding API с тремя вариантами payload по `endpoint_kind`: `openai`, `llama-cpp`, `tei`.
- Идемпотентный pipeline: для каждой пары `(path, model)` пересчёт выполняется только если в `embeddings` нет записи или `content_hash` устарел относительно `notes`.
- UUID v5 от абсолютного пути заметки в роли id точки Qdrant.
- Стандартные флаги: `--model <name>` для ограничения одной моделью, `--all` для безусловного пересчёта всего корпуса.
- Юнит-тесты на конфиг, на db-слой, на клиенты embedding API и Qdrant (через `httptest`); интеграционный тест `mdx embed` end-to-end на временной SQLite + httptest-серверах.

### Не входит (отложено)

- Подкоманда `mdx search` — это M1_embeddings.
- `:MdxSearch` в neovim-плагине — это M2_embeddings.
- Чанкирование длинных заметок: содержимое отправляется в API целиком, превышение контекста — забота модели.
- Чистка точек в Qdrant при удалении/перемещении заметок — задача расширения `mdx gc`, см. open questions в `embeddings.md`.
- Авто-embed по `didSave` через LSP — open question.
- Гибридный поиск (sparse-вектор BM25/SPLADE) — open question.
- Reranking-модели — open question.
- Параллелизм по моделям и batch'ам — pipeline однопоточный.
- Hot-reload конфига `embedding.yaml` — читается один раз на старте `mdx embed`.

## Технические решения

| Вопрос | Решение |
|---|---|
| Формат конфига | YAML. Парсер — `gopkg.in/yaml.v3`, уже подключён для frontmatter. Тащить TOML/JSON-парсеры ради отдельного файла нет смысла. |
| Резолв пути конфига | Флаг `--embedding-config` → переменная `MDX_EMBEDDING_CONFIG` → `$XDG_CONFIG_HOME/mdx/embedding.yaml` → `~/.config/mdx/embedding.yaml`. По образцу `config.ResolveIgnorePath`. |
| Отсутствие файла конфига | `mdx embed` завершается с ошибкой, в сообщении приводится ожидаемый путь. Запуск без моделей бессмыслен — дефолт «работа без конфига» не имеет полезного поведения. |
| Версия схемы БД | Поднимается с v1 до v2. `schema.sql` обновляется (для свежих БД), `migrations.go` получает миграцию `1→2`, добавляющую таблицу `embeddings`. |
| Identifier точки Qdrant | UUID v5 = `uuid.NewSHA1(namespace, []byte(absolutePath))`. Namespace — константа в пакете `embed`, фиксируется единожды и больше не меняется. Идемпотентность upsert обеспечивается этим id. |
| Транспорт к Qdrant | HTTP REST через `net/http`. gRPC — open question. |
| Транспорт к embedding API | HTTP REST через `net/http`. Тело запроса формируется по `endpoint_kind`: `openai` (`{model, input}` → `{data:[{embedding}]}`), `llama-cpp` (`{content}` → `{embedding}`), `tei` (`{inputs}` → `[[...]]`). |
| Размер batch | По полю `batch_size` модели. Тексты одной модели нарезаются на пачки этого размера, каждая отправляется одним HTTP-запросом. Соответствующие точки upsert-ятся в Qdrant одним запросом. |
| Что подаётся модели как «документ» | Сырой текст файла (включая frontmatter), плюс `document_prefix` модели. Никаких дополнительных трансформаций (удаление frontmatter, нормализация whitespace и т.п.). |
| Превышение контекста модели | Не обрезается на стороне `mdx`. Если модель отвергает запрос — ошибка батча, см. ниже. |
| Транзакционность | Запись в `embeddings` идёт после успешного upsert в Qdrant. Сбой между этими шагами безопасен: при следующем запуске `content_hash` всё ещё несовпадающий, точка пересчитается и перепишется. |
| Поведение при ошибке батча | Текст ошибки в stderr, счётчик `failed` инкрементируется на размер батча, цикл продолжается со следующего батча. Один сбой не останавливает прогон. |
| Конкурентность | Однопоточно: модели обрабатываются по очереди, внутри модели — последовательная отправка батчей. Параллелизм — open question. |
| Команда `mdx embed` без аргументов | Прогон всех моделей из конфига. `--model <name>` сужает до одной (имя должно совпадать с одним из `models[].name`). |
| Флаг `--all` | Игнорирует таблицу `embeddings` и пересчитывает все заметки для выбранных моделей. Полезен после смены `query_prefix`/`document_prefix`/`api_model_name` без переименования модели. |
| Прогресс и сводка | `quiet`-режим, как у `scan`/`gc` (`-q`). По завершении (если не `--quiet`) — строка `embedded: N, skipped: M, failed: K, elapsed: T`. Промежуточный прогресс — без TUI; ошибки идут в stderr по мере возникновения. |

## Структура каталогов проекта (что добавляется к M5)

```
/home/oleg/projects/mdx/lsp/
├── cmd/mdx/
│   └── main.go                       ← регистрация подкоманды embed
└── internal/
    ├── config/
    │   ├── embedding.go              ← новый: ResolveEmbeddingPath, LoadEmbedding
    │   └── embedding_test.go         ← новый
    ├── db/
    │   ├── schema.sql                ← обновлён: добавлена таблица embeddings
    │   ├── migrations.go             ← обновлён: миграция 1→2
    │   ├── embeddings.go             ← новый: PendingEmbeddings, RecordEmbedding
    │   └── embeddings_test.go        ← новый
    ├── embed/                        ← новый пакет: helpers без оркестрации
    │   ├── doc.go
    │   ├── id.go                     ← UUID v5 от пути
    │   ├── id_test.go
    │   ├── model.go                  ← клиент embedding API (3 endpoint_kind)
    │   ├── model_test.go
    │   ├── qdrant.go                 ← клиент Qdrant (collection + upsert)
    │   └── qdrant_test.go
    └── cli/
        ├── embed.go                  ← новый: EmbedOptions, EmbedStats, RunEmbed (оркестрация)
        └── embed_test.go             ← e2e на httptest + временной SQLite
```

## Шаги выполнения

Шаги упорядочены так, чтобы после каждого можно было запустить осмысленную проверку — либо `go test ./...`, либо ручной прогон `mdx embed` с реальным Qdrant и моделью.

### Шаг 1. Скаффолдинг подкоманды и пакета `embed`

Зарегистрировать `mdx embed` как cobra-команду с заглушкой и создать пакет `internal/embed` со скелетом точки входа.

#### Состав `cmd/mdx/main.go`

Добавить переменные флагов:

```go
var (
    flagEmbedConfig string
    flagEmbedModel  string
    flagEmbedAll    bool
)
```

Объявить команду:

```go
var embedCmd = &cobra.Command{
    Use:   "embed",
    Short: "Compute embeddings for indexed notes and upsert them into Qdrant",
    Args:  cobra.NoArgs,
    RunE:  runEmbed,
}
```

В `init()` зарегистрировать флаги и команду:

```go
embedCmd.Flags().StringVar(&flagEmbedConfig, "embedding-config", "",
    "path to embedding config (default: $XDG_CONFIG_HOME/mdx/embedding.yaml)")
embedCmd.Flags().StringVar(&flagEmbedModel, "model", "",
    "limit run to a single model name from config")
embedCmd.Flags().BoolVar(&flagEmbedAll, "all", false,
    "ignore embeddings table and recompute every note")
embedCmd.Flags().BoolVarP(&flagQuiet, "quiet", "q", false, "suppress summary line")

rootCmd.AddCommand(embedCmd)
```

`runEmbed` на этом шаге — минимальная заглушка, возвращающая ошибку `embed: not implemented`. Открытие БД, чтение конфига и миграция в `runEmbed` появляются в Шагах 2–3, когда становятся востребованы. На текущем шаге `fmt` уже импортирован в `main.go`, дополнительных импортов не требуется:

```go
func runEmbed(cmd *cobra.Command, args []string) error {
    return fmt.Errorf("embed: not implemented")
}
```

#### Состав `internal/embed/doc.go`

Пакет `embed` собирается ради клиентов embedding API и Qdrant и helper'а для UUID v5; оркестрация embed-прогона живёт в `internal/cli/embed.go` рядом с `cli/scan.go` и `cli/gc.go`. На текущем шаге пакет состоит из одного файла с package-level комментарием — этого достаточно, чтобы он существовал и был импортируем последующими шагами.

```go
// Package embed provides clients for the embedding API and Qdrant, plus
// the UUID v5 helper used as point id. Orchestration of an mdx embed run
// lives in internal/cli/embed.go.
package embed
```

#### Состав `internal/cli/embed.go`

По образцу `cli/scan.go` и `cli/gc.go` — собственные типы `EmbedOptions` и `EmbedStats` и функция `RunEmbed`, заглушенная до Шага 8. Сигнатура `RunEmbed` намеренно не принимает конфиг на этом шаге: `config.EmbeddingConfig` вводится в Шаге 2, и в нём же сигнатура расширяется до `RunEmbed(ctx, conn, cfg, opts)`. До тех пор скелет должен компилироваться без forward-ссылок.

```go
package cli

import (
    "context"
    "database/sql"
    "errors"
    "time"
)

// EmbedOptions collects flags driving a single mdx embed run.
type EmbedOptions struct {
    Model string // empty = all models from config
    All   bool   // ignore embeddings table and recompute every note
}

// EmbedStats summarizes one mdx embed run.
type EmbedStats struct {
    Embedded int
    Skipped  int
    Failed   int
    Elapsed  time.Duration
}

// RunEmbed is filled in by Step 8. The signature widens to accept
// config.EmbeddingConfig in Step 2.
func RunEmbed(ctx context.Context, conn *sql.DB, opts EmbedOptions) (EmbedStats, error) {
    return EmbedStats{}, errors.New("embed: not implemented")
}
```

`EmbedOptions.Quiet` намеренно отсутствует: подавлением итоговой строки занимается `runEmbed` в `main.go` по `flagQuiet`, оркестратору флаг не нужен.

**Готовность:** `go build ./...` зелёное; `mdx embed --help` отображает справку с флагами `--embedding-config`, `--model`, `--all`, `-q`; `mdx embed` завершается с ненулевым кодом и сообщением `embed: not implemented`. Открытия БД и обращения к ФС на этом шаге не происходит — это становится видно по тому, что команда возвращает ошибку даже при `--db /nonexistent/path`.

### Шаг 2. Конфиг `embedding.yaml`

Реализовать парсер и валидацию конфигурации.

#### Состав `internal/config/embedding.go`

Структуры:

```go
type EmbeddingConfig struct {
    QdrantURL  string        `yaml:"qdrant_url"`
    Collection string        `yaml:"collection"`
    Models     []ModelConfig `yaml:"models"`
}

type ModelConfig struct {
    Name             string `yaml:"name"`
    Endpoint         string `yaml:"endpoint"`
    EndpointKind     string `yaml:"endpoint_kind"` // openai | llama-cpp | tei
    APIModelName     string `yaml:"api_model_name"`
    Dim              int    `yaml:"dim"`
    Distance         string `yaml:"distance"` // cosine (only value supported in M0)
    QueryPrefix      string `yaml:"query_prefix"`
    DocumentPrefix   string `yaml:"document_prefix"`
    BatchSize        int    `yaml:"batch_size"`
    DefaultForSearch bool   `yaml:"default_for_search"`
}
```

Функции:

```go
func ResolveEmbeddingPath(flag string) (string, error)
func LoadEmbedding(path string) (EmbeddingConfig, []string, error)
```

`ResolveEmbeddingPath` повторяет схему `ResolveIgnorePath`: флаг → `MDX_EMBEDDING_CONFIG` → `$XDG_CONFIG_HOME/mdx/embedding.yaml` → `~/.config/mdx/embedding.yaml`.

`LoadEmbedding`:

1. Если файла нет — возвращает `EmbeddingConfig{}, nil, os.ErrNotExist`. Решение «нет конфига = ошибка» принимается на уровне `runEmbed`, не здесь.
2. Парсит YAML через `yaml.Unmarshal`.
3. Валидирует:
    - `qdrant_url` непустой и парсится как URL;
    - `collection` непустой;
    - `len(models) >= 1`;
    - имена моделей уникальны;
    - у каждой модели непустые `name`, `endpoint`, `endpoint_kind`, `api_model_name`; `endpoint_kind` ∈ `{"openai","llama-cpp","tei"}`; `dim > 0`; `distance == "cosine"` (любое другое значение — ошибка с указанием, что в M0 поддержан только cosine); `batch_size > 0`;
    - не более одной модели с `default_for_search: true`; если `len(models) > 1` и таких нет — ошибка.
4. Список `[]string` warnings возвращает предупреждения, не приводящие к ошибке (на M0 — пуст; зарезервирован под будущие предупреждения по образцу `LoadIgnore`).

#### Тесты `internal/config/embedding_test.go`

Табличные тесты на `LoadEmbedding`:

- валидный конфиг с одной моделью без `default_for_search`;
- валидный конфиг с двумя моделями, одна с `default_for_search: true`;
- две модели, ни одна не помечена default — ожидается ошибка;
- две модели, обе помечены default — ошибка;
- неизвестный `endpoint_kind` — ошибка;
- `distance: dot` — ошибка с упоминанием cosine;
- `dim: 0` — ошибка;
- дубликат `name` — ошибка;
- отсутствующий файл — ошибка `os.ErrNotExist`.

И отдельный тест на `ResolveEmbeddingPath` по образцу `ignore_test.go::TestResolveIgnorePath`.

#### Расширение сигнатуры `RunEmbed`

Сигнатура `cli.RunEmbed`, заглушенная в Шаге 1, расширяется параметром `cfg config.EmbeddingConfig`:

```go
func RunEmbed(ctx context.Context, conn *sql.DB, cfg config.EmbeddingConfig, opts EmbedOptions) (EmbedStats, error) {
    return EmbedStats{}, errors.New("embed: not implemented")
}
```

Тело по-прежнему возвращает «not implemented» — оркестрация добавляется в Шаге 8. Расширение делается сейчас, чтобы вызов из `runEmbed` ниже потреблял загруженный `cfg` и проект собирался без обходных манёвров.

#### Подключение в `runEmbed`

```go
func runEmbed(cmd *cobra.Command, args []string) error {
    cfgPath, err := config.ResolveEmbeddingPath(flagEmbedConfig)
    if err != nil {
        return err
    }
    cfg, warnings, err := config.LoadEmbedding(cfgPath)
    if err != nil {
        return fmt.Errorf("embedding config (%s): %w", cfgPath, err)
    }
    for _, w := range warnings {
        fmt.Fprintf(os.Stderr, "mdx: embedding: %s\n", w)
    }

    dbPath, err := db.ResolvePath(flagDB)
    if err != nil {
        return err
    }
    conn, err := db.Open(dbPath)
    if err != nil {
        return err
    }
    defer conn.Close()
    if err := db.Migrate(conn); err != nil {
        return err
    }

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()

    _, err = cli.RunEmbed(ctx, conn, cfg, cli.EmbedOptions{
        Model: flagEmbedModel,
        All:   flagEmbedAll,
    })
    return err
}
```

Печать итоговой строки пока опущена — она появится в Шаге 8 вместе с реальной сводкой. Возврат `err` от заглушки приводит к ожидаемому ненулевому коду выхода с сообщением `embed: not implemented`.

**Готовность:** тесты `internal/config` зелёные; `mdx embed` с валидным конфигом в стандартном пути загружает конфиг, открывает БД, доходит до заглушки `cli.RunEmbed` и завершается с `embed: not implemented`; с битым конфигом печатает понятное сообщение об ошибке и завершается с ненулевым кодом до открытия БД.

### Шаг 3. Миграция таблицы `embeddings` (v1 → v2)

Добавить таблицу `embeddings` в схему БД и перевести `migrations.go` на поддержку миграции 1→2.

#### Изменения в `internal/db/schema.sql`

Дописать в конец:

```sql
CREATE TABLE embeddings (
  path          TEXT NOT NULL,
  model         TEXT NOT NULL,
  content_hash  TEXT NOT NULL,
  embedded_at   INTEGER NOT NULL,
  PRIMARY KEY (path, model),
  FOREIGN KEY (path) REFERENCES notes(path) ON DELETE CASCADE
);

CREATE INDEX idx_embeddings_model ON embeddings(model);
```

Индекс `idx_embeddings_model` ускоряет запросы вида «отдай все заметки, у которых нет вектора для модели X» — основной паттерн в Шаге 7.

#### Изменения в `internal/db/migrations.go`

Поднять `currentVersion = 2`. Заменить ветку «unsupported migration» на пошаговую миграцию:

```go
func upgradeSchema(conn *sql.DB) error {
    var have int
    if err := conn.QueryRow(
        `SELECT COALESCE(MAX(version), 0) FROM schema_version`,
    ).Scan(&have); err != nil {
        return fmt.Errorf("read version: %w", err)
    }
    for v := have + 1; v <= currentVersion; v++ {
        if err := applyMigration(conn, v); err != nil {
            return fmt.Errorf("migrate to v%d: %w", v, err)
        }
    }
    return nil
}

func applyMigration(conn *sql.DB, version int) error {
    tx, err := conn.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()
    switch version {
    case 2:
        if _, err := tx.Exec(migrationV2); err != nil {
            return err
        }
    default:
        return fmt.Errorf("no migration for v%d", version)
    }
    if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, version); err != nil {
        return err
    }
    return tx.Commit()
}

const migrationV2 = `
CREATE TABLE embeddings (
  path          TEXT NOT NULL,
  model         TEXT NOT NULL,
  content_hash  TEXT NOT NULL,
  embedded_at   INTEGER NOT NULL,
  PRIMARY KEY (path, model),
  FOREIGN KEY (path) REFERENCES notes(path) ON DELETE CASCADE
);
CREATE INDEX idx_embeddings_model ON embeddings(model);
`
```

DDL миграции `v2` идентична соответствующему фрагменту `schema.sql`. Дублирование принимается осознанно: `schema.sql` нужен для fresh-инсталла одним блоком, миграция — для существующих БД.

#### Тесты `internal/db/migrations` (расширить существующий `db_test.go`)

- Свежая БД: после `Migrate` присутствуют таблицы `notes`, `links`, `tags`, `embeddings`; `schema_version` содержит две строки (1 и 2) или одну со значением 2 — поведение фиксируется в зависимости от выбранной реализации `applyFreshSchema`. Если в `schema.sql` после изменения вставляется `version=2`, версия одна.
- БД, заранее приведённая к состоянию v1 (выполнены DDL `notes/links/tags`, в `schema_version` запись `1`): после `Migrate` таблица `embeddings` создана, `schema_version` содержит `1` и `2`.
- БД на текущей версии: повторный `Migrate` не выполняет DDL.

#### Корректировка `applyFreshSchema`

Записывает `version = currentVersion` (`2` после повышения), как и сейчас. Свежая БД сразу на актуальной схеме без промежуточных строк в `schema_version`.

**Готовность:** тесты `internal/db` зелёные. `mdx scan` на свежей и на старой БД обнаруживает таблицу `embeddings` (`sqlite3 mdx.db ".schema embeddings"`).

### Шаг 4. Слой db для таблицы `embeddings`

Реализовать запросы, которыми оркестратор отбирает кандидатов на embed и фиксирует результат.

#### Состав `internal/db/embeddings.go`

```go
type PendingNote struct {
    Path        string
    ContentHash string
    Title       sql.NullString
    Mtime       int64
    Frontmatter sql.NullString // raw JSON or YAML, передаётся в payload как есть
}

// PendingEmbeddings возвращает заметки, для которых для модели model нет
// записи в embeddings либо записанный content_hash не совпадает с текущим
// в notes. Если all=true, возвращает все заметки независимо от embeddings.
func PendingEmbeddings(ctx context.Context, conn *sql.DB, model string, all bool) ([]PendingNote, error)

// RecordEmbedding делает INSERT OR REPLACE в embeddings.
func RecordEmbedding(ctx context.Context, conn *sql.DB, path, model, contentHash string, embeddedAt int64) error
```

`PendingEmbeddings` — единственный SQL-запрос:

```sql
SELECT n.path, n.content_hash, n.title, n.mtime, n.frontmatter
FROM notes n
LEFT JOIN embeddings e
  ON e.path = n.path AND e.model = ?
WHERE ? = 1
   OR e.path IS NULL
   OR e.content_hash <> n.content_hash
```

Параметры: `(model, allFlag)`. При `all=true` второй параметр равен `1` и условие `WHERE` срабатывает безусловно; иначе — проверяет отсутствие или устаревание embedding.

`RecordEmbedding`:

```sql
INSERT INTO embeddings (path, model, content_hash, embedded_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(path, model) DO UPDATE SET
  content_hash = excluded.content_hash,
  embedded_at  = excluded.embedded_at
```

#### Тесты `internal/db/embeddings_test.go`

- На временной in-memory БД с парой записей в `notes`:
    - `PendingEmbeddings(model, all=false)` возвращает все при пустой `embeddings`;
    - после `RecordEmbedding` для одной из них возвращает только оставшуюся;
    - после изменения `content_hash` в `notes` для уже записанной — она снова появляется в результате;
    - `all=true` возвращает все независимо от состояния `embeddings`.
- `RecordEmbedding`: повторный вызов для той же `(path, model)` обновляет `content_hash` и `embedded_at`, не плодит дубликатов.
- Каскад при удалении `notes`: `DELETE FROM notes WHERE path=?` удаляет строку и из `embeddings`.

**Готовность:** тесты `internal/db` зелёные.

### Шаг 5. UUID v5 от пути

Выделить генерацию id точек Qdrant в отдельный модуль с фиксированным namespace.

#### Состав `internal/embed/id.go`

```go
package embed

import "github.com/google/uuid"

// pointNamespace is the fixed UUID v4 used as namespace for v5 ids.
// Generated once and treated as a constant; changing it invalidates all
// previously written points.
var pointNamespace = uuid.MustParse("REPLACE-WITH-GENERATED-UUID")

// PointID returns the deterministic UUID v5 for a note path.
func PointID(absPath string) uuid.UUID {
    return uuid.NewSHA1(pointNamespace, []byte(absPath))
}
```

`pointNamespace` генерируется однократно командой `uuidgen` (или эквивалентом) и зашивается в код. После этого его нельзя менять, иначе все ранее записанные точки станут «недостижимы» по новому id (новый id → новая точка → старая остаётся осиротевшей).

#### Тесты `internal/embed/id_test.go`

- Детерминированность: `PointID("/a/b.md") == PointID("/a/b.md")`;
- Различие по входу: разные пути дают разные UUID;
- Версия и вариант UUID: проверка `id.Version() == 5` и `id.Variant() == uuid.RFC4122`.

**Готовность:** тесты зелёные.

### Шаг 6. Клиент embedding API

Реализовать `Embed`, поддерживающий три варианта серверов одной точкой входа.

#### Состав `internal/embed/model.go`

```go
type ModelClient struct {
    cfg    config.ModelConfig
    http   *http.Client
}

func NewModelClient(cfg config.ModelConfig) *ModelClient

// Embed batches texts according to cfg.BatchSize and returns a vector for
// each input in the same order. prefix is applied to every text in the
// batch (used for document_prefix; query_prefix lives in the search path
// and is M1).
func (c *ModelClient) Embed(ctx context.Context, texts []string, prefix string) ([][]float32, error)
```

Внутри `Embed`:

1. Применяет `prefix` к каждому тексту: `prefix + text` (без разделителя — за разделитель отвечает сам префикс, см. пример Qwen3 в `embeddings.md`).
2. Нарезает на батчи по `cfg.BatchSize`.
3. Для каждого батча формирует тело по `cfg.EndpointKind`:
    - `openai`: `POST cfg.Endpoint`, тело `{"model": cfg.APIModelName, "input": [text...]}`, ответ `{"data":[{"embedding":[...], "index":N}, ...]}`. Перед возвратом отсортировать по `index` для устойчивости.
    - `llama-cpp`: для каждого текста отдельный `POST cfg.Endpoint`, тело `{"content": text}`, ответ `{"embedding": [...]}`. Server llama.cpp на момент написания не гарантирует batch на `/embedding`; запросы внутри батча всё равно последовательные, batch здесь — единица отчёта об ошибке.
    - `tei`: `POST cfg.Endpoint`, тело `{"inputs": [text...]}`, ответ — массив массивов float, в том же порядке.
4. Каждое тело отправляет `c.http.Do(...)`; на не-2xx — ошибка с кодом и первой строкой тела; на сетевые ошибки — обёрнутая ошибка с `cfg.Name`.
5. Собирает результат в `[][]float32` в порядке `texts`.

`http.Client` с таймаутом по умолчанию 60 секунд; конфигурируется константой пакета — отдельный конфиг-флаг по таймауту в M0 не вводится.

#### Тесты `internal/embed/model_test.go`

Через `httptest.NewServer` — три отдельных теста, по одному на `endpoint_kind`:

- `openai`: сервер отдаёт `data` с embedding-ами, проверяется корректное сопоставление и применение `prefix`.
- `llama-cpp`: сервер отдаёт `embedding` для каждого `content`, проверяется последовательная отправка и порядок результата.
- `tei`: сервер отдаёт массив массивов, проверяется размер batch.
- Кейсы ошибок: 500 от сервера → ошибка `Embed`; невалидный JSON в ответе → ошибка с указанием модели.

**Готовность:** тесты зелёные.

### Шаг 7. Клиент Qdrant

Реализовать создание коллекции, дозаливку именованных векторов и batch upsert.

#### Состав `internal/embed/qdrant.go`

```go
type QdrantClient struct {
    baseURL    string
    http       *http.Client
}

func NewQdrantClient(baseURL string) *QdrantClient

// EnsureCollection создаёт коллекцию, если её нет, и добавляет в неё
// именованные векторы для всех моделей из cfg.Models, которых там ещё нет.
func (q *QdrantClient) EnsureCollection(ctx context.Context, cfg config.EmbeddingConfig) error

type Point struct {
    ID      uuid.UUID
    Vectors map[string][]float32 // ключ — name модели
    Payload map[string]any
}

// Upsert загружает batch точек одним запросом.
func (q *QdrantClient) Upsert(ctx context.Context, collection string, points []Point) error
```

Поведение:

- `EnsureCollection`:
    1. `GET /collections/{collection}` — если 404, выполняет `PUT /collections/{collection}` с телом
       `{"vectors": {<modelName>: {"size": <dim>, "distance": "Cosine"}, ...}}`
       сразу со всеми моделями; завершение.
    2. Если 200, парсит ответ, собирает множество имеющихся `vectors`. Для каждой модели из `cfg.Models`, отсутствующей в множестве, выполняет `PATCH /collections/{collection}` (Qdrant `update_collection` API) с добавлением одного именованного вектора.
    3. Все ответы Qdrant ожидают `status: ok`; при ошибке — обёрнутая ошибка.
- `Upsert`:
    - `PUT /collections/{collection}/points?wait=true`, тело
      `{"points": [{"id": "<uuid>", "vector": {<modelName>: [...], ...}, "payload": {...}}, ...]}`.
    - Параметр `wait=true` гарантирует, что при возврате точка действительно записана — это важно для сценария «сразу после `mdx embed` запустить `mdx search`».

Payload точки на M0 (формируется оркестратором, не клиентом):

```json
{
  "path": "/abs/path/to/note.md",
  "title": "Title or null",
  "mtime": 1714000000,
  "content_hash": "sha256...",
  "frontmatter": "<raw frontmatter or null>"
}
```

`frontmatter` пишется как строка (raw YAML/JSON, как лежит в БД). На M0 не парсится в структурированный JSON в payload — это упрощает клиента, и для будущих filter-запросов будет добавлен второй проход. Если же фронтматтер уже хранится в БД как JSON, передаётся как JSON-объект напрямую — определяется по тому, что именно лежит в `notes.frontmatter`.

#### Тесты `internal/embed/qdrant_test.go`

Через `httptest.NewServer` имитирующий Qdrant:

- `EnsureCollection`: 404 → `PUT` с правильным телом → успех.
- `EnsureCollection`: 200 с уже существующей моделью → отсутствующие добавляются через `PATCH`; присутствующие не трогаются.
- `EnsureCollection`: ошибка 500 → ошибка с упоминанием collection.
- `Upsert`: тело запроса соответствует ожидаемому JSON; `wait=true` присутствует в URL; на 4xx/5xx — ошибка.

**Готовность:** тесты зелёные.

### Шаг 8. Оркестрация `mdx embed`

Связать конфиг, db-слой и клиентов в единый pipeline.

#### Состав `internal/cli/embed.go`

Тело `RunEmbed`, заглушенное в Шагах 1–2, наполняется реальной оркестрацией. Сигнатура и типы (`EmbedOptions`, `EmbedStats`) уже устаканены — не меняются:

```go
func RunEmbed(ctx context.Context, conn *sql.DB, cfg config.EmbeddingConfig, opts EmbedOptions) (EmbedStats, error)
```

Алгоритм:

1. Выбрать список моделей: если `opts.Model != ""` — найти модель по имени (ошибка, если нет); иначе — `cfg.Models`.
2. `qd := embed.NewQdrantClient(cfg.QdrantURL); qd.EnsureCollection(ctx, cfg)`. Все модели сразу — даже те, которые не запрашиваются опциями: коллекция держит схему всего конфига, чтобы между запусками не было «то одна модель есть, то нет».
3. Для каждой выбранной модели:
    - `pending, err := db.PendingEmbeddings(ctx, conn, model.Name, opts.All)`;
    - если `len(pending) == 0` — `stats.Skipped` остаётся; перейти к следующей модели;
    - инициализировать `client := embed.NewModelClient(model)`;
    - нарезать `pending` на батчи по `model.BatchSize`. Для каждого батча:
        - прочитать содержимое файлов с диска (`os.ReadFile(path)`); файлы, которые не читаются, исключаются из батча — `stats.Failed++` на каждый, ошибка в stderr;
        - вызвать `client.Embed(ctx, texts, model.DocumentPrefix)`. На ошибку — `stats.Failed += len(batch)`, ошибка в stderr, переход к следующему батчу;
        - сформировать `embed.Point` для каждой записи (id из `embed.PointID(path)`, payload из `db.PendingNote`);
        - вызвать `qd.Upsert(ctx, cfg.Collection, points)`. На ошибку — `stats.Failed += len(batch)`, ошибка в stderr, переход к следующему батчу;
        - после успешного upsert: `db.RecordEmbedding(ctx, conn, path, model.Name, contentHash, time.Now().Unix())` для каждой записи. Ошибка SQLite-записи — лог в stderr, `stats.Failed++`, но точка в Qdrant уже существует (см. транзакционность в технических решениях).
        - `stats.Embedded += len(batch успешных)`.
4. По завершении `stats.Elapsed = time.Since(start)`; вернуть.

`ctx` проверяется на каждой итерации внешнего цикла (по моделям и по батчам); при отмене — `RunEmbed` возвращает `ctx.Err()` с уже накопленными `stats`.

Импорты `cli/embed.go` после наполнения: стандартные (`context`, `database/sql`, `errors` или `fmt`, `os`, `time`) плюс `internal/config`, `internal/db`, `internal/embed`.

#### Тесты `internal/cli/embed_test.go`

E2E на временной SQLite + два `httptest.NewServer` (имитирующие embedding API и Qdrant):

- Свежая БД с тремя `notes`: `RunEmbed` вызывает `EnsureCollection`, делает три embed-вызова (или один с batch_size=3), три точки попадают в upsert. После — `embeddings` содержит три записи.
- Повторный `RunEmbed`: zero embed-вызовов, `stats.Skipped = 3`, `stats.Embedded = 0`.
- Изменение `content_hash` для одной записи: `RunEmbed` делает один embed/upsert; в `embeddings` обновлён `content_hash`.
- Сбой одного embedding-запроса (моковый сервер отдаёт 500): `stats.Failed > 0`, прогон не падает, остальные точки записаны.
- `opts.All = true`: пересчёт всех записей независимо от состояния `embeddings`.
- `opts.Model = "X"`, отсутствующее в конфиге: ошибка до выхода в pipeline.

#### Подключение в `runEmbed`

К версии из Шага 2 добавляется только печать итоговой строки; всё остальное остаётся прежним. Тело `runEmbed` после Шага 8:

```go
func runEmbed(cmd *cobra.Command, args []string) error {
    cfgPath, err := config.ResolveEmbeddingPath(flagEmbedConfig)
    if err != nil {
        return err
    }
    cfg, warnings, err := config.LoadEmbedding(cfgPath)
    if err != nil {
        return fmt.Errorf("embedding config (%s): %w", cfgPath, err)
    }
    for _, w := range warnings {
        fmt.Fprintf(os.Stderr, "mdx: embedding: %s\n", w)
    }

    dbPath, err := db.ResolvePath(flagDB)
    if err != nil {
        return err
    }
    conn, err := db.Open(dbPath)
    if err != nil {
        return err
    }
    defer conn.Close()
    if err := db.Migrate(conn); err != nil {
        return err
    }

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()

    stats, err := cli.RunEmbed(ctx, conn, cfg, cli.EmbedOptions{
        Model: flagEmbedModel,
        All:   flagEmbedAll,
    })
    if err != nil {
        return err
    }

    if !flagQuiet {
        fmt.Printf("embedded: %d, skipped: %d, failed: %d, elapsed: %s\n",
            stats.Embedded, stats.Skipped, stats.Failed, stats.Elapsed)
    }
    return nil
}
```

**Готовность:** тесты `internal/cli` (включая `embed_test.go`) и `internal/embed` зелёные. Ручная проверка с реальным Qdrant на `127.0.0.1:6333` и embedding-моделью на `127.0.0.1:8888`: после `mdx scan ~/notes` и `mdx embed` в коллекции `mdx` появляются точки; повторный `mdx embed` печатает `embedded: 0, skipped: N`.

### Шаг 9. README и пример конфига

Дополнить `lsp/README.md` (или соответствующий пользовательский README) разделом про `mdx embed`.

#### Что добавить

- Краткое описание команды и предусловия (`mdx scan` уже выполнен, доступны Qdrant и embedding-сервер).
- Пример минимального `embedding.yaml` для одной модели Qwen3-Embedding-4B на llama-server (или OpenAI-совместимом эндпойнте — по фактической установке пользователя).
- Перечень флагов `--embedding-config`, `--model`, `--all`, `-q` с однострочным описанием.
- Указание, что для удаления заметок из коллекции отдельной операции пока нет (см. open questions в `embeddings.md`); пересоздать коллекцию можно вручную через Qdrant API.

**Готовность:** README отвечает на вопрос «как запустить `mdx embed` в первый раз» в рамках одного экрана.

## Критерии готовности M0_embeddings

1. `go build ./...` и `go test ./...` зелёные на всём проекте.
2. На свежей БД `mdx scan` создаёт схему v2 (присутствует таблица `embeddings`); на БД, оставшейся от M5/предыдущей версии (v1), `mdx scan` или `mdx embed` выполняет миграцию 1→2 без потери данных в `notes`/`links`/`tags`.
3. `mdx embed --help` отображает флаги `--embedding-config`, `--model`, `--all`, `-q`.
4. `mdx embed` с валидным `~/.config/mdx/embedding.yaml`, поднятым Qdrant и поднятой embedding-моделью завершается с сообщением `embedded: N, skipped: 0, failed: 0, elapsed: T`. В Qdrant в коллекции `mdx` присутствует N точек с одним именованным вектором на сконфигурированную модель и payload-полями `path`, `title`, `mtime`, `content_hash`, `frontmatter`. В таблице `embeddings` — N строк.
5. Повторный `mdx embed` без изменений в `notes` завершается с `embedded: 0, skipped: N, failed: 0`; новых обращений к embedding-серверу не происходит (проверяется его логом).
6. После правки тела одной заметки и нового прогона `mdx scan`: `mdx embed` пересчитывает ровно одну точку (`embedded: 1, skipped: N-1`); в Qdrant соответствующий вектор обновлён, в `embeddings` — обновлён `content_hash` и `embedded_at`.
7. `mdx embed --all` пересчитывает все точки независимо от состояния `embeddings`.
8. `mdx embed --model X`, где `X` не объявлена в конфиге, завершается с ненулевым кодом и понятным сообщением до обращения к Qdrant.
9. При недоступности embedding-сервера или Qdrant `mdx embed` логирует ошибку в stderr и завершается с ненулевым кодом; БД и коллекция остаются в консистентном промежуточном состоянии (часть точек могла записаться — это допустимо, см. транзакционность).

После выполнения этих пунктов M0_embeddings закрыт, переходим к M1_embeddings (подкоманда `mdx search`).
