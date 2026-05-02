vim.opt.rtp:prepend(vim.fn.expand("~/projects/mdx/nvim"))

local sql = require("mdx.sql")

local function make_tmpdir()
	local dir = vim.fn.tempname()
	vim.fn.mkdir(dir, "p")
	return dir
end

local function write(path, content)
	vim.fn.writefile(vim.split(content, "\n"), path)
end

describe("mdx.sql.parse_query_file", function()
	it("description из первой строки -- description: ...", function()
		local got = sql.parse_query_file("-- description: Active tasks\nSELECT path FROM notes")
		assert.equals("Active tasks", got.description)
		assert.equals("-- description: Active tasks\nSELECT path FROM notes", got.sql)
	end)

	it("description с trailing whitespace обрезается", function()
		local got = sql.parse_query_file("-- description: Hello   \nSELECT 1")
		assert.equals("Hello", got.description)
	end)

	it("без описания → description = nil", function()
		local got = sql.parse_query_file("SELECT path FROM notes")
		assert.is_nil(got.description)
	end)

	it("обычный комментарий без description: тоже nil", function()
		local got = sql.parse_query_file("-- just a regular comment\nSELECT 1")
		assert.is_nil(got.description)
	end)
end)

describe("mdx.sql.list", function()
	local tmpdir

	before_each(function()
		tmpdir = make_tmpdir()
	end)

	after_each(function()
		vim.fn.delete(tmpdir, "rf")
	end)

	it("возвращает [] для несуществующего каталога", function()
		assert.are.same({}, sql.list({ query_dir = tmpdir .. "/nope" }))
	end)

	it("возвращает [] для пустого каталога", function()
		assert.are.same({}, sql.list({ query_dir = tmpdir }))
	end)

	it("находит .sql файлы и сортирует по имени", function()
		write(tmpdir .. "/zebra.sql", "SELECT 1")
		write(tmpdir .. "/alpha.sql", "-- description: First\nSELECT 2")
		write(tmpdir .. "/middle.sql", "SELECT 3")
		-- non-.sql файл должен быть проигнорирован
		write(tmpdir .. "/notes.txt", "ignored")

		local result = sql.list({ query_dir = tmpdir })
		assert.equals(3, #result)
		assert.equals("alpha", result[1].name)
		assert.equals("First", result[1].description)
		assert.is_truthy(result[1].sql:match("SELECT 2"))
		assert.equals("middle", result[2].name)
		assert.is_nil(result[2].description)
		assert.equals("zebra", result[3].name)
	end)
end)

describe("mdx.sql.resolve_dir", function()
	it("nil если каталог не существует", function()
		local dir, attempted = sql.resolve_dir({ query_dir = "/no/such/path/here" })
		assert.is_nil(dir)
		assert.equals("/no/such/path/here", attempted)
	end)

	it("возвращает абсолютный путь, если каталог есть", function()
		local tmpdir = make_tmpdir()
		local dir = sql.resolve_dir({ query_dir = tmpdir })
		assert.equals(tmpdir, dir)
		vim.fn.delete(tmpdir, "rf")
	end)

	it("раскрывает ~", function()
		local home_dir = vim.fn.expand("~")
		local dir = sql.resolve_dir({ query_dir = "~" })
		assert.equals(home_dir, dir)
	end)
end)
