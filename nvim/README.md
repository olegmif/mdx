# mdx — neovim-плагин

Навигация по markdown-ссылкам и conceal путей. Часть проекта [`mdx`](https://github.com/olegmif/mdx).

При курсоре на `[text](path)` нажатие настроенного keymap'а открывает целевой файл — в текущем окне или в вертикальном сплите. Path-часть ссылки скрывается через conceal: видна только подчёркнутая `text`, как гиперссылка. При входе курсора на строку со ссылкой она раскрывается полностью.

## Требования

- Neovim ≥ 0.11
- `nvim-treesitter` с парсерами `markdown` и `markdown_inline`

## Установка

Через `lazy.nvim`:

```lua
{
    dir = vim.fn.expand("~/projects/mdx/nvim"),
    name = "mdx",
    ft = "markdown",
    dependencies = {
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

## Настройки и дефолты

```lua
require("mdx").setup({
    keymaps = {
        follow = "<leader>mf",       -- открыть ссылку в текущем окне
        follow_split = "<leader>ms", -- открыть в вертикальном сплите
    },
    conceal = true, -- скрывать path-часть ссылки
})
```

Любой keymap отключается передачей `false`:

```lua
require("mdx").setup({
    keymaps = { follow_split = false },
    conceal = false,
})
```

## Команды

- `:MdxFollow` — открыть ссылку под курсором в текущем окне.
- `:MdxFollowSplit` — открыть в вертикальном сплите.

Поведение при отсутствии ссылки или внешнем URL:
- если под курсором нет ссылки — уведомление `mdx: no link under cursor`,
- если ссылка указывает на URL (`http://`, `https://`, `mailto:`, `ftp://`, `tel:`, `#anchor`) — уведомление `mdx: external URL, ignored`, файл не открывается.

## API

- `require("mdx").follow()` — то же, что `:MdxFollow`. Возвращает `true`, если сценарий обработан (открытие или уведомление о внешнем URL), `false`, если под курсором нет ссылки.
- `require("mdx").follow_split()` — аналог `:MdxFollowSplit`.
- `require("mdx").setup(opts)` — конфигурация плагина.

## Скоуп

Плагин активен только в буферах с `filetype=markdown` (через `ft = "markdown"` в lazy-spec). Команды и keymap'ы вне markdown-буферов недоступны.

Внутри fenced code blocks конструкции вида `[text](path)` корректно игнорируются: парсер `markdown_inline` не инъецируется в код-блоки, поэтому conceal к ним не применяется и `:MdxFollow` на них ничего не делает.
