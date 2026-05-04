# M2_embeddings — neovim-поиск

## Цель

Появляется команда `:MdxSearch [query]` и одноимённая функция в Lua-плагине `nvim/`. При вызове без аргумента команда запрашивает строку через `vim.ui.input`; при вызове с аргументом — использует его напрямую. Введённый запрос отправляется в `mdx search --format json --limit N <query>` через `vim.system`; полученный массив результатов открывается в Telescope-picker'е, где каждая строка — заметка из выдачи Qdrant. Enter открывает заметку в текущем окне; стандартные Telescope-мэппинги `<C-v>`/`<C-x>`/`<C-t>` — в vsplit/split/tab.

После M2_embeddings dense-поиск по корпусу доступен из neovim без переключения в терминал. Других подсистем M2 не трогает: индексирование, embed и LSP остаются прежними, новых LSP-методов не вводится.

## Что входит и что не входит

### Входит

- Новый Lua-модуль `nvim/lua/mdx/search.lua` с функциями `M.run`, `M.invoke`, `M.parse`.
- Расширение `nvim/lua/mdx/picker.lua` функцией `M.search_results(hits, on_select)`.
- Регистрация пользовательской команды `:MdxSearch [query]` в `nvim/lua/mdx/init.lua` и буфер-локального keymap в `nvim/ftplugin/markdown.lua`.
- Раздел `search` в дефолтной конфигурации плагина (`mdx_bin`, `limit`, `timeout_ms`) и поле `keymaps.search`.
- Plenary-тесты `nvim/tests/search_spec.lua`: табличный тест на `M.parse` и integration-тест на `M.run` через подмену `picker.search_results`.
- Раздел про `:MdxSearch` в `nvim/README.md`.

### Не входит (отложено)

- LSP-метод `mdx/search` — open question в `embeddings.md`, вводится при появлении кейса частых поисков подряд.
- Источники запроса, отличные от `vim.ui.input` и аргумента команды (visual-selection, параграф под курсором) — open question в `embeddings.md`.
- Reranking, фильтры по payload, гибридный (dense + sparse) поиск — open questions, на стороне CLI ещё не реализованы (`M1_embeddings.md`).
- Отображение score в строках picker'а и пользовательский score-threshold — добавляются по конкретному запросу.
- Кэш результатов в плагине: каждый вызов `:MdxSearch` идёт в живой `mdx`.
- Авто-embed по `didSave` через LSP — отдельная open question в `embeddings.md`.

## Технические решения

| Вопрос | Решение |
|---|---|
| Транспорт CLI vs LSP | Используется CLI: `mdx search --format json --limit N <query>` через `vim.system`. Узкое место поиска — embedding API (сотни мс) и k-NN в Qdrant; оверхед запуска процесса (~10–50 мс) на этом фоне незначим. LSP-метод `mdx/search` остаётся open question и может быть введён позже без слома пользовательского API команды. |
| Синхронность | `vim.system(cmd, opts, on_exit)` с асинхронным callback'ом, обёрнутым в `vim.schedule_wrap` для безопасного обращения к API neovim. UI не блокируется на время запроса. |
| Источник запроса | Если позиционный аргумент `:MdxSearch` задан — он используется как есть. Иначе — `vim.ui.input{prompt = "MdxSearch: "}` с пустым default. Возврат `nil` или пустой строки — отмена без notify. |
| Путь к бинарю | Поле `search.mdx_bin` в конфиге плагина, default `"mdx"`. Резолв через `$PATH`; других стратегий поиска бинаря не предусматривается. |
| Limit и timeout | Поля `search.limit` (default `30`) и `search.timeout_ms` (default `30000`) в конфиге. `limit` передаётся в CLI флагом `--limit`; `timeout_ms` — опцией `timeout` для `vim.system`, по истечении процесс убивается, callback получает ненулевой код. |
| Парсинг вывода | `vim.json.decode(obj.stdout)` оборачивается в `pcall`. На вход допустимы пустой массив `[]` и массив объектов `{path, score, title?}`. Поле `title` опционально (omitempty в CLI для заметок без `title:` в frontmatter). |
| Picker | Новая функция `picker.search_results(hits, on_select)`. `display` — `<title> (<display_path>)`, где `display_path` строится через `mdx.insert.to_display_path` (как в `picker.sql_results`); при пустом `title` подставляется `vim.fn.fnamemodify(path, ":t:r")`. `ordinal` — `<title> <path>`. `previewer` — `conf.file_previewer`. `sorter` — `sorters.empty()`, чтобы Telescope не перебивал порядок Qdrant fuzzy-матчингом против `ordinal`. |
| Действие при выборе | `vim.cmd.edit(vim.fn.fnameescape(hit.path))` в callback `on_select`. Открытие в split/vsplit/tab обеспечивается стандартными мэппингами Telescope; собственные мэппинги в `attach_mappings` не добавляются. |
| Пустой результат | `vim.notify("mdx: no results", vim.log.levels.INFO)`; picker не открывается. Это согласовано с поведением CLI, где пустой результат — не ошибка. |
| Ошибка CLI | Если `obj.code ~= 0`, выводится `vim.notify("mdx: search failed: " .. obj.stderr, vim.log.levels.ERROR)`. Невалидный JSON в stdout — отдельный notify уровня ERROR с текстом ошибки от `pcall`. |
| Timeout | После истечения `search.timeout_ms` `vim.system` шлёт SIGKILL и завершает callback с ненулевым кодом. В сообщении notify'а `obj.stderr` обычно пуст; распознавание именно timeout-кейса по сравнению `obj.code` с константой не делается — пользователю достаточно сообщения «search failed». |
| Keymap | Поле `keymaps.search`, default `<leader>m/` (vim-идиоматичное «слэш — поиск»). Регистрируется буфер-локально в `nvim/ftplugin/markdown.lua` по образцу остальных пунктов `keymaps`. |
| Конфигурируемость через `setup` | Дефолтный раздел `search = {...}` объединяется с пользовательскими опциями через `vim.tbl_deep_extend("force", ...)` в существующей `M.setup`. Дополнительной валидации полей не вводится: некорректные значения проявляются на первом вызове. |
| Тестируемость | Pure-функция `M.parse` тестируется табличным `describe/it`. Сборка end-to-end (`M.run`) тестируется через подмену `picker.search_results` и `vim.ui.input` — по образцу `tests/tag_search_spec.lua`. Сам `vim.system` не мокируется; ветка «реальный CLI отвечает» покрывается ручной проверкой из README. |

