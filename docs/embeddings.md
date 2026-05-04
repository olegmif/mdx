# mdx — стратегия embeddings и dense-поиска

## Что и зачем

Расширение `mdx` подсистемой semantic-поиска: для каждой проиндексированной заметки внешней моделью вычисляется embedding-вектор; векторы хранятся в Qdrant с метаданными из БД mdx как payload. Поиск по запросу — векторизация запроса той же моделью и k-NN-поиск в Qdrant с возвратом списка путей к заметкам.

Подсистема надстраивается над архитектурой `strategy.md`. Источник истины по содержимому заметок остаётся прежним (файловая система → SQLite-индекс mdx); Qdrant — отдельный кэш векторов, восстановимый из ФС полным пересчётом. Гибридный поиск (dense + sparse, BM25/SPLADE) — за пределами текущего скоупа; архитектура (named vectors в одном collection с общим payload) совместима с его последующим добавлением.

## Ключевые решения и почему так

### Хранилище — Qdrant, одна коллекция с именованными векторами 

Одна коллекция (`mdx`) с несколькими именованными векторами — по одному на embedding-модель. Это даёт:

- общий payload и единый id точки на разные модели — нет дублирования метаданных и кросс-модельных рассинхронов;
- лёгкое подключение новой модели — добавление именованного вектора в существующую коллекцию через `update_collection`;
- готовность к гибридному поиску — sparse-вектор добавляется тем же механизмом именованных векторов.

Альтернатива «отдельная коллекция на модель» отвергнута: дублирование payload, отдельная процедура поиска по нескольким моделям, проблемы синхронизации удалений.

### Метрика сходства — Cosine

Cosine для всех именованных векторов. Причины:

- документация Qwen3-Embedding специфицирует cosine как штатный способ скоринга;
- Cosine инвариантен к норме вектора — совместим с моделями, не нормализующими выход;
- если вектор уже нормализован, Qdrant вычисляет cosine как скалярное произведение, разница в производительности с `Dot` пренебрежима.

`Dot` потребовал бы гарантировать нормализацию на стороне клиента и сорвался бы при подключении модели, не нормализующей выход. `Euclid` для embedding-моделей retrieval-назначения нерелевантен.

### Identifier точки — UUID v5 от пути

Qdrant принимает в качестве id `uint64` либо UUID. Используем UUID v5 с фиксированным namespace и абсолютным путём заметки — детерминированный, идемпотентный id: повторный embed одной и той же заметки делает upsert той же точки, без дубликатов и без необходимости поддерживать отдельную таблицу соответствий.

### Whole-note embedding, без чанкирования

Одна заметка → одна точка с одним вектором (на модель). Чанкирование (разбиение длинных заметок на абзацы или окна с overlap) — вне скоупа M0–M2. Заметки длиннее контекста модели обрезаются с записью WARN. Возврат к чанкированию — отдельная задача после оценки реальных лимитов на корпусе пользователя.

### Инвалидация по `content_hash`

`content_hash` из таблицы `notes` — единственный сигнал «контент изменился, вектор устарел». При запуске `mdx embed`:

- если для пары (path, model) запись в локальной таблице embedding-метаданных есть и `content_hash` совпадает с текущим в `notes` — пропуск;
- иначе — пересчёт и upsert в Qdrant, обновление локальной записи.

Изменения только во frontmatter без правки тела всё равно меняют `content_hash` (хеш считается от полного содержимого) — это приемлемо: payload в Qdrant также обновляется, расхождения не возникает.

### Локальное состояние embeddings — метаданные, не векторы

Поле `vector BLOB`, упомянутое в `strategy.md` как заглушка для будущей таблицы, не используется: векторы живут в Qdrant. Локальная таблица `embeddings` отражает только статус — какие модели уже посчитаны и для какого `content_hash`. Это позволяет команде embed работать инкрементально без обращения к Qdrant за `retrieve` на каждую заметку.

### Инструкции для query/document — параметры модели, не зашитая логика

Asymmetric-retrieval-модели (включая Qwen3-Embedding) требуют разных prefix для query и для document. Это часть конфигурации модели (`query_prefix`, `document_prefix`), а не логика, зашитая в `mdx`. Документы embed-ятся без префикса (или с указанным в конфиге), запросы — с префиксом по шаблону модели. Смена prefix считается изменением модели и требует пересчёта корпуса (см. open questions).

### Конфигурация моделей — отдельный YAML-файл

`~/.config/mdx/embedding.yaml` (резолв по тем же правилам, что у `ignore`: флаг → переменная → `$XDG_CONFIG_HOME/mdx/embedding.yaml`). Hot-reload вне скоупа. YAML выбран по консистентности с парсингом frontmatter, который уже использует `gopkg.in/yaml.v3`.

