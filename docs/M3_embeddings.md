# M3_embeddings — gc для Qdrant

## Цель

Расширение подкоманды `mdx gc`: после очистки локальной SQLite-БД дополнительно вычистить коллекцию Qdrant от точек, у которых в `payload.path` указан путь, отсутствующий в таблице `notes` после фазы mdx-DB. Недоступность Qdrant или отсутствие `embedding.yaml` не приводят к падению `mdx gc` и не блокируют очистку SQLite — это вторичный, best-effort этап.

После M3_embeddings состояние Qdrant сходится с состоянием SQLite mdx после каждого `mdx gc`. Точки, оставшиеся от давно удалённых заметок (включая снятые с диска или попавшие под `ignore`), исчезают из коллекции; точки, оказавшиеся в Qdrant иным путём (ручная вставка, остатки старого корпуса) и не имеющие соответствующей строки в `notes`, тоже удаляются.

## Что входит и что не входит

### Входит

- Методы `Scroll` и `DeletePoints` в `internal/embed/qdrant.go` с тестами на `httptest`.
- Расширение `internal/cli/gc.go`: новая функция `cleanQdrant`, новые поля в `GCStats`, расширение сигнатуры `RunGC` опциональным `*config.EmbeddingConfig`.
- Подключение в `cmd/mdx/main.go::runGC`: добавление флага `--embedding-config` к `gcCmd`, опциональная загрузка конфига, передача в `RunGC`.
- E2E-тесты `RunGC` с моком Qdrant: успешный сценарий, недоступный Qdrant, отсутствующий `embedding.yaml`.
- Раздел про новое поведение в `lsp/README.md`.

### Не входит (отложено)

- Per-model gc (удаление только точек указанной модели) — open question, бессмысленно при текущей one-collection-многоvector-схеме.
- Отдельная подкоманда `mdx qdrant-gc` без участия SQLite — не нужно, gc остаётся одной точкой входа.
- Обратная синхронизация (`notes` есть, Qdrant пуст) — это область `mdx embed`, не gc.
- Параллельный scroll/delete — последовательно, как и в M0_embeddings.
- Прогресс-бар или потоковый вывод хода удаления — на текущем объёме корпуса не нужно.
- Пересоздание коллекции, очистка по фильтру `payload.path` через Qdrant `delete by filter` — реализация идёт по UUID-ID (см. таблицу решений).

## Технические решения

