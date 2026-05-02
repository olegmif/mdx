local M = {}

-- Произвольный SELECT-запрос к БД через LSP. Возвращает массив строк
-- в виде { [col] = value, ... } или nil при ошибке.
-- Сервер отклонит всё, что не SELECT/WITH.
function M.run(sql, ...)
	local clients = vim.lsp.get_clients({ name = "mdx" })
	if #clients == 0 then
		vim.notify("mdx: LSP client not attached", vim.log.levels.ERROR)
		return nil
	end

	local args = { ... }
	local response = clients[1]:request_sync(
		"mdx/query",
		{ sql = sql, args = args },
		5000,
		0
	)

	if not response then
		vim.notify("mdx: query timed out", vim.log.levels.ERROR)
		return nil
	end

	if response.err then
		local msg = type(response.err) == "table" and response.err.message or tostring(response.err)
		vim.notify("mdx: query error: " .. msg, vim.log.levels.ERROR)
		return nil
	end

	return response.result or {}
end

return M
