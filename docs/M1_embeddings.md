# M1_embeddings — поиск

## Цель

Появляется подкоманда `mdx search <query>`. Команда векторизует поисковый запрос той же моделью, которой ранее были embed-ированы заметки (с применением `query_prefix` модели), отправляет вектор в Qdrant `points/search` по соответствующему named vector и печатает список заметок, отсортированных по убыванию score. Поддерживаются два формата вывода: `text` (путь на строке) и `json` (массив объектов `{path, score, title}`).

После M1_embeddings становится возможен dense-поиск по корпусу из CLI. На этот фундамент M2_embeddings навешивает neovim-команду `:MdxSearch` с Telescope-picker'ом — она будет потреблять либо тот же `mdx search --format json`, либо новый LSP-метод (выбор делается в M2).

## Что входит и что не входит

### Входит

- Подкоманда `mdx search <query>`, регистрируемая в `cmd/mdx/main.go` рядом со `scan`/`gc`/`lsp`/`embed`. Принимает один или несколько позиционных аргументов; они склеиваются через пробел в строку запроса (чтобы не приходилось обрамлять кавычками каждый раз).
- Метод `Search` в Qdrant-клиенте `internal/embed/qdrant.go`: `POST /collections/{name}/points/search` с named-vector-телом и `with_payload: true`.
- Резолвер модели для поиска: `--model NAME` → именованная; иначе модель с `default_for_search: true`; иначе единственная модель в конфиге.
- Оркестрация в `internal/cli/search.go::RunSearch`: применить `query_prefix`, embed-ить запрос, отправить вектор в Qdrant, вернуть результаты.
- Форматирование вывода в `internal/cli/search.go`: `FormatText` (по строке на путь) и `FormatJSON` (массив `{path, score, title}`).
- Стандартные флаги: `--model NAME`, `--limit N` (default 20), `--format text|json` (default `text`), плюс наследуемый `--db` (хотя БД на этом шаге не используется — единообразие persistence-флагов сохраняем).
- Юнит-тесты на `Search` (через `httptest`), резолвер модели, форматтеры; интеграционный тест `RunSearch` end-to-end.

### Не входит (отложено)

- `:MdxSearch` в neovim-плагине — это M2_embeddings.
- Reranking top-N через cross-encoder — open question в `embeddings.md`.
- Filter-параметры (`frontmatter.type=task` и т.п.) — open question.
- Гибридный поиск (dense + sparse) — open question.
- Score threshold (`score_threshold`) — на M0–M1 любой результат, который вернул Qdrant в рамках `limit`, считается релевантным; обрезка по score откладывается до появления реальных кейсов «хвост шума».
- Кэш результатов запроса — поиск всегда идёт в живой Qdrant.
- Multi-model fan-out (запрос ко всем моделям сразу с RRF) — open question.

## Технические решения