## Структура каталогов проекта (что добавляется к M1_embeddings)

```
/home/oleg/projects/mdx/nvim/
├── lua/mdx/
│   ├── init.lua            ← + раздел defaults.search, defaults.keymaps.search, M.search, регистрация :MdxSearch
│   ├── search.lua          ← новый: M.run(query?), M.invoke(query, on_done), M.parse(stdout)
│   └── picker.lua          ← + функция M.search_results(hits, on_select)
├── ftplugin/
│   └── markdown.lua        ← + блок биндинга keymaps.search
└── tests/
    └── search_spec.lua     ← новый: табличный M.parse + integration через подмену picker
```

Файлы Go-стороны не правятся: M2 целиком живёт в Lua-плагине и опирается на `mdx search`, реализованный в M1.

## Шаги выполнения

Шаги упорядочены так, чтобы после каждого можно было запустить осмысленную проверку — либо `NVIM_APPNAME=nvim-dev nvim --headless -c PlenaryBustedDirectory ...`, либо ручной вызов `:MdxSearch` в живом neovim.

### Шаг 1. Скаффолдинг модуля, команды и keymap

Создать `nvim/lua/mdx/search.lua` со скелетом модуля, добавить раздел `search` в дефолтную конфигурацию плагина и зарегистрировать команду `:MdxSearch`.

#### Состав `nvim/lua/mdx/search.lua`

```lua
local M = {}

function M.run(query)
    vim.notify("mdx: search not implemented", vim.log.levels.INFO)
end

return M
```

#### Состав `nvim/lua/mdx/init.lua`

Добавить в `defaults`:

```lua
keymaps = {
    -- ...существующие пункты...
    search = "<leader>m/",
},
search = {
    mdx_bin    = "mdx",
    limit      = 30,
    timeout_ms = 30000,
},
```

Добавить функцию модуля и регистрацию команды:

```lua
function M.search(query)
    require("mdx.search").run(query)
end

-- внутри M.setup:
vim.api.nvim_create_user_command("MdxSearch", function(opts)
    local q = opts.args ~= "" and opts.args or nil
    require("mdx").search(q)
end, { nargs = "?" })
```

#### Состав `nvim/ftplugin/markdown.lua`

Добавить блок биндинга по образцу остальных:

```lua
if config.keymaps.search then
    vim.keymap.set("n", config.keymaps.search, function()
        mdx.search()
    end, { buffer = true, desc = "mdx: dense search across notes" })
end
```

**Готовность:** `:MdxSearch` присутствует в `:command`-листинге, `:MdxSearch hello` и `<leader>m/` в markdown-буфере приводят к notify «search not implemented». Конфиг расширяется без побочных эффектов на остальные команды.

### Шаг 2. Парсер вывода CLI

Реализовать pure-функцию `M.parse(stdout)`, которая декодирует JSON и возвращает массив hits с предсказуемой структурой.

#### Состав `nvim/lua/mdx/search.lua`

