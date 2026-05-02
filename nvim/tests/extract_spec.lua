vim.opt.rtp:prepend(vim.fn.expand("~/projects/mdx/nvim"))

require("mdx").setup({})

local function make_tmpdir()
	local dir = vim.fn.tempname()
	vim.fn.mkdir(dir, "p")
	return dir
end

local function setup_buffer_in_dir(dir, lines)
	local path = dir .. "/source.md"
	vim.fn.writefile({}, path)
	vim.cmd.edit(vim.fn.fnameescape(path))
	vim.api.nvim_buf_set_lines(0, 0, -1, false, lines)
	return path
end

describe("mdx.extract", function()
	local tmpdir

	before_each(function()
		tmpdir = make_tmpdir()
		require("mdx").config.prompt_filename = false
	end)

	after_each(function()
		vim.fn.delete(tmpdir, "rf")
	end)

	it("пустые метки → notify, без изменений", function()
		setup_buffer_in_dir(tmpdir, { "untouched" })
		vim.fn.setpos("'<", { 0, 0, 0, 0 })
		vim.fn.setpos("'>", { 0, 0, 0, 0 })

		require("mdx.extract").extract()

		assert.equals("untouched", vim.api.nvim_buf_get_lines(0, 0, 1, false)[1])
		local files = vim.fn.glob(tmpdir .. "/*.md", false, true)
		assert.equals(1, #files) -- только исходный source.md
	end)
end)