### Payload в Qdrant — подмножество строки `notes`

В payload Qdrant попадают `path`, `title`, `mtime`, `content_hash`, `frontmatter`. Поля `links` и `tags` не дублируются: они доступны в SQLite mdx по `path` и их синхронизация через Qdrant удвоила бы сложность инвалидации. Поле `path` индексируется в Qdrant для быстрых фильтров (например, при удалении точек по списку путей в `mdx gc`).

## Архитектура

### Внешние зависимости

| Сервис | Эндпойнт | Роль |
|---|---|---|
| Embedding model API | конфигурируется (на старте `http://127.0.0.1:8888`) | Векторизация заметок и запросов |
| Qdrant | конфигурируется (на старте `http://127.0.0.1:6333`) | Хранение векторов и k-NN-поиск |

Связь — HTTP/REST. gRPC к Qdrant — open question по производительности.

### Конфигурация — `~/.config/mdx/embedding.yaml`

```yaml
qdrant_url: http://127.0.0.1:6333
collection: mdx

models:
  - name: qwen3-embedding-4b
    endpoint: http://127.0.0.1:8888/v1/embeddings
    endpoint_kind: openai            # openai | llama-cpp | tei
    api_model_name: Qwen/Qwen3-Embedding-4B
    dim: 2560
    distance: cosine
    query_prefix: "Instruct: Given a web search query, retrieve relevant passages that answer the query\nQuery: "
    document_prefix: ""
    batch_size: 16
    default_for_search: true
```

Несколько элементов `models` — несколько named vectors. Поле `default_for_search` определяет модель по умолчанию для `mdx search` и для neovim-команды; при единственной модели может опускаться, при нескольких без явного default — ошибка конфигурации.

### Схема Qdrant collection

```
collection: mdx
  vectors:
    qwen3-embedding-4b: { size: 2560, distance: Cosine }
    <future-model>:     { size: ...,  distance: Cosine }
  payload schema:
    path           string  (indexed)
    title          string
    mtime          integer
    content_hash   string
    frontmatter    json
```

Индекс на `path` — единственный обязательный; остальные поля payload индексируются по мере появления реальных filter-сценариев.

### Изменения в схеме SQLite mdx

Таблица `embeddings` пересматривается относительно `strategy.md`:

```
embeddings(
  path          TEXT NOT NULL,
  model         TEXT NOT NULL,
  content_hash  TEXT NOT NULL,
  embedded_at   INTEGER NOT NULL,
  PRIMARY KEY (path, model),
  FOREIGN KEY (path) REFERENCES notes(path) ON DELETE CASCADE
)
```

`vector BLOB` не хранится — векторы в Qdrant. `path` каскадируется при удалении заметки из `notes`; чистка соответствующих точек в Qdrant — обязанность `mdx gc` (расширение, см. open questions).

### Подкоманды

| Подкоманда | Назначение |
|---|---|
| `mdx embed` | Для каждой модели из конфига и каждой заметки из `notes`, для которой `embeddings` либо не содержит записи, либо `content_hash` устарел — векторизовать через эндпойнт модели и сделать upsert в Qdrant. Флаги: `--model <name>` (ограничить одной моделью), `--all` (пересчитать всё, игнорируя `embeddings`). |
| `mdx search <query>` | Векторизовать запрос моделью (`--model` или default), выполнить k-NN-поиск в Qdrant, вывести результат. Флаги: `--limit N` (default 20), `--format text\|json` (default `text` — путь на строке; `json` — массив `{path, score, title}`). |

### Связь LSP с подсистемой embeddings

На M0–M2 LSP не участвует в embed/search: embed запускается командой пользователя, search вызывается отдельной CLI- или neovim-командой. Авто-embed по `didSave` и LSP-метод `mdx/search` — open questions.

## План реализации

### M0_embeddings — генерация векторов

- Конфиг `~/.config/mdx/embedding.yaml`: резолв пути, парсер, валидация (хотя бы одна модель, уникальность `name`, единственность `default_for_search`).
- HTTP-клиент к Qdrant: создание collection при отсутствии, добавление named vector через `update_collection` при появлении новой модели в конфиге, batch upsert точек.
- HTTP-клиент к embedding API: варианты payload запроса по `endpoint_kind` (`openai`/`llama-cpp`/`tei`), batch.
- Подкоманда `mdx embed`: проход по `notes`, для каждой пары (path, model) сравнение `content_hash` с записью в `embeddings`; пропуск или пересчёт.
- Запись/обновление строки в `embeddings` после успешного upsert в Qdrant. Транзакционность best-effort: при сбое после upsert и до записи в SQLite повторный запуск пересчитает (избыточная, но безопасная работа).
- Логирование: итоговая сводка `embedded N, skipped M, failed K`; ошибки одной заметки не прерывают цикл.