| Вопрос | Решение |
|---|---|
| Резолв модели | `--model NAME` → имя; пусто → модель с `default_for_search: true`; иначе единственная модель в конфиге; иначе ошибка. Логика симметрична `selectModels` из `cli/embed.go`, но другая — там без default берутся все модели, здесь без default нужна ровно одна. |
| Применение `query_prefix` | Конкатенация `prefix + query` через тот же путь, что у документов в `embed.ModelClient.Embed`. Для Qwen3-Embedding и подобных asymmetric-моделей prefix несёт инструкцию; пустой prefix допустим (символметричные модели). |
| Search в Qdrant | `POST /collections/{name}/points/search`. Тело — `{"vector":{"name":"<model>","vector":[...]},"limit":N,"with_payload":true}`. Формат с явным `vector.name` обязателен для коллекций с named vectors. |
| Получение payload | `with_payload: true`. Payload компактный (path/title/mtime/content_hash/frontmatter), оптимизация через `with_payload: ["path","title"]` смысла не имеет. |
| Default limit | 20, задаётся как дефолт cobra-флага. В `RunSearch` стоит дополнительный guard `if limit <= 0 { limit = 20 }` — чтобы импортирующий пакет код мог опускать поле. |
| Многословный запрос | `cobra.MinimumNArgs(1)` + `strings.Join(args, " ")`. Заключать запрос в кавычки не обязательно. |
| Формат вывода | Два формата (`text`/`json`), реализуются как функции `FormatText` и `FormatJSON` в `cli/search.go`. `runSearch` в `cmd/mdx` выбирает функцию по флагу. Это симметрично с `runEmbed`, где саммари тоже печатается на уровне cmd. |
| Пустой результат | Пустой stdout (для `text`) или пустой массив `[]` (для `json`). Никаких сообщений типа «не найдено» — не нужно усложнять стрим вывода для пайпов. |
| Отсутствие выбранной модели | Ошибка возвращается до обращения к embedding API и Qdrant — `selectSearchModel` падает первой. |
| Пустой Qdrant / нет коллекции | Если `EnsureCollection` не вызывался (а на M1 он не вызывается), GET к Qdrant вернёт 404 → ошибка. Сценарий «искать до первого `mdx embed`» считается некорректным; ошибка должна быть понятной. |
| Score | Возвращается как `float32` (тип Qdrant). В JSON печатается как число; на M0–M1 не округляется. |
| Сортировка | Полагаемся на порядок Qdrant: results приходят отсортированными по убыванию score. Дополнительной сортировки на стороне клиента не делаем. |
| `RunSearch` и SQLite-соединение | `RunSearch` не принимает `*sql.DB` — поиск в M1 живёт целиком на стороне Qdrant и embedding API. SQLite потребуется только в случае reranking/filter, и это уже за пределами M1. |

## Структура каталогов проекта (что добавляется к M0_embeddings)

```
/home/oleg/projects/mdx/lsp/
├── cmd/mdx/
│   └── main.go                       ← регистрация подкоманды search, runSearch
└── internal/
    ├── embed/
    │   ├── qdrant.go                 ← + метод Search и связанные типы
    │   └── qdrant_test.go            ← + тесты Search
    └── cli/
        ├── search.go                 ← новый: SearchOptions, SearchHit, RunSearch, FormatText, FormatJSON
        └── search_test.go            ← новый: e2e RunSearch + юниты на форматтеры
```

## Шаги выполнения

Шаги упорядочены так, чтобы после каждого можно было запустить осмысленную проверку — либо `go test ./...`, либо ручной прогон `mdx search` с реальным Qdrant и embedding-сервером.

### Шаг 1. Скаффолдинг подкоманды

Зарегистрировать `mdx search` как cobra-команду со всеми флагами и создать заглушку `cli.RunSearch`.

#### Состав `cmd/mdx/main.go`

Добавить переменные флагов:

```go
var (
    flagSearchModel  string
    flagSearchLimit  int
    flagSearchFormat string
)
```

Объявить команду:

```go
var searchCmd = &cobra.Command{
    Use:   "search <query>...",
    Short: "Run dense search over the indexed corpus and print matching note paths",
    Args:  cobra.MinimumNArgs(1),
    RunE:  runSearch,
}
```

В `init()` зарегистрировать флаги и команду:

```go
searchCmd.Flags().StringVar(&flagSearchModel, "model", "",
    "model name from config (default: model with default_for_search=true)")
searchCmd.Flags().IntVar(&flagSearchLimit, "limit", 20,
    "maximum number of results")
searchCmd.Flags().StringVar(&flagSearchFormat, "format", "text",
    "output format: text or json")

rootCmd.AddCommand(searchCmd)
```

`runSearch` на этом шаге — заглушка: `return fmt.Errorf("search: not implemented")`. БД, конфиг и сетевые вызовы появляются в Шагах 4–5.

#### Состав `internal/cli/search.go`

```go
package cli

import (
    "context"
    "errors"

    "github.com/olegmif/mdx/lsp/internal/config"
)

// SearchOptions collects flags driving a single mdx search call.
type SearchOptions struct {
    Model string // empty = default model from config
    Limit int    // <=0 ⇒ defaultSearchLimit applied inside RunSearch
}

// SearchHit is one ranked match returned by RunSearch.
type SearchHit struct {
    Path  string  `json:"path"`
    Score float32 `json:"score"`
    Title string  `json:"title,omitempty"`
}

// RunSearch is filled in by Step 4.
func RunSearch(ctx context.Context, cfg config.EmbeddingConfig, query string, opts SearchOptions) ([]SearchHit, error) {
    return nil, errors.New("search: not implemented")
}
```