| Вопрос | Решение |
|---|---|
| Порядок этапов | Сначала фаза mdx-DB (текущее поведение `RunGC`): сбор orphan-paths, транзакционное удаление, `COMMIT`. Только после успешного `COMMIT` — фаза Qdrant. Падение фазы mdx-DB → ранний `return` с ошибкой, Qdrant не трогается. |
| Ошибка фазы Qdrant | Любая ошибка (сетевая, HTTP non-2xx, парсинг ответа, отсутствие коллекции) выводится как `mdx: gc: qdrant: <op>: <err>` в stderr и фиксируется в `GCStats.QdrantFailed = true`. `RunGC` возвращает `nil`, если фаза mdx-DB прошла; exit code `mdx gc` остаётся 0. |
| Отсутствие `embedding.yaml` | `errors.Is(err, fs.ErrNotExist)` → фаза Qdrant тихо пропускается, `GCStats.QdrantSkipped = true`. Любая другая ошибка чтения/парсинга — WARN в stderr, фаза пропускается, `QdrantSkipped = true`. Симметрично сценарию «у пользователя нет embedding-подсистемы». |
| Передача конфига в `RunGC` | Через дополнительный параметр `embedCfg *config.EmbeddingConfig`. `nil` ⇒ Qdrant-фаза пропускается. Резолв и загрузка делаются в `cmd/mdx/main.go::runGC`; `internal/cli` не знает про пути и переменные окружения. |
| Источник набора «должных остаться» | После фазы mdx-DB читается `SELECT path FROM notes` целиком в `map[string]struct{}`. Чтение перед фазой mdx-DB неприменимо: orphan-paths уже не должны быть в Qdrant. |
| Источник набора Qdrant-точек | Полный scroll коллекции с `with_payload: ["path"]`, `with_vector: false`. Pagination через `next_page_offset` до `null`. |
| Размер batch'а Scroll | `limit: 1000` точек на запрос. На текущем корпусе (≈ 5k заметок) — 1–6 запросов. |
| Точки без `path` в payload | Считаются orphan. Payload-схема mdx (M0_embeddings) гарантирует наличие `path` у всех собственных точек; пропуск ключа — сигнал ручной вставки или сбоя `mdx embed`. Удалять без ожидания ручного разбирательства. |
| Удаление | `POST /collections/{name}/points/delete?wait=true` со списком UUID-строк. Без чанкинга: типичный объём orphan-точек на проход — единицы/десятки. При появлении кейсов в сотнях тысяч пересмотреть. |
| `wait=true` | Чтобы summary `qdrant_removed: N` отражал фактически удалённое, а не «принятое в очередь». |
| ID точки | Возвращается Qdrant как UUID-строка. Без сверки с `embed.PointID(path)`: если рассинхрон есть, удаляем по факту присутствия в коллекции. |
| Флаг `--embedding-config` у `gc` | Добавляется по образцу `embed`/`search`, шарит переменную `flagEmbedConfig` (та же, что у других подкоманд). Без флага — резолв через `MDX_EMBEDDING_CONFIG`/`XDG_CONFIG_HOME`/`~/.config/mdx/embedding.yaml`. |
| Поля `GCStats` | Добавляются `QdrantDeleted int`, `QdrantKept int`, `QdrantSkipped bool`, `QdrantFailed bool`. Без per-model breakdown: коллекция одна, операция удаления — атомарна для всех named vectors точки. |
| Формат summary | Текущая строка `removed: N, kept: M, elapsed: T` дополняется хвостом `, qdrant_removed: K, qdrant_kept: L`. Если `QdrantSkipped` — хвост `, qdrant: skipped`; если `QdrantFailed` — `, qdrant: error` (детали уже в stderr WARN). |
| Тесты | Unit-тесты `Scroll` и `DeletePoints` через `httptest` (по образцу `qdrant_test.go`). E2E на `RunGC`: (a) happy path с моком Qdrant — mdx-DB и коллекция почищены, статистика корректна; (b) Qdrant закрыт до вызова — mdx-DB всё равно почищена, `QdrantFailed = true`, ошибки нет; (c) `embedCfg = nil` — Qdrant не трогается, `QdrantSkipped = true`. |

## Структура каталогов проекта (что добавляется к M2_embeddings)

```
/home/oleg/projects/mdx/lsp/
├── cmd/mdx/
│   └── main.go                       ← + флаг --embedding-config у gcCmd, резолв конфига, проброс в cli.RunGC
└── internal/
    ├── embed/
    │   ├── qdrant.go                 ← + методы Scroll и DeletePoints, связанные типы запросов/ответов
    │   └── qdrant_test.go            ← + тесты Scroll (с pagination) и DeletePoints
    └── cli/
        ├── gc.go                     ← + cleanQdrant, расширение GCStats, расширение сигнатуры RunGC
        └── gc_test.go                ← + e2e: happy path / qdrant down / no embedding config
```

Ни один файл вне `cmd/mdx`, `internal/embed` и `internal/cli` не правится. Lua-плагин не задействован.

## Шаги выполнения

Шаги упорядочены так, чтобы после каждого можно было запустить осмысленную проверку — `go test ./...` либо ручной прогон `mdx gc` с поднятым/выключенным Qdrant.

### Шаг 1. Метод `Scroll` в Qdrant-клиенте

Добавить в `internal/embed/qdrant.go` метод pagination'ого обхода точек коллекции, возвращающий минимальный для gc'я набор полей.

#### Состав `internal/embed/qdrant.go`

Тип результата:

```go
// ScrollPoint — одна точка в выдаче Scroll. Payload сужен до того,
// что нужно gc'ю; полный payload остаётся в коллекции нетронутым.
type ScrollPoint struct {
    ID   string
    Path string
}
```

Метод:

```go
// Scroll итерирует все точки коллекции, возвращая их id и payload.path.
// Внутри делает несколько HTTP-запросов с pagination через next_page_offset.
// batchSize задаёт размер одной страницы; <=0 трактуется как 1000.
func (q *QdrantClient) Scroll(ctx context.Context, collection string, batchSize int) ([]ScrollPoint, error)
```

Тело запроса:

```json
{
  "limit": <batchSize>,
  "with_payload": ["path"],
  "with_vector": false,
  "offset": <next_page_offset|null>
}
```

Эндпойнт: `POST /collections/{collection}/points/scroll`. Поле `offset` опускается в первом запросе и подставляется из `result.next_page_offset` в последующих; цикл завершается, когда сервер вернул `null`.

