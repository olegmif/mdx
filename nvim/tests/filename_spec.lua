vim.opt.rtp:prepend(vim.fn.expand("~/projects/mdx/nvim"))

local filename = require("mdx.filename")

describe("mdx.filename.timestamp", function()
	it("формат YYYYMMDDHHMMSS.md", function()
		local name = filename.timestamp()
		assert.is_truthy(name:match("^%d%d%d%d%d%d%d%d%d%d%d%d%d%d%.md$"))
	end)
end)

describe("mdx.filename.get", function()
	it("без prompt_filename → таймштамп", function()
		local name = filename.get({ prompt_filename = false })
		assert.is_truthy(name:match("^%d%d%d%d%d%d%d%d%d%d%d%d%d%d%.md$"))
	end)

	it("config = nil → таймштамп", function()
		local name = filename.get(nil)
		assert.is_truthy(name:match("^%d%d%d%d%d%d%d%d%d%d%d%d%d%d%.md$"))
	end)

	it("prompt_filename + ввод имени → имя с .md", function()
		local original = vim.fn.input
		vim.fn.input = function()
			return "myname"
		end
		local name = filename.get({ prompt_filename = true })
		vim.fn.input = original
		assert.equals("myname.md", name)
	end)

	it("prompt_filename + имя уже с .md → без удвоения", function()
		local original = vim.fn.input
		vim.fn.input = function()
			return "myname.md"
		end
		local name = filename.get({ prompt_filename = true })
		vim.fn.input = original
		assert.equals("myname.md", name)
	end)

	it("prompt_filename + пустой ввод → таймштамп (default)", function()
		local original = vim.fn.input
		vim.fn.input = function()
			return ""
		end
		local name = filename.get({ prompt_filename = true })
		vim.fn.input = original
		assert.is_truthy(name:match("^%d%d%d%d%d%d%d%d%d%d%d%d%d%d%.md$"))
	end)
end)
