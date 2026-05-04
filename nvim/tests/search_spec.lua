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

describe("mdx.search.run integration", function()
	local picker = require("mdx.picker")

	local saved = {}
	local notifications

	before_each(function()
		notifications = {}
		saved.ui_input = vim.ui.input
		saved.notify = vim.notify
		saved.invoke = search.invoke
		saved.search_results = picker.search_results
		vim.notify = function(msg, lvl)
			table.insert(notifications, { msg = msg, lvl = lvl })
		end
	end)

	after_each(function()
		vim.ui.input = saved.ui_input
		vim.notify = saved.notify
		search.invoke = saved.invoke
		picker.search_results = saved.search_results
	end)

	it("полная цепочка ui.input → invoke → picker → edit", function()
		local target = "/tmp/mdx-test-search-flow.md"
		vim.fn.writefile({ "x" }, target)

		vim.ui.input = function(_, cb)
			cb("query")
		end
		search.invoke = function(query, cb)
			assert.equals("query", query)
			cb({ { path = target, score = 0.9, title = "Foo" } }, nil)
		end
		picker.search_results = function(hits, on_select)
			assert.equals(1, #hits)
			on_select(hits[1])
		end

		require("mdx").search()
		assert.equals(target, vim.api.nvim_buf_get_name(0))
		assert.equals(0, #notifications)
		os.remove(target)
	end)

	it("пустой результат → INFO notify, picker не зовётся", function()
		local picker_called = false
		search.invoke = function(_, cb)
			cb({}, nil)
		end
		picker.search_results = function()
			picker_called = true
		end

		require("mdx").search("anything")

		assert.is_false(picker_called)
		assert.equals(1, #notifications)
		assert.equals(vim.log.levels.INFO, notifications[1].lvl)
		assert.is_truthy(notifications[1].msg:lower():find("no results"))
	end)

	it("ошибка invoke → ERROR notify, picker не зовётся", function()
		local picker_called = false
		search.invoke = function(_, cb)
			cb(nil, "search failed: boom")
		end
		picker.search_results = function()
			picker_called = true
		end

		require("mdx").search("anything")

		assert.is_false(picker_called)
		assert.equals(1, #notifications)
		assert.equals(vim.log.levels.ERROR, notifications[1].lvl)
		assert.is_truthy(notifications[1].msg:find("boom"))
	end)
end)
