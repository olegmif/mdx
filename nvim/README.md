# mdx — neovim-плагин

Навигация по markdown-ссылкам и conceal путей. Часть проекта `mdx`.

## Подключение через lazy.nvim

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
