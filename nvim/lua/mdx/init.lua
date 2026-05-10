local M = {}

local defaults = {
	keymaps = {
		-- follow / follow_split допускают как строку, так и список строк
		-- (в любом из вариантов false полностью отключает биндинг)
		follow = { "<leader>mf", "<CR>" },
		follow_split = { "<leader>ms", "<C-CR>" },
		insert_link = "<leader>mi",
		tag_search = "<leader>mt",
		extract = "<leader>me",
		new_note = "<leader>mn",
		sql = "<leader>mq",
		query = "<leader>mr",
		query_insert = "<leader>mR",
		search = "<leader>m/",
	},
	conceal = true,
	prompt_filename = false,
	query_dir = "~/.config/mdx/queries",
	script_dir = "~/.config/mdx/scripts",
	search = {
		mdx_bin = "mdx",
		limit = 30,
		timeout_ms = 30000,
	},
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
	require("mdx.picker").tag_search(M.config, function(entry)
		vim.cmd.edit(vim.fn.fnameescape(entry.path))
	end)
end

function M.insert_link()
	require("mdx.picker").open(M.config, function(entry)
		local insert = require("mdx.insert")
		local link = insert.format_link(entry.path, entry.title)
		insert.at_cursor(link)
	end)
end

function M.extract()
	require("mdx.extract").extract()
end

function M.new_note()
	require("mdx.new").new()
end

function M.sql()
	require("mdx.sql").open(M.config)
end

function M.query()
	require("mdx.script").query(M.config)
end

function M.query_insert()
	require("mdx.script").query_insert(M.config)
end

function M.search(query)
	require("mdx.search").run(query)
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
	vim.api.nvim_create_user_command("MdxExtract", function()
		require("mdx").extract()
	end, { range = true })
	vim.api.nvim_create_user_command("MdxNew", function()
		require("mdx").new_note()
	end, {})
	vim.api.nvim_create_user_command("MdxSql", function()
		require("mdx").sql()
	end, {})
	vim.api.nvim_create_user_command("MdxQuery", function()
		require("mdx").query()
	end, {})
	vim.api.nvim_create_user_command("MdxQueryInsert", function()
		require("mdx").query_insert()
	end, {})
	vim.api.nvim_create_user_command("MdxSearch", function(opts)
		local q = opts.args ~= "" and opts.args or nil
		require("mdx").search(q)
	end, { nargs = "?" })
end

return M
