local M = {}

function M.setup(opts)
	vim.api.nvim_create_user_command("MdxFollow", M.follow, {})
	vim.api.nvim_create_user_command("MdxFollowSplit", M.follow_split, {})
end

local function open_link(opener)
	local link = require("mdx.link").under_cursor()
	if not link then
		vim.notify("mdx: no link under cursor", vim.log.levels.INFO)
		return false
	end

	local source_dir = vim.fs.dirname(vim.api.nvim_buf_get_name(0))
	local path = require("mdx.resolve").target_to_path(link.target, source_dir)
	if not path then
		vim.notify("mdx: external URL, ignored", vim.log.levels.INFO)
		return true
	end
	opener(vim.fn.fnameescape(path))
	return true
end

function M.follow()
	return open_link(vim.cmd.edit)
end

function M.follow_split()
	return open_link(vim.cmd.vsplit)
end

return M
