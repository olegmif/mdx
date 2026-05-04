vim.opt.rtp:prepend(vim.fn.expand("~/projects/mdx/nvim"))

local search = require("mdx.search")

describe("mdx.search.parse", function()
	it("nil stdout → пустой массив, ошибки нет", function()
		local hits, err = search.parse(nil)
		assert.are.same({}, hits)
		assert.is_nil(err)
	end)

	it("пустая строка → пустой массив, ошибки нет", function()
		local hits, err = search.parse("")
		assert.are.same({}, hits)
		assert.is_nil(err)
	end)

	it("массив из трёх hits с title и без — сохраняется как есть", function()
		local input = vim.fn.json_encode({
			{ path = "a.md", score = 0.9, title = "Alpha" },
			{ path = "b.md", score = 0.5 },
			{ path = "c.md", score = 0.1, title = "Gamma" },
		})
		local hits, err = search.parse(input)
		assert.is_nil(err)
		assert.equals(3, #hits)
		assert.equals("a.md", hits[1].path)
		assert.equals("Alpha", hits[1].title)
		assert.equals("b.md", hits[2].path)
		assert.is_nil(hits[2].title)
		assert.equals("c.md", hits[3].path)
		assert.equals("Gamma", hits[3].title)
	end)

	it("невалидный JSON → nil и непустая ошибка", function()
		local hits, err = search.parse("{not json")
		assert.is_nil(hits)
		assert.is_string(err)
		assert.is_true(#err > 0)
	end)

	it("JSON-объект вместо массива → nil и сообщение про array", function()
		local hits, err = search.parse('{"foo":"bar"}')
		assert.is_nil(hits)
		assert.is_string(err)
		assert.is_truthy(err:lower():find("array"))
	end)

	it("JSON-скаляр вместо массива → nil и сообщение про array", function()
		local hits, err = search.parse("42")
		assert.is_nil(hits)
		assert.is_string(err)
		assert.is_truthy(err:lower():find("array"))
	end)
end)
