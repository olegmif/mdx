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