**Готовность:** `go build ./...` зелёное; `mdx search --help` показывает три флага и описание; `mdx search hello` завершается с ненулевым кодом и сообщением `search: not implemented`. Сети и БД на этом шаге не задействованы.

### Шаг 2. Метод `Search` в Qdrant-клиенте

Добавить метод поиска и связанные типы в `internal/embed/qdrant.go`. Расширить `qdrant_test.go` тестом на mock'е.

#### Состав `internal/embed/qdrant.go`

Тип результата:

```go
// SearchHit — одна точка-результат поиска. Payload приходит как
// произвольный объект; вызывающий код извлекает нужные поля.
type SearchHit struct {
    ID      string         // UUID точки
    Score   float32
    Payload map[string]any
}
```

Метод:

```go
// Search выполняет k-NN-поиск по named vector vectorName в коллекции
// collection, возвращает не более limit точек с payload. Без фильтров —
// расширение фильтрами идёт в open questions.
func (q *QdrantClient) Search(ctx context.Context, collection, vectorName string, vector []float32, limit int) ([]SearchHit, error)
```

Тело запроса:

```json
{
  "vector": {"name": "<vectorName>", "vector": [...]},
  "limit": <limit>,
  "with_payload": true
}
```

Эндпойнт: `POST /collections/{collection}/points/search`.

Парсинг ответа:

```go
type searchResponse struct {
    Result []struct {
        ID      any            `json:"id"`
        Score   float32        `json:"score"`
        Payload map[string]any `json:"payload"`
    } `json:"result"`
}
```

`ID` декодируется как `any`, потому что Qdrant умеет возвращать как UUID-строку, так и uint64; на M0–M1 наши id всегда UUID, но защищаемся от регрессии типа. Метод приводит `id` к строке через `fmt.Sprintf("%v", id)`.

Ошибки и проверка `status: ok` — через тот же `doJSON`-путь, что в M0_embeddings; для возврата результата читать тело придётся отдельно. Чтобы не изобретать второй транспорт, выделяется `doJSONReply(ctx, method, path, body, op, out)` (вариант `doJSON` с декодом результата в произвольный `out`), а текущий `doJSON` делегирует в `doJSONReply` с пропуском декодирования через `*ackOnly` декодер. Конкретная форма рефакторинга — деталь реализации.

#### Тесты `internal/embed/qdrant_test.go`

Через `httptest.NewServer`:

- Сервер по `POST /collections/mdx/points/search` декодирует тело, проверяет `vector.name`, `vector.vector`, `limit`, `with_payload`, отдаёт `result` с двумя точками.
- `Search` возвращает срез из двух элементов с правильными score и payload, в порядке прихода (сортировку Qdrant клиент не делает).
- Кейс ошибки: 500 → ошибка с упоминанием collection и кода.

**Готовность:** тесты `internal/embed/qdrant_test.go` зелёные. Метод корректно сериализует named-vector-форму запроса.

### Шаг 3. Резолвер модели для поиска

Реализовать выбор модели по правилам M1: явный `--model` → `default_for_search` → единственная модель → ошибка.

#### Состав `internal/cli/search.go`

```go
func selectSearchModel(cfg config.EmbeddingConfig, name string) (config.ModelConfig, error)
```

Поведение:

1. Если `name != ""` — линейный поиск по `cfg.Models`; не нашли → ошибка `model %q not found in embedding config`.
2. Иначе — найти модель с `DefaultForSearch == true`. Нашли → вернуть.
3. Иначе — если `len(cfg.Models) == 1`, вернуть единственную.
4. Иначе — ошибка `no default search model configured` (этот кейс отсекается валидацией конфига при `len(Models) > 1`, но guard оставляем для надёжности).

#### Тесты в `internal/cli/search_test.go`

