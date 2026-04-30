vim.opt.rtp:prepend(vim.fn.expand("~/projects/mdx/nvim"))

local insert = require("mdx.insert")

describe("mdx.insert.format_link", function()
	it("тот же каталог", function()
		assert.equals("[Foo](./foo.md)", insert.format_link("/p/notes/foo.md", "/p/notes", "Foo"))
	end)

	it("вложенный каталог", function()
		assert.equals("[Foo](./sub/foo.md)", insert.format_link("/p/notes/sub/foo.md", "/p/notes", "Foo"))
	end)

	it("родительский каталог", function()
		assert.equals("[Foo](../foo.md)", insert.format_link("/p/foo.md", "/p/notes", "Foo"))
	end)

	it("соседний каталог через родителя", function()
		assert.equals("[Foo](../other/foo.md)", insert.format_link("/p/other/foo.md", "/p/notes", "Foo"))
	end)

	it("заголовок с markdown-символами оставляется как есть", function()
		assert.equals("[a [b] c](./foo.md)", insert.format_link("/p/notes/foo.md", "/p/notes", "a [b] c"))
	end)
end)

describe("mdx.insert.at_cursor", function()
	local bufnr

	before_each(function()
		bufnr = vim.api.nvim_create_buf(false, true)
		vim.api.nvim_set_current_buf(bufnr)
	end)

	after_each(function()
		vim.api.nvim_buf_delete(bufnr, { force = true })
	end)

	it("вставляет строку в пустой буфер", function()
		vim.api.nvim_buf_set_lines(bufnr, 0, -1, false, { "" })
		vim.api.nvim_win_set_cursor(0, { 1, 0 })
		require("mdx.insert").at_cursor("[Foo](./foo.md)")
		assert.equals("[Foo](./foo.md)", vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)[1])
	end)

	it("вставляет после курсора в непустой строке", function()
		vim.api.nvim_buf_set_lines(bufnr, 0, -1, false, { "ab" })
		vim.api.nvim_win_set_cursor(0, { 1, 0 }) -- курсор на 'a'
		require("mdx.insert").at_cursor("X")
		assert.equals("aXb", vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)[1])
	end)
end)

describe("mdx.insert_link integration", function()
	it("полная цепочка через подмену picker.open", function()
		local bufnr = vim.api.nvim_create_buf(false, true)
		vim.api.nvim_set_current_buf(bufnr)
		vim.api.nvim_buf_set_name(bufnr, "/p/notes/current.md")
		vim.api.nvim_buf_set_lines(bufnr, 0, -1, false, { "" })
		vim.api.nvim_win_set_cursor(0, { 1, 0 })

		local picker = require("mdx.picker")
		local original = picker.open
		picker.open = function(_, on_select)
			on_select({ path = "/p/notes/sub/foo.md", title = "Foo" })
		end

		local ok, err = pcall(require("mdx").insert_link)
		picker.open = original
		assert(ok, err)

		assert.equals("[Foo](./sub/foo.md)", vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)[1])
	end)
end)
