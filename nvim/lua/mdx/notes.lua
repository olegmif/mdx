local M = {}

function M.list()
	local clients = vim.lsp.get_clients({ name = "mdx" })
	if #clients == 0 then
		vim.notify("mdx: LSP client not attached", vim.log.levels.ERROR)
		return nil
	end

	local response = clients[1]:request_sync("mdx/listNotes", vim.empty_dict(), 2000, 0)

	if not response then
		vim.notify("mdx: listNotes timed out", vim.log.levels.ERROR)
		return nil
	end

	if response.err then
		local msg = type(response.err) == "table" and response.err.message or tostring(response.err)
		vim.notify("mdx: listNotes error: " .. msg, vim.log.levels.ERROR)
		return nil
	end

	return response.result
end

return M
