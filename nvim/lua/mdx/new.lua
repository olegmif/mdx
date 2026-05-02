local M = {}

local function target_dir()
	local d = vim.fn.expand("%:p:h")
	if d == "" then
		return vim.fn.getcwd()
	end
	return d
end

function M.new()
	local config = require("mdx").config

	local filename = require("mdx.filename").get(config)
	if not filename then
		return
	end

	local target = target_dir() .. "/" .. filename

	if vim.fn.filereadable(target) == 1 then
		vim.notify("mdx: file already exists: " .. target, vim.log.levels.ERROR)
		return
	end

	-- Pre-create empty file: чтобы LSP didOpen смог сделать Stat и проиндексировать.
	if vim.fn.writefile({}, target) ~= 0 then
		vim.notify("mdx: failed to write " .. target, vim.log.levels.ERROR)
		return
	end

	vim.cmd.edit(vim.fn.fnameescape(target))
end

return M
