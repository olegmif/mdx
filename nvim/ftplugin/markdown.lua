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