Парсинг ответа:

```go
type scrollResponse struct {
    Result struct {
        Points []struct {
            ID      any            `json:"id"`
            Payload map[string]any `json:"payload"`
        } `json:"points"`
        NextPageOffset any `json:"next_page_offset"`
    } `json:"result"`
}
```

`ID` декодируется как `any` (Qdrant возвращает либо UUID-строку, либо uint64); приводится к строке через `fmt.Sprintf("%v", id)`. `payload.path` извлекается через type assertion к `string`; пустая/отсутствующая строка кладётся в `ScrollPoint.Path = ""`.

#### Тесты в `internal/embed/qdrant_test.go`

Через `httptest.NewServer`:

- Сервер на `POST /collections/mdx/points/scroll`: первый запрос (без `offset`) отдаёт две точки и `next_page_offset: "p2"`; второй (с `offset: "p2"`) — одну точку и `next_page_offset: null`. `Scroll` возвращает срез из трёх точек, `id` и `path` извлечены корректно, в правильном порядке.
- Кейс ошибки: 500 → ошибка с упоминанием collection.
- Кейс «точка без path»: `payload` не содержит ключа `path` → `ScrollPoint.Path == ""`.

**Готовность:** новые тесты в `internal/embed/qdrant_test.go` зелёные. Метод корректно склеивает страницы и не лезет в сеть после первого ответа с `next_page_offset: null`.

### Шаг 2. Метод `DeletePoints` в Qdrant-клиенте

Добавить в `internal/embed/qdrant.go` метод удаления точек по списку id.

#### Состав `internal/embed/qdrant.go`

```go
// DeletePoints удаляет точки коллекции по их id. wait=true — чтобы
// вызывающий код мог сразу полагаться на новое состояние коллекции.
// Пустой ids — no-op без обращения к серверу.
func (q *QdrantClient) DeletePoints(ctx context.Context, collection string, ids []string) error
```

Тело запроса: `{"points": ["uuid-1", "uuid-2", ...]}`. Эндпойнт: `POST /collections/{collection}/points/delete?wait=true`. Транспорт — существующий `doJSON` (ack-only).

#### Тесты в `internal/embed/qdrant_test.go`

- Сервер на `POST /collections/mdx/points/delete`: декодирует тело, проверяет совпадение списка ids; отвечает `{"result":{},"status":"ok"}`. `DeletePoints` возвращает `nil`.
- Пустой `ids` → метод возвращает `nil`, сервер не дёргается (handler с `t.Fatal`).
- 500 → ошибка с упоминанием collection и кода.

**Готовность:** новые тесты на `DeletePoints` зелёные.

### Шаг 3. Расширение `GCStats` и сигнатуры `RunGC`

Подготовить `internal/cli/gc.go` к добавлению Qdrant-фазы: расширить `GCStats`, добавить `embedCfg *config.EmbeddingConfig` в сигнатуру `RunGC`. На этом шаге логика Qdrant ещё не подключается; `embedCfg` не используется. Существующее поведение mdx-DB-фазы сохраняется.

#### Состав `internal/cli/gc.go`

```go
type GCStats struct {
    Deleted        int           // notes rows removed
    Kept           int           // notes rows that survived
    QdrantDeleted  int           // qdrant points removed
    QdrantKept     int           // qdrant points kept
    QdrantSkipped  bool          // фаза qdrant не запускалась (нет конфига)
    QdrantFailed   bool          // фаза qdrant запускалась и упала
    Elapsed        time.Duration
}

func RunGC(ctx context.Context, conn *sql.DB, ignorePrefixes []string, embedCfg *config.EmbeddingConfig) (GCStats, error)
```

`embedCfg == nil` ⇒ `stats.QdrantSkipped = true`, фаза не выполняется (полная логика Qdrant — Шаги 4–5).

Все вызывающие точки (`cmd/mdx/main.go::runGC`, существующие тесты `gc_test.go`) обновляются: добавляется `nil` в позиции нового параметра. Существующие тесты должны продолжать проходить без модификаций иначе.

**Готовность:** `go build ./...` и `go test ./...` зелёные; `mdx gc` ведёт себя как до M3 (фаза Qdrant не выполняется, `QdrantSkipped = true` фиксируется в статистике, summary остаётся прежним).

### Шаг 4. Функция `cleanQdrant`

