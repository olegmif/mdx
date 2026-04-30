vim.opt.rtp:prepend(vim.fn.expand("~/projects/mdx/nvim"))

local insert = require("mdx.insert")

describe("mdx.insert.format_link", function()
	it("файл под $HOME — путь начинается с ~/", function()
		local home = vim.fn.expand("~")
		assert.equals(
			"[Foo](~/notes/foo.md)",
			insert.format_link(home .. "/notes/foo.md", "Foo")
		)
	end)

	it("корневой $HOME → одиночный ~", function()
		local home = vim.fn.expand("~")
		assert.equals("[Home](~)", insert.format_link(home, "Home"))
	end)

	it("файл вне $HOME — абсолютный путь без изменений", function()
		assert.equals("[Hosts](/etc/hosts)", insert.format_link("/etc/hosts", "Hosts"))
	end)

	it("путь с .. нормализуется", function()
		local home = vim.fn.expand("~")
		assert.equals(
			"[Foo](~/notes/foo.md)",
			insert.format_link(home .. "/notes/sub/../foo.md", "Foo")
		)
	end)

	it("заголовок с markdown-символами оставляется как есть", function()
		local home = vim.fn.expand("~")
		assert.equals(
			"[a [b] c](~/notes/foo.md)",
			insert.format_link(home .. "/notes/foo.md", "a [b] c")
		)
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
		require("mdx.insert").at_cursor("[Foo](~/foo.md)")
		assert.equals("[Foo](~/foo.md)", vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)[1])
	end)

	it("вставляет после курсора в непустой строке", function()
		vim.api.nvim_buf_set_lines(bufnr, 0, -1, false, { "ab" })
		vim.api.nvim_win_set_cursor(0, { 1, 0 })
		require("mdx.insert").at_cursor("X")
		assert.equals("aXb", vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)[1])
	end)
end)

describe("mdx.insert_link integration", function()
	it("полная цепочка через подмену picker.open", function()
		local home = vim.fn.expand("~")
		local bufnr = vim.api.nvim_create_buf(false, true)
		vim.api.nvim_set_current_buf(bufnr)
		vim.api.nvim_buf_set_lines(bufnr, 0, -1, false, { "" })
		vim.api.nvim_win_set_cursor(0, { 1, 0 })

		local picker = require("mdx.picker")
		local original = picker.open
		picker.open = function(_, on_select)
			on_select({ path = home .. "/notes/foo.md", title = "Foo" })
		end

		local ok, err = pcall(require("mdx").insert_link)
		picker.open = original
		assert(ok, err)

		assert.equals(
			"[Foo](~/notes/foo.md)",
			vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)[1]
		)
	end)
end)