```lua
-- M.parse декодирует stdout из `mdx search --format json` в массив
-- hit-объектов вида { path = string, score = number, title = string? }.
-- При невалидном JSON возвращает nil и текст ошибки от vim.json.decode.
function M.parse(stdout)
    if stdout == nil or stdout == "" then
        return {}, nil
    end
    local ok, decoded = pcall(vim.json.decode, stdout)
    if not ok then
        return nil, tostring(decoded)
    end
    if type(decoded) ~= "table" then
        return nil, "expected JSON array, got " .. type(decoded)
    end
    return decoded, nil
end
```

#### Состав `nvim/tests/search_spec.lua`

Табличный тест на `M.parse`:

- пустая строка и `nil` → пустой массив, ошибки нет;
- валидный массив из трёх объектов (с title и без) → массив сохраняется как есть;
- невалидный JSON → возвращает `nil` и непустую ошибку;
- JSON-объект вместо массива → `nil` и сообщение об ожидаемом типе.

**Готовность:** `nvim/tests/search_spec.lua` в части `describe("mdx.search.parse")` зелёный.

### Шаг 3. Асинхронный вызов CLI

Реализовать `M.invoke(query, on_done)` через `vim.system`. Сборка команды и обработка кодов выхода — здесь; интерпретация результата и picker — в Шаге 5.

#### Состав `nvim/lua/mdx/search.lua`

```lua
-- M.invoke шлёт `mdx search --format json --limit N <query>` через
-- vim.system с timeout. Callback on_done(hits, err) вызывается из
-- vim.schedule. hits == nil сигнализирует об ошибке; err содержит
-- человекочитаемое сообщение, готовое для vim.notify.
function M.invoke(query, on_done)
    local cfg = require("mdx").config.search
    local cmd = {
        cfg.mdx_bin, "search",
        "--format", "json",
        "--limit", tostring(cfg.limit),
        query,
    }
    vim.system(cmd, { text = true, timeout = cfg.timeout_ms }, vim.schedule_wrap(function(obj)
        if obj.code ~= 0 then
            local stderr = (obj.stderr and obj.stderr ~= "") and obj.stderr or "exit code " .. tostring(obj.code)
            on_done(nil, "search failed: " .. stderr)
            return
        end
        local hits, err = M.parse(obj.stdout)
        if not hits then
            on_done(nil, "failed to parse search output: " .. err)
            return
        end
        on_done(hits, nil)
    end))
end
```

**Готовность:** ручной вызов `:lua require("mdx.search").invoke("qdrant", function(hits, err) print(vim.inspect(hits or err)) end)` на живом сетапе печатает массив hits либо строку ошибки. Тестов на этом шаге не добавляется: `vim.system` в plenary-тестах не мокируется.

### Шаг 4. Picker для результатов

Добавить в `nvim/lua/mdx/picker.lua` функцию `M.search_results(hits, on_select)`.

#### Состав `nvim/lua/mdx/picker.lua`

```lua
function M.search_results(hits, on_select)
    local ok, pickers = pcall(require, "telescope.pickers")
    if not ok then
        vim.notify("mdx: telescope.nvim is required", vim.log.levels.ERROR)
        return
    end
    local finders = require("telescope.finders")
    local sorters = require("telescope.sorters")
    local conf = require("telescope.config").values
    local actions = require("telescope.actions")
    local action_state = require("telescope.actions.state")
    local insert = require("mdx.insert")

    pickers
        .new({}, {
            prompt_title = string.format("mdx: search (%d)", #hits),
            finder = finders.new_table({
                results = hits,
                entry_maker = function(hit)
                    local title = hit.title and hit.title ~= "" and hit.title
                        or vim.fn.fnamemodify(hit.path, ":t:r")
                    local display_path = insert.to_display_path(hit.path)
                    return {
                        value = hit,
                        path = hit.path,
                        display = string.format("%s (%s)", title, display_path),
                        ordinal = title .. " " .. hit.path,
                    }
                end,
            }),
            sorter = sorters.empty(),
            previewer = conf.file_previewer({}),
            attach_mappings = function(prompt_bufnr, _)
                actions.select_default:replace(function()
                    local selection = action_state.get_selected_entry()
                    actions.close(prompt_bufnr)
                    if selection and on_select then
                        on_select(selection.value)
                    end
                end)
                return true
            end,
        })
        :find()
end
```

**Готовность:** функция вызывается без ошибок при ручном вводе массива из 2–3 синтетических hits в `:lua`; Enter передаёт hit в callback. Автоматических тестов на picker не пишется: Telescope открывает плавающее окно, плохо тестируемое в plenary headless.

### Шаг 5. Сборка `M.run` и обработка edge cases

Связать `vim.ui.input`, `M.invoke` и `picker.search_results` в `M.run`. Реализовать ветки пустого запроса, пустого результата и ошибки.

#### Состав `nvim/lua/mdx/search.lua`