- одна модель без `default_for_search`, `name=""` → возвращена эта модель;
- одна модель с `default_for_search`, `name=""` → возвращена;
- две модели, одна с `default_for_search`, `name=""` → возвращена помеченная как default;
- две модели, явный `name="m2"` → возвращена `m2`;
- две модели, `name="ghost"` → ошибка с подстрокой `"ghost"`.

**Готовность:** табличный тест зелёный.

### Шаг 4. Оркестрация `RunSearch`

Связать резолвер модели, embedding-клиент и Qdrant-клиент.

#### Состав `internal/cli/search.go`

```go
const defaultSearchLimit = 20

func RunSearch(ctx context.Context, cfg config.EmbeddingConfig, query string, opts SearchOptions) ([]SearchHit, error) {
    // 1. Резолв модели.
    // 2. Векторизация query через embed.NewModelClient(model).Embed(ctx, []string{query}, model.QueryPrefix).
    //    Длина результата проверяется (== 1).
    // 3. limit := opts.Limit; if limit <= 0 { limit = defaultSearchLimit }
    // 4. qd := embed.NewQdrantClient(cfg.QdrantURL); qdHits, err := qd.Search(ctx, cfg.Collection, model.Name, vec, limit).
    // 5. Преобразовать []embed.SearchHit → []cli.SearchHit, доставая Path/Title из Payload.
    //    Поля payload — string'и (mtime — int/float). Path обязателен; если его нет — это сигнал
    //    несинхронизированной коллекции, точка пропускается с записью WARN в stderr.
}
```

Извлечение payload:

```go
func payloadString(p map[string]any, key string) string {
    if v, ok := p[key]; ok {
        if s, ok := v.(string); ok {
            return s
        }
    }
    return ""
}
```

`Title` — допустимо отсутствие; пустая строка попадает в `SearchHit.Title` и в JSON-выводе omitempty прячет поле.

#### Подключение в `runSearch`

```go
func runSearch(cmd *cobra.Command, args []string) error {
    cfgPath, err := config.ResolveEmbeddingPath(flagEmbedConfig)
    if err != nil {
        return err
    }
    cfg, _, err := config.LoadEmbedding(cfgPath)
    if err != nil {
        return fmt.Errorf("embedding config (%s): %w", cfgPath, err)
    }

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()

    query := strings.Join(args, " ")
    hits, err := cli.RunSearch(ctx, cfg, query, cli.SearchOptions{
        Model: flagSearchModel,
        Limit: flagSearchLimit,
    })
    if err != nil {
        return err
    }

    return printSearchHits(hits, flagSearchFormat)
}
```

`printSearchHits` появляется в Шаге 5.

`flagEmbedConfig` переиспользуется — это тот же путь к `embedding.yaml`, что у `mdx embed`. БД для M1 не открывается: search не трогает SQLite.

**Готовность:** `RunSearch` с моковыми Qdrant и embedding API возвращает корректный список `SearchHit`. Юнит-тест на распаковку payload (отдельно от e2e, см. Шаг 6).

### Шаг 5. Форматирование вывода

Реализовать функции форматирования и переключатель в `cmd/mdx`.

#### Состав `internal/cli/search.go`

```go
// FormatText returns one path per line, no header. Empty result → empty string.
func FormatText(hits []SearchHit) string

// FormatJSON returns a JSON array of hits ([{"path","score","title"}, ...]).
// Empty result → "[]\n".
func FormatJSON(hits []SearchHit) ([]byte, error)
```

`FormatText`: цикл с `strings.Builder`, в конце каждой строки — `\n`. Score и title в text-формате не печатаются: для конвейеров с `xargs`/`fzf`/`vim -r` нужен только путь.

`FormatJSON`: `json.MarshalIndent(hits, "", "  ")` (или просто `json.Marshal`; форматирование — деталь). Структура `SearchHit` уже несёт JSON-теги (`path`/`score`/`title,omitempty`).

#### Состав `cmd/mdx/main.go`

```go
func printSearchHits(hits []cli.SearchHit, format string) error {
    switch format {
    case "text":
        os.Stdout.WriteString(cli.FormatText(hits))
        return nil
    case "json":
        data, err := cli.FormatJSON(hits)
        if err != nil {
            return err
        }
        os.Stdout.Write(append(data, '\n'))
        return nil
    default:
        return fmt.Errorf("unknown --format %q (text|json)", format)
    }
}
```

