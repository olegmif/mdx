vim.opt.rtp:prepend(vim.fn.expand("~/projects/mdx/nvim"))

require("mdx").setup({})

local function make_tmpdir()
	local dir = vim.fn.tempname()
	vim.fn.mkdir(dir, "p")
	return dir
end

describe("mdx.new", function()
	local tmpdir

	before_each(function()
		tmpdir = make_tmpdir()
		require("mdx").config.prompt_filename = false
		-- Открываем существующий файл в tmpdir, чтобы expand("%:p:h") вернул tmpdir.
		local seed = tmpdir .. "/seed.md"
		vim.fn.writefile({ "" }, seed)
		vim.cmd.edit(vim.fn.fnameescape(seed))
	end)

	after_each(function()
		vim.fn.delete(tmpdir, "rf")
	end)

	it("создаёт пустой файл по таймштампу и открывает его в текущем буфере", function()
		require("mdx.new").new()

		-- Текущий буфер указывает на новый файл.
		local current = vim.api.nvim_buf_get_name(0)
		assert.is_truthy(current:match("/%d%d%d%d%d%d%d%d%d%d%d%d%d%d%.md$"))

		-- Файл существует и пустой.
		assert.equals(1, vim.fn.filereadable(current))
		local content = vim.fn.readfile(current)
		assert.is_true(#content == 0 or (#content == 1 and content[1] == ""))
	end)

	it("отказывается перезаписывать существующий файл", function()
		local original_input = vim.fn.input
		vim.fn.input = function()
			return "fixed-name"
		end
		require("mdx").config.prompt_filename = true

		-- Создаём файл с тем именем, которое будет запрошено.
		local clash = tmpdir .. "/fixed-name.md"
		vim.fn.writefile({ "old content" }, clash)

		require("mdx.new").new()

		vim.fn.input = original_input
		require("mdx").config.prompt_filename = false

		-- Содержимое не изменилось.
		assert.are.same({ "old content" }, vim.fn.readfile(clash))
	end)
end)
