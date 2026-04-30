-- Гарантируем, что плагин на runtimepath даже если lazy.nvim его не активировал.
vim.opt.rtp:prepend(vim.fn.expand("~/projects/mdx/nvim"))

local link = require("mdx.link")

local function setup_buffer(lines, cursor)
	local bufnr = vim.api.nvim_create_buf(false, true)
	vim.api.nvim_set_current_buf(bufnr)
	vim.api.nvim_buf_set_lines(bufnr, 0, -1, false, lines)
	vim.bo[bufnr].filetype = "markdown"
	-- parse(true) — с инъекциями: вытянет markdown_inline-дерево
	vim.treesitter.get_parser(bufnr, "markdown"):parse(true)
	vim.api.nvim_win_set_cursor(0, cursor) -- 1-based row, 0-based col
end

describe("mdx.link.under_cursor", function()
	it("находит ссылку, когда курсор на тексте", function()
		setup_buffer({ "[hello](./foo.md)" }, { 1, 2 })
		local l = link.under_cursor()
		assert.is_not_nil(l)
		assert.equals("hello", l.text)
		assert.equals("./foo.md", l.target)
		assert.equals(0, l.start)
		assert.equals(17, l.finish)
	end)

	it("находит ссылку, когда курсор на target", function()
		setup_buffer({ "[hello](./foo.md)" }, { 1, 12 })
		local l = link.under_cursor()
		assert.is_not_nil(l)
		assert.equals("./foo.md", l.target)
	end)

	it("возвращает nil, когда курсор не на ссылке", function()
		setup_buffer({ "just plain text here" }, { 1, 5 })
		assert.is_nil(link.under_cursor())
	end)

	it("выбирает вторую ссылку, когда курсор внутри неё", function()
		setup_buffer({ "[a](./a.md) and [b](./b.md)" }, { 1, 18 })
		local l = link.under_cursor()
		assert.is_not_nil(l)
		assert.equals("b", l.text)
	end)

	it("работает со ссылкой с title", function()
		setup_buffer({ '[hello](./foo.md "title")' }, { 1, 2 })
		local l = link.under_cursor()
		assert.is_not_nil(l)
		assert.equals("hello", l.text)
		assert.equals("./foo.md", l.target)
	end)

	it("возвращает nil для ссылки внутри fenced code block", function()
		setup_buffer({
			"```",
			"[fake](./fake.md)",
			"```",
		}, { 2, 2 })
		assert.is_nil(link.under_cursor())
	end)
end)