Неизвестный формат отвергается явной ошибкой.

#### Тесты `internal/cli/search_test.go`

- `FormatText`: три hits → ровно `path1\npath2\npath3\n`; пустой срез → `""`.
- `FormatJSON`: три hits → парсится обратно в `[]SearchHit`, поля совпадают; hit без title не содержит ключа `"title"` (omitempty работает).

**Готовность:** юниты на форматтеры зелёные.

### Шаг 6. End-to-end тест `RunSearch`

Полный сценарий с mock'ами Qdrant и embedding API.

#### Состав `internal/cli/search_test.go`

E2E через два `httptest.NewServer`:

- Embedding-сервер: принимает `{"input":["Q: hello"]}` (с применённым `query_prefix`), возвращает один embedding длины N.
- Qdrant-сервер: на `POST /collections/mdx/points/search` декодирует тело, проверяет `vector.name == "m1"`, `limit == 5`, `with_payload == true`, отдаёт три точки с payload `{path, title}` и убывающими score.
- `RunSearch(...)` возвращает срез из трёх `SearchHit` в правильном порядке. Проверяется, что path и title подняты из payload.

Дополнительные кейсы (на тех же mock'ах):

- `--limit` не задан (`Limit: 0` в опциях) → клиент Qdrant получает `limit: 20`.
- Несуществующая модель → ошибка до обращения к embedding API и Qdrant (mocks делают `t.Fatal` в handler'е, чтобы доказать, что они не вызваны).

**Готовность:** `go test ./internal/cli/...` зелёный, включая новые тесты.

### Шаг 7. README и ручная проверка

Добавить в `lsp/README.md` раздел про `mdx search`.

#### Что добавить

- Краткое описание команды и предусловия (`mdx embed` уже отработал хотя бы для одной модели; коллекция и точки в Qdrant созданы).
- Пример: `mdx search "qdrant configuration"` и `mdx search --format json "qdrant configuration" | jq '.[0]'`.
- Перечень флагов: `--model`, `--limit`, `--format`, наследуемый `--db` (бесполезен на этом шаге, но единообразен с другими подкомандами).
- Указание, что выбор модели по умолчанию идёт через `default_for_search` в конфиге; поведение при множестве моделей без default — конфиг отвергается ещё в `LoadEmbedding`.

#### Ручная проверка

С реальным Qdrant и embedding-сервером:

```
mdx search "tag tags"
mdx search --limit 5 "qdrant configuration"
mdx search --format json --limit 3 "embeddings strategy"
```

Каждый запуск должен вернуть осмысленные пути к заметкам с убывающим score. Полное отсутствие результатов — пустой stdout (не ошибка).

**Готовность:** README отвечает на вопрос «как запустить `mdx search` в первый раз» в рамках одного экрана; ручная проверка на живом корпусе показывает разумные результаты.

## Критерии готовности M1_embeddings

1. `go build ./...` и `go test ./...` зелёные на всём проекте.
2. `mdx search --help` отображает флаги `--model`, `--limit`, `--format` и принимает многословный позиционный запрос.
3. `mdx search "<query>"` с валидным `embedding.yaml`, поднятым Qdrant и embedding-сервером возвращает до 20 путей по строке на убывающем score.
4. `--limit N` усекает количество результатов до N.
5. `--format json` печатает корректный JSON-массив, парсящийся через `jq`. Поле `title` отсутствует в записях, у которых заметка не имеет title.
6. `--model X` для несуществующей модели завершается с ненулевым кодом и сообщением, упоминающим имя `X`. Ни embedding-сервер, ни Qdrant при этом не дёргаются.
7. Поиск по коллекции с одной моделью и без `default_for_search: true` срабатывает (единственная модель выбирается автоматически).
8. Поиск без аргументов отвергается cobra с usage-ошибкой.
9. Пустой результат поиска — пустой stdout для `text` и `[]` для `json`; ошибки нет.

После выполнения этих пунктов M1_embeddings закрыт, переходим к M2_embeddings (neovim-команда `:MdxSearch` с Telescope).
