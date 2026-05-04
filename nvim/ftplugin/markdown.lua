-- mdx: настройки markdown-буфера. Применяются для буфера на основе M.congig

local mdx = require("mdx")
local config = mdx.config

if config.conceal then
	vim.opt_local.conceallevel = 2
	vim.opt_local.concealcursor = ""
end

if config.keymaps.follow then
	vim.keymap.set("n", config.keymaps.follow, function()
		mdx.follow()
	end, { buffer = true, desc = "mdx: follow link" })
end

if config.keymaps.follow_split then
	vim.keymap.set("n", config.keymaps.follow_split, function()
		mdx.follow_split()
	end, { buffer = true, desc = "mdx: follow link in vsplit" })
end

if config.keymaps.insert_link then
	vim.keymap.set("n", config.keymaps.insert_link, function()
		mdx.insert_link()
	end, { buffer = true, desc = "mdx: insert link to existing note" })
end

if config.keymaps.tag_search then
	vim.keymap.set("n", config.keymaps.tag_search, function()
		mdx.tag_search()
	end, { buffer = true, desc = "mdx: search notes by tag" })
end

if config.keymaps.extract then
	-- :<C-u> сбрасывает auto-вставленный '<,'> в командной строке;
	-- к моменту запуска MdxExtract метки '< и '> уже выставлены.
	vim.keymap.set("x", config.keymaps.extract, ":<C-u>MdxExtract<CR>", {
		buffer = true,
		desc = "mdx: extract selection to new note",
	})
end

if config.keymaps.new_note then
	vim.keymap.set("n", config.keymaps.new_note, function()
		mdx.new_note()
	end, { buffer = true, desc = "mdx: create new note" })
end

if config.keymaps.sql then
	vim.keymap.set("n", config.keymaps.sql, function()
		mdx.sql()
	end, { buffer = true, desc = "mdx: run saved sql query" })
end

if config.keymaps.query then
	vim.keymap.set("n", config.keymaps.query, function()
		mdx.query()
	end, { buffer = true, desc = "mdx: run saved lua script (show)" })
end

if config.keymaps.query_insert then
	vim.keymap.set("n", config.keymaps.query_insert, function()
		mdx.query_insert()
	end, { buffer = true, desc = "mdx: run saved lua script (insert)" })
end

if config.keymaps.search then
	vim.keymap.set("n", config.keymaps.search, function()
		mdx.search()
	end, { buffer = true, desc = "mdx: dense search across notes" })
end
