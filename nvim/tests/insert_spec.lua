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