В `internal/cli/gc.go` добавить функцию, которая принимает текущий набор `notes.path` и инструкции для Qdrant'а, и возвращает счётчики удалённых/сохранённых точек либо ошибку. Все ошибки логируются как WARN в stderr и не пропагируются наружу; статистика отражает последнюю успешную часть прохода.

```go
// cleanQdrant выполняет Qdrant-фазу gc'я: scroll коллекции, diff с
// notesPaths, удаление orphan-точек. Все ошибки выводятся в stderr и
// конвертируются в (deleted, kept, qdrantFailed=true). Не возвращает
// error — вызывающий код всегда продолжает работу.
func cleanQdrant(ctx context.Context, cfg config.EmbeddingConfig, notesPaths map[string]struct{}) (deleted, kept int, failed bool)
```

Поведение по шагам:

1. Создаётся `embed.NewQdrantClient(cfg.QdrantURL)`.
2. Вызывается `Scroll(ctx, cfg.Collection, 1000)`. Ошибка → WARN, ранний `return 0, 0, true`.
3. Для каждой точки: `Path == "" || _, ok := notesPaths[Path]; !ok` ⇒ id в orphan-список; иначе `kept++`.
4. Если orphan-список пуст — `return 0, kept, false`.
5. `DeletePoints(ctx, cfg.Collection, orphans)`. Ошибка → WARN с подсчётом `len(orphans)`, `return 0, kept, true`.
6. Успех → `return len(orphans), kept, false`.

Формат WARN'а: `mdx: gc: qdrant: <op>: <err>` (например, `mdx: gc: qdrant: scroll: ...`).

**Готовность:** функция компилируется, на этом шаге ещё не вызывается; вызов появляется в Шаге 5.

### Шаг 5. Подключение `cleanQdrant` в `RunGC` и обновление summary

В `RunGC` после успешного `COMMIT` mdx-DB фазы:

