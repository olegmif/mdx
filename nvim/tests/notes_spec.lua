vim.opt.rtp:prepend(vim.fn.expand("~/projects/mdx/nvim"))

local notes = require("mdx.notes")

local function write(path, content)
	vim.fn.mkdir(vim.fs.dirname(path), "p")
	local f = io.open(path, "w")
	f:write(content or "")
	f:close()
end

describe("mdx.notes.collect", function()
	local root

	before_each(function()
		root = vim.fn.tempname()
		vim.fn.mkdir(root, "p")
	end)

	after_each(function()
		vim.fn.delete(root, "rf")
	end)

	it("возвращает пустой список при nil root", function()
		assert.same({}, notes.collect(nil))
	end)

	it("возвращает пустой список при пустом root", function()
		assert.same({}, notes.collect(""))
	end)

	it("находит одиночный .md файл в корне", function()
		write(root .. "/foo.md")
		assert.same({ root .. "/foo.md" }, notes.collect(root))
	end)

	it("находит .md файлы рекурсивно и сортирует результат", function()
		write(root .. "/a.md")
		write(root .. "/sub/b.md")
		write(root .. "/sub/sub2/c.md")
		local result = notes.collect(root)
		assert.equals(3, #result)
		assert.equals(root .. "/a.md", result[1])
		assert.equals(root .. "/sub/b.md", result[2])
		assert.equals(root .. "/sub/sub2/c.md", result[3])
	end)

	it("игнорирует не-.md файлы", function()
		write(root .. "/a.md")
		write(root .. "/b.txt")
		write(root .. "/c.lua")
		assert.same({ root .. "/a.md" }, notes.collect(root))
	end)

	it("исключает .git/ по умолчанию", function()
		write(root .. "/a.md")
		write(root .. "/.git/b.md")
		assert.same({ root .. "/a.md" }, notes.collect(root))
	end)

	it("исключает node_modules/ по умолчанию", function()
		write(root .. "/a.md")
		write(root .. "/node_modules/b.md")
		assert.same({ root .. "/a.md" }, notes.collect(root))
	end)

	it("учитывает пользовательские исключения через opts.exclude", function()
		write(root .. "/a.md")
		write(root .. "/_drafts/b.md")
		local result = notes.collect(root, { exclude = { "/_drafts/" } })
		assert.same({ root .. "/a.md" }, result)
	end)
end)

describe("mdx.notes.title", function()
	local root

	before_each(function()
		root = vim.fn.tempname()
		vim.fn.mkdir(root, "p")
	end)

	after_each(function()
		vim.fn.delete(root, "rf")
	end)

	local function write(path, content)
		local f = io.open(path, "w")
		f:write(content)
		f:close()
	end

	it("возвращает первый H1 из файла", function()
		write(root .. "/foo.md", "# My Title\n\nSome text")
		assert.equals("My Title", notes.title(root .. "/foo.md"))
	end)

	it("пропускает frontmatter и пустые строки до H1", function()
		write(root .. "/foo.md", "---\ntitle: meta\n---\n\n# Real Title\n")
		assert.equals("Real Title", notes.title(root .. "/foo.md"))
	end)

	it("возвращает имя файла без .md, когда H1 нет", function()
		write(root .. "/foo.md", "Just text, no header\n## Sub also no\n")
		assert.equals("foo", notes.title(root .. "/foo.md"))
	end)

	it("возвращает имя файла, когда путь не существует", function()
		assert.equals("missing", notes.title(root .. "/missing.md"))
	end)

	it("обрезает пробелы вокруг заголовка", function()
		write(root .. "/foo.md", "#   Spaced Title   \n")
		assert.equals("Spaced Title", notes.title(root .. "/foo.md"))
	end)

	it("пропускает ## Subheading и берёт первый # H1", function()
		write(root .. "/foo.md", "## Sub\n\n# Real Title\n")
		assert.equals("Real Title", notes.title(root .. "/foo.md"))
	end)
end)
