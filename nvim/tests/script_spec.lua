vim.opt.rtp:prepend(vim.fn.expand("~/projects/mdx/nvim"))

local script = require("mdx.script")

local function make_tmpdir()
	local dir = vim.fn.tempname()
	vim.fn.mkdir(dir, "p")
	return dir
end

local function write(path, content)
	vim.fn.writefile(vim.split(content, "\n"), path)
end

describe("mdx.script.parse_script_file", function()
	it("description из первой строки -- description: ...", function()
		local got = script.parse_script_file("-- description: My report\nreturn 42")
		assert.equals("My report", got.description)
		assert.equals("-- description: My report\nreturn 42", got.source)
	end)

	it("description с trailing whitespace обрезается", function()
		local got = script.parse_script_file("-- description: Hello   \nreturn 1")
		assert.equals("Hello", got.description)
	end)

	it("без описания → description = nil", function()
		local got = script.parse_script_file("return 1")
		assert.is_nil(got.description)
	end)

	it("обычный комментарий без description: тоже nil", function()
		local got = script.parse_script_file("-- just a regular comment\nreturn 1")
		assert.is_nil(got.description)
	end)
end)

describe("mdx.script.list", function()
	local tmpdir

	before_each(function()
		tmpdir = make_tmpdir()
	end)

	after_each(function()
		vim.fn.delete(tmpdir, "rf")
	end)

	it("возвращает [] для несуществующего каталога", function()
		assert.are.same({}, script.list({ script_dir = tmpdir .. "/nope" }))
	end)

	it("возвращает [] для пустого каталога", function()
		assert.are.same({}, script.list({ script_dir = tmpdir }))
	end)

	it("находит .lua файлы и сортирует по имени", function()
		write(tmpdir .. "/zebra.lua", "return 1")
		write(tmpdir .. "/alpha.lua", "-- description: First\nreturn 2")
		write(tmpdir .. "/middle.lua", "return 3")
		write(tmpdir .. "/notes.txt", "ignored")

		local result = script.list({ script_dir = tmpdir })
		assert.equals(3, #result)
		assert.equals("alpha", result[1].name)
		assert.equals("First", result[1].description)
		assert.equals("middle", result[2].name)
		assert.is_nil(result[2].description)
		assert.equals("zebra", result[3].name)
	end)
end)

describe("mdx.script.exec", function()
	it("возвращает значение скрипта", function()
		local result, err = script.exec({ name = "t", source = 'return "hello"' })
		assert.is_nil(err)
		assert.equals("hello", result)
	end)

	it("number возвращается как есть", function()
		local result, err = script.exec({ name = "t", source = "return 42" })
		assert.is_nil(err)
		assert.equals(42, result)
	end)

	it("ошибка компиляции → err установлен", function()
		local result, err = script.exec({ name = "t", source = 'return "unterminated' })
		assert.is_nil(result)
		assert.is_truthy(err:match("^load error:"))
	end)

	it("ошибка выполнения → err установлен", function()
		local result, err = script.exec({ name = "t", source = 'error("oops")' })
		assert.is_nil(result)
		assert.is_truthy(err:match("^error:"))
		assert.is_truthy(err:match("oops"))
	end)

	it("mdx-API доступен в окружении", function()
		local result, err = script.exec({
			name = "t",
			source = "return type(mdx.notes())",
		})
		assert.is_nil(err)
		assert.equals("table", result)
	end)

	it("vim API доступен в окружении", function()
		local result, err = script.exec({
			name = "t",
			source = 'return vim.fn.has("nvim") == 1',
		})
		assert.is_nil(err)
		assert.is_true(result)
	end)

	it("стандартные globals доступны", function()
		local result, err = script.exec({
			name = "t",
			source = 'return string.upper("foo")',
		})
		assert.is_nil(err)
		assert.equals("FOO", result)
	end)
end)

describe("mdx.script.to_string", function()
	it("string возвращается как есть", function()
		assert.equals("hi", script.to_string("hi"))
	end)

	it("nil → пустая строка", function()
		assert.equals("", script.to_string(nil))
	end)

	it("массив строк джойнится через \\n", function()
		assert.equals("a\nb\nc", script.to_string({ "a", "b", "c" }))
	end)

	it("таблица записей через vim.inspect", function()
		local out = script.to_string({ { path = "/a.md" } })
		assert.is_truthy(out:match("path"))
	end)

	it("число через tostring", function()
		assert.equals("42", script.to_string(42))
	end)

	it("пустая таблица → vim.inspect (не пустая строка)", function()
		-- Пустая таблица не является «массивом строк» (#result == 0),
		-- поэтому идёт через vim.inspect → "{}".
		assert.equals("{}", script.to_string({}))
	end)
end)
