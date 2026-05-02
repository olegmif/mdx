-- Публичный API, доступный внутри пользовательских скриптов в каталоге
-- script_dir как глобальный mdx.*. Только обёртки над имеющимися
-- LSP-методами и тривиальные построения поверх них.

local M = {}

local function lsp_ready()
	return #vim.lsp.get_clients({ name = "mdx" }) > 0
end

function M.notes()
	if not lsp_ready() then
		return {}
	end
	return require("mdx.notes").list() or {}
end

function M.search_tags(include, exclude)
	if not lsp_ready() then
		return {}
	end
	return require("mdx.tags").search(include or {}, exclude or {}) or {}
end

function M.notes_today()
	local today = os.date("%Y-%m-%d")
	local result = {}
	for _, n in ipairs(M.notes()) do
		local mtime = vim.fn.getftime(n.path)
		if mtime > 0 and os.date("%Y-%m-%d", mtime) == today then
			table.insert(result, n)
		end
	end
	return result
end

function M.query(sql, ...)
	if not lsp_ready() then
		return {}
	end
	return require("mdx.query").run(sql, ...) or {}
end

return M