```lua
local function show(query)
    M.invoke(query, function(hits, err)
        if err then
            vim.notify("mdx: " .. err, vim.log.levels.ERROR)
            return
        end
        if #hits == 0 then
            vim.notify("mdx: no results", vim.log.levels.INFO)
            return
        end
        require("mdx.picker").search_results(hits, function(hit)
            vim.cmd.edit(vim.fn.fnameescape(hit.path))
        end)
    end)
end

function M.run(query)
    if query and query ~= "" then
        show(query)
        return
    end
    vim.ui.input({ prompt = "MdxSearch: " }, function(input)
        if not input or input == "" then
            return
        end
        show(input)
    end)
end
```

**Готовность:** на живом сетапе `:MdxSearch qdrant configuration` открывает Telescope с осмысленными заметками; `:MdxSearch` без аргумента поднимает `vim.ui.input`; Esc на промпте — тихая отмена; запрос, который Qdrant вернул пустым, — INFO-notify «no results» без открытия picker'а.

### Шаг 6. Тесты

Добавить в `nvim/tests/search_spec.lua` integration-тест на `M.run` через подмену `picker.search_results` и `vim.ui.input`, по образцу `tests/tag_search_spec.lua`.

#### Состав `nvim/tests/search_spec.lua`

Раздел `describe("mdx.search.run integration")`:

- стаб `vim.ui.input` отвечает строкой запроса; стаб `picker.search_results` принимает `hits` и сразу вызывает `on_select` с первым элементом; стаб `M.invoke` подставляет фиксированный массив hits, минуя `vim.system` — `M.run` без аргумента приводит к открытию заданного файла (`vim.api.nvim_buf_get_name(0)` равен ожидаемому пути);
- та же подмена `M.invoke` с пустым массивом → вызов `M.run("anything")` приводит к `vim.notify` уровня INFO без обращения к picker;
- `M.invoke` возвращает ошибку → `M.run("anything")` приводит к `vim.notify` уровня ERROR.

Подмена `vim.notify` через установку временной функции и восстановление в конце теста.

**Готовность:** `NVIM_APPNAME=nvim-dev nvim --headless -c "PlenaryBustedDirectory tests/" -c "qa!"` зелёный, включая новые подтесты.

### Шаг 7. README и ручная проверка

Добавить в `nvim/README.md` раздел про `:MdxSearch`.

#### Что добавить

- Краткое описание команды: dense-поиск через `mdx search` с открытием результатов в Telescope-picker'е.
- Предусловия: установлен Go-бинарь `mdx` в `$PATH` (или путь задан через `search.mdx_bin`), `mdx embed` уже выполнен хотя бы для одной модели, поднят Qdrant и embedding-сервер.
- Перечень полей конфигурации `search` (`mdx_bin`, `limit`, `timeout_ms`) и keymap (`keymaps.search`, default `<leader>m/`).
- Примеры: `:MdxSearch`, `:MdxSearch qdrant configuration`, биндинг через keymap в markdown-буфере.

#### Ручная проверка

В живом neovim с настроенным плагином и поднятым Qdrant + embedding-сервером:

```
:MdxSearch qdrant configuration
:MdxSearch
<leader>m/
```

Каждый сценарий должен открыть Telescope с непустым списком; Enter открывает выбранную заметку в текущем окне, `<C-v>`/`<C-x>`/`<C-t>` — в vsplit/split/tab. `:MdxSearch zzzqqqxxx` (заведомо пустая выдача) должен напечатать INFO-сообщение без открытия picker'а.

**Готовность:** README содержит работающие примеры на одном экране; ручная проверка на живом корпусе показывает разумные результаты и предсказуемое поведение во всех ветках.

## Критерии готовности M2_embeddings

1. `NVIM_APPNAME=nvim-dev nvim --headless -c "PlenaryBustedDirectory tests/" -c "qa!"` зелёный по всему `nvim/tests/`.
2. `:MdxSearch <query>` в живом neovim открывает Telescope-picker с заметками, отсортированными по убыванию score Qdrant.
3. `:MdxSearch` без аргумента поднимает `vim.ui.input`; Esc — тихая отмена.
4. Биндинг `<leader>m/` (или переопределённый через `keymaps.search`) в markdown-буфере вызывает то же поведение, что `:MdxSearch` без аргумента.
5. Запрос с пустой выдачей выдаёт INFO-notify «no results» без открытия picker'а.
6. Сбой CLI (отсутствует `mdx`, не поднят Qdrant, неверный конфиг) приводит к ERROR-notify с текстом из stderr; neovim не падает и не зависает.
7. Конфигурация плагина через `require("mdx").setup({ search = { limit = 50 } })` корректно перекрывает дефолты.
8. После выполнения этих пунктов M2_embeddings закрыт; следующий шаг — открытые вопросы из `embeddings.md` по приоритету.