1. Если `embedCfg == nil` — `stats.QdrantSkipped = true`, переход к финализации.
2. Иначе — `notesPaths`, полученный через `SELECT path FROM notes` (после удаления orphan'ов), складывается в `map[string]struct{}`. Ошибка чтения — WARN, `stats.QdrantFailed = true`, переход к финализации.
3. Вызов `cleanQdrant(ctx, *embedCfg, notesPaths)`; результат пишется в `stats.QdrantDeleted/QdrantKept`, `stats.QdrantFailed` накапливается логическим OR.

В `cmd/mdx/main.go::runGC` функция вывода summary дополняется:

- если `QdrantSkipped` — хвост `, qdrant: skipped`;
- иначе если `QdrantFailed` — хвост `, qdrant: error`;
- иначе — хвост `, qdrant_removed: %d, qdrant_kept: %d`.

`-q`/`--quiet` подавляет всю строку, как сейчас.

**Готовность:** на mock-стенде с поднятым httptest-Qdrant'ом `RunGC` корректно удаляет orphan-точки и отражает статистику; ручной прогон `mdx gc` без поднятого Qdrant'а печатает `qdrant: error` без ненулевого exit-кода.

### Шаг 6. Подключение `--embedding-config` в `cmd/mdx/runGC`

Добавить флаг `--embedding-config` к `gcCmd` (общий с `embed`/`search`, через ту же переменную `flagEmbedConfig`). В `runGC` зарезолвить путь и попытаться загрузить конфиг.

#### Состав `cmd/mdx/main.go`

```go
gcCmd.Flags().StringVar(&flagEmbedConfig, "embedding-config", "",
    "path to embedding config (default: $XDG_CONFIG_HOME/mdx/embedding.yaml)")
```

В `runGC`:

```go
var embedCfg *config.EmbeddingConfig
cfgPath, err := config.ResolveEmbeddingPath(flagEmbedConfig)
if err == nil {
    cfg, _, loadErr := config.LoadEmbedding(cfgPath)
    switch {
    case loadErr == nil:
        embedCfg = &cfg
    case errors.Is(loadErr, fs.ErrNotExist):
        // конфига нет — Qdrant-фаза тихо пропустится в RunGC
    default:
        fmt.Fprintf(os.Stderr, "mdx: gc: embedding config (%s): %v\n", cfgPath, loadErr)
    }
}
```

Затем `embedCfg` передаётся в `cli.RunGC(ctx, conn, ignorePrefixes, embedCfg)`.

**Готовность:** `mdx gc --help` показывает `--embedding-config`; на машине без `embedding.yaml` `mdx gc` молча пропускает Qdrant-фазу (`qdrant: skipped` в summary); на машине с конфигом — выполняется или печатает `qdrant: error`/`qdrant_removed: N, qdrant_kept: M`.

### Шаг 7. End-to-end тесты `RunGC`

Расширить `internal/cli/gc_test.go` тремя кейсами с моком Qdrant.

Состав:

- **happy path с Qdrant.** Поднимается `httptest.Server`, реализующий `POST /collections/mdx/points/scroll` (отдающий три точки: одна — путь существующего файла из notes, две — пути несуществующих файлов) и `POST /collections/mdx/points/delete` (накапливает удалённые ids). В `RunGC` передаётся непустой `embedCfg` с `QdrantURL = httptest.URL`. После прогона: `stats.Deleted == 2` (mdx-DB), `stats.QdrantDeleted == 2`, `stats.QdrantKept == 1`, `stats.QdrantFailed == false`, `stats.QdrantSkipped == false`. Mock зафиксировал ровно два delete-id.
- **Qdrant недоступен.** httptest-сервер не поднимается; `embedCfg.QdrantURL` указывает на свободный порт (`http://127.0.0.1:1`). `RunGC` возвращает `nil`, `stats.QdrantFailed == true`, `stats.QdrantSkipped == false`, mdx-DB вычищена корректно.
- **Без `embedCfg`.** Передаётся `nil`. `stats.QdrantSkipped == true`, `stats.QdrantFailed == false`, `stats.QdrantDeleted == 0`. mdx-DB вычищена корректно.

**Готовность:** `go test ./internal/cli/...` зелёный, включая новые подтесты.

### Шаг 8. README и ручная проверка

Обновить раздел `## Garbage-collect orphan rows` в `lsp/README.md`.

#### Что добавить

- Описание новой Qdrant-фазы и условий её запуска (резолв `embedding.yaml` по тем же правилам, что у `mdx embed`).
- Пункт «Qdrant unavailability is non-fatal»: при недоступности сервера или коллекции `mdx gc` всё равно вычищает SQLite, печатает WARN в stderr и завершается с кодом 0. В summary хвост — `qdrant: error`.
- Пункт «no embedding config — phase skipped»: если `embedding.yaml` не найден ни по флагу, ни по `MDX_EMBEDDING_CONFIG`, ни по XDG/`~/.config/mdx`, фаза Qdrant пропускается тихо (`qdrant: skipped`).
- Новый флаг `--embedding-config`.
- Обновлённый пример summary-строки.

#### Ручная проверка

С поднятыми Qdrant и mdx-БД (после реального `mdx embed` с заметками, часть которых физически удалена с диска):

```
mdx gc                        # с поднятым Qdrant'ом
systemctl --user stop qdrant  # симулировать недоступность
mdx gc                        # та же команда, Qdrant недоступен
rm ~/.config/mdx/embedding.yaml
mdx gc                        # без embedding-конфига
```

Каждый прогон должен завершаться кодом 0; summary отражает соответствующее состояние Qdrant-фазы.

**Готовность:** README отвечает на вопрос «что произойдёт, если Qdrant выключен в момент `mdx gc`» в одном абзаце; ручная проверка на живом стенде показывает три предсказуемых summary.

## Критерии готовности M3_embeddings

1. `go build ./...` и `go test ./...` зелёные на всём проекте.
2. `mdx gc --help` показывает флаг `--embedding-config`.
3. `mdx gc` на стенде с поднятым Qdrant и заметками, удалёнными с диска, корректно удаляет соответствующие строки из `notes` и точки из коллекции; summary содержит `qdrant_removed: N, qdrant_kept: M`.
4. `mdx gc` при выключенном Qdrant'е (или коллекции, которой нет) завершается с кодом 0; summary содержит `qdrant: error`; mdx-DB вычищена.
5. `mdx gc` на машине без `embedding.yaml` завершается с кодом 0; summary содержит `qdrant: skipped`; никаких WARN'ов про Qdrant в stderr нет.
6. Точки в Qdrant, чей `payload.path` не присутствует в `notes` после фазы mdx-DB, удаляются — даже если их в Qdrant положили иным способом, не через `mdx embed` для текущего корпуса.
7. Точки, у которых в payload отсутствует ключ `path`, удаляются как orphan'ы.
8. После выполнения этих пунктов M3_embeddings закрыт; следующий шаг — open questions из `embeddings.md` по приоритету.