**Результат:** `mdx embed` за один прогон приводит Qdrant в соответствие с текущим состоянием `notes` для всех моделей из конфига. Повторные запуски идемпотентны.

### M1_embeddings — поиск

- Подкоманда `mdx search <query>`: выбор модели (`--model` или default), применение `query_prefix`, отправка в embedding API, получение вектора.
- Запрос к Qdrant: `points/search` по соответствующему named vector, `limit = N`, без фильтров.
- Вывод: `text` — путь на строке, отсортировано по убыванию score; `json` — массив `{path, score, title}`.

**Результат:** dense-поиск по корпусу из CLI.

### M2_embeddings — neovim-команда поиска

- Lua-команда `:MdxSearch`; keymap `<leader>ms`.
- Промпт через `vim.ui.input`. Источник дефолтного значения промпта (пусто, выделение visual-mode, текущий параграф) — open question; на старте — пустой default.
- Вызов `mdx search --format json` через `vim.system` (синхронно с индикацией ожидания) либо через LSP-метод `mdx/search` — выбор делается в M2 по итогу прототипа.
- Результат — Telescope-picker: каждая строка `title (display_path)` со score; preview — содержимое заметки. Enter — `vim.cmd.edit(path)`.
- Пустой результат — сообщение, picker не открывается.

**Результат:** dense-поиск по корпусу из neovim.

## Открытые вопросы

- **Гибридный поиск (dense + sparse).** BM25/SPLADE как отдельный named (sparse) vector в том же collection. Пересчёт sparse — отдельная подкоманда либо совмещённая с `mdx embed`. Приоритизация результатов — RRF или взвешенная комбинация. Возврат после оценки качества чистого dense-поиска.
- **Чанкирование длинных заметок.** Триггер — оценка доли заметок, превосходящих контекст модели. Реализация — окно с overlap, расширение payload полем `chunk_index`, id точки = `uuid5(namespace, path + "#" + chunk_index)`.
- **`mdx gc` и Qdrant.** Расширение `gc`: после удаления строк из `notes`/`embeddings` — удаление соответствующих точек в Qdrant по фильтру `path`. Сейчас векторы удалённых заметок остаются в коллекции до полной пересборки.
- **Авто-embed через LSP.** Triggered embed на `didSave` снимает потребность вручную запускать `mdx embed`. Требует решения: блокировать save до завершения embed (плохо) или фоновая очередь с уведомлением о завершении.
- **Reranking.** Cross-encoder поверх top-N от Qdrant. Отдельная модель в конфиге, опциональный второй проход в `mdx search`.
- **Filter-параметры в search.** Прокидывание Qdrant-payload-фильтра (`frontmatter.type=task` и т.п.) флагами `mdx search`.
- **gRPC к Qdrant.** REST достаточен на текущем объёме; gRPC — при росте корпуса.
- **Смена `query_prefix`/`document_prefix`/`api_model_name` без изменения `name` модели.** Сейчас расценивается как «та же модель, актуальный вектор» — фактически вектор устарел. Возможные решения: включить prefix-ы в локальный fingerprint модели и инвалидировать при изменении; либо договориться, что любое изменение модели сопровождается сменой `name`.
- **Источник запроса в `:MdxSearch`.** Промпт vs визуальное выделение vs параграф под курсором. Решается в M2 по итогу обкатки.

## Технические настройки

- **HTTP-клиент:** `net/http` стандартной библиотеки.
- **Конфигурация:** YAML, парсер — `gopkg.in/yaml.v3` (уже подключён).
- **UUID:** `github.com/google/uuid` (уже подключён транзитивно), `uuid.NewSHA1(namespace, []byte(path))` для UUID v5; namespace — фиксированная константа в `embed`-пакете.
- **Размещение кода:**
  - `lsp/internal/embed/config.go` — загрузка и валидация `embedding.yaml`.
  - `lsp/internal/embed/qdrant.go` — клиент Qdrant.
  - `lsp/internal/embed/model.go` — клиент embedding API (варианты по `endpoint_kind`).
  - `lsp/internal/embed/embed.go` — оркестрация `mdx embed`.
  - `lsp/internal/embed/search.go` — оркестрация `mdx search`.
  - `lsp/internal/cli/embed.go`, `lsp/internal/cli/search.go` — CLI-обёртки.
  - `lsp/internal/db/embeddings.go` — операции над таблицей `embeddings`.
- **Тесты:**
  - табличные на конфиг (`config_test.go`) и UUID-генерацию;
  - юнит-тесты клиентов с моком HTTP через `net/http/httptest`;
  - интеграционный тест `mdx embed` end-to-end на временной SQLite + httptest-эндпойнтах под Qdrant и embedding API.
