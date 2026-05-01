local M = {}

local defaults = {
	keymaps = {
		follow = "<leader>mf",
		follow_split = "<leader>ms",
		insert_link = "<leader>mi",
		tag_search = "<leader>mt",
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

function M.tag_search()
	require("mdx.picker").tag_search(M.config, function() end)
end

function M.insert_link()
	require("mdx.picker").open(M.config, function(entry)
		local insert = require("mdx.insert")
		local link = insert.format_link(entry.path, entry.title)
		insert.at_cursor(link)
	end)
end

function M.setup(opts)
	M.config = vim.tbl_deep_extend("force", defaults, opts or {})
	vim.api.nvim_create_user_command("MdxFollow", M.follow, {})
	vim.api.nvim_create_user_command("MdxFollowSplit", M.follow_split, {})
	vim.api.nvim_create_user_command("MdxInsertLink", function()
		require("mdx").insert_link()
	end, {})
	vim.api.nvim_create_user_command("MdxTagSearch", function()
		require("mdx").tag_search()
	end, {})
end

return M
