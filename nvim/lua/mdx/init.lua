local M = {}

local defaults = {
	keymaps = {
		follow = "<leader>mf",
		follow_split = "<leader>ms",
	},
	conceal = true,
}

M.config = vim.deepcopy(defaults)

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

function M.insert_link()
	local buf_name = vim.api.nvim_buf_get_name(0)
	local source_dir = vim.fs.dirname(buf_name)
	if source_dir == "" or source_dir == "." then
		source_dir = vim.fn.getcwd()
	end

	require("mdx.picker").open(M.config, function(entry)
		local link = require("mdx.insert").format_link(entry.path, source_dir, entry.title)
		require("mdx.insert").at_cursor(link)
	end)
end

function M.setup(opts)
	M.config = vim.tbl_deep_extend("force", defaults, opts or {})
	vim.api.nvim_create_user_command("MdxFollow", M.follow, {})
	vim.api.nvim_create_user_command("MdxFollowSplit", M.follow_split, {})
	vim.api.nvim_create_user_command("MdxInsertLink", function()
		require("mdx").insert_link()
	end, {})
end

return M
