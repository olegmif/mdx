# mdx — neovim-плагин

Навигация по markdown-ссылкам, conceal путей и picker по заметкам. Часть проекта [`mdx`](https://github.com/olegmif/mdx).

При курсоре на `[text](path)` нажатие настроенного keymap'а открывает целевой файл — в текущем окне или в вертикальном сплите. Path-часть ссылки скрывается через conceal: видна только подчёркнутая `text`, как гиперссылка; при входе курсора на строку она раскрывается полностью.

`<leader>mi` открывает Telescope-picker со списком заметок, известных индексу `mdx`-LSP. Выбор вставляет в позицию курсора готовую markdown-ссылку.

## Требования

- Neovim ≥ 0.11
- `nvim-treesitter` с парсерами `markdown` и `markdown_inline`
- `telescope.nvim` (для picker'а)
- Запущенный `mdx`-LSP-сервер с заполненной БД (см. [strategy.md](../docs/strategy.md))

## Установка

Через `lazy.nvim`:

```lua
{
    dir = vim.fn.expand("~/projects/mdx/nvim"),
    name = "mdx",
    ft = "markdown",
    dependencies = {
        "nvim-telescope/telescope.nvim",
        {
            "nvim-treesitter/nvim-treesitter",
            opts = {
                ensure_installed = { "markdown", "markdown_inline" },
            },
        },
    },
    config = function()
        require("mdx").setup({})
    end,
}
```

LSP-клиент `mdx` должен быть настроен и автоподключаться к markdown-буферам отдельно (через `vim.lsp.config` или `lspconfig`). Picker не работает без подключённого клиента.

## Настройки и дефолты

```lua
require("mdx").setup({
    keymaps = {
        follow = "<leader>mf",       -- открыть ссылку в текущем окне
        follow_split = "<leader>ms", -- открыть в вертикальном сплите
        insert_link = "<leader>mi",  -- picker по заметкам
    },
    conceal = true, -- скрывать path-часть ссылки
})
```

Любой keymap отключается передачей `false`:

```lua
require("mdx").setup({
    keymaps = { insert_link = false },
    conceal = false,
})
```

## Команды

- `:MdxFollow` — открыть ссылку под курсором в текущем окне.
- `:MdxFollowSplit` — открыть в вертикальном сплите.
- `:MdxInsertLink` — открыть picker по существующим заметкам, выбор вставляет ссылку.

Поведение `:MdxFollow` / `:MdxFollowSplit`:
- если под курсором нет ссылки — уведомление `mdx: no link under cursor`,
- если ссылка указывает на URL (`http://`, `https://`, `mailto:`, `ftp://`, `tel:`, `#anchor`) — уведомление `mdx: external URL, ignored`.

Поведение `:MdxInsertLink`:
- если LSP-клиент `mdx` не подключён — уведомление `mdx: LSP client not attached`, picker не открывается;
- если БД индекса пуста — уведомление `mdx: no notes in index`, picker не открывается;
- иначе открывается picker с заголовком `mdx: insert link` и боковым превью содержимого заметки. Выбор вставляет в позицию курсора строку `[Заголовок](путь)` и ставит курсор сразу после `)`.

## API

- `require("mdx").follow()` — то же, что `:MdxFollow`. Возвращает `true`, если сценарий обработан, `false`, если под курсором нет ссылки.
- `require("mdx").follow_split()` — аналог `:MdxFollowSplit`.
- `require("mdx").insert_link()` — то же, что `:MdxInsertLink`.
- `require("mdx").setup(opts)` — конфигурация плагина.

## Формат вставляемой ссылки

`:MdxInsertLink` формирует строку `[title](path)` по правилу:
- `path` начинается с `~/`, если файл лежит под `$HOME`;
- иначе — абсолютный путь;
- относительная арифметика (`../`, `./sub/`) не применяется. Ссылки остаются валидными независимо от того, куда переехал исходный файл, пока цель остаётся под `$HOME`.

`title` берётся из колонки `notes.title` в БД. Если в БД заголовок пуст (frontmatter без `title`), LSP подставляет имя файла без расширения `.md`.

## Источник списка заметок

Picker делает синхронный LSP-запрос `mdx/listNotes` к серверу. Сервер возвращает все известные ему заметки из БД. БД наполняется:
- запуском `mdx scan <path>` — рекурсивный скан и индексация;
- `mdx`-LSP'ом на `textDocument/didOpen` и `textDocument/didSave` — точечная индексация открытых/сохраняемых файлов.

Если в picker'е не видно нужной заметки — обычно достаточно `mdx scan` от соответствующего корня.

## Диагностика

LSP отправляет warning-диагностики на:
- битые ссылки (`broken link: <target>`) — целевой файл не существует;
- ссылки с пустым title (`empty link title: [](<target>)`) — conceal делает такие ссылки **визуально невидимыми**, поэтому warning подсвечивает их в `signcolumn`.

## Замкнутый цикл

1. `<leader>mi` → выбрать заметку в picker'е → ссылка `[title](~/path.md)` вставилась в курсор.
2. На той же ссылке `<leader>mf` → файл `path.md` открывается в текущем окне.

## Скоуп

Плагин активен только в буферах с `filetype=markdown` (через `ft = "markdown"` в lazy-spec). Команды и keymap'ы вне markdown-буферов недоступны.

Внутри fenced code blocks конструкции вида `[text](path)` корректно игнорируются: парсер `markdown_inline` не инъецируется в код-блоки, поэтому conceal к ним не применяется и `:MdxFollow` на них ничего не делает.
