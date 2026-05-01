local M = {}

function M.parse_query(prompt) -- returns include[], exclude[]
	local include, exclude = {}, {}

	if not prompt then
		return include, exclude
	end

	local trailing_space = prompt:sub(-1):match("%s") ~= nil

	local parts = {}
	for tok in prompt:gmatch("%S+") do
		table.insert(parts, tok)
	end

	if not trailing_space and #parts > 0 then
		table.remove(parts)
	end

	for _, tok in ipairs(parts) do
		local first = tok:sub(1, 1)
		if first == "-" then
			local rest = tok:sub(2)
			if rest ~= "" then
				table.insert(exclude, rest)
			end
		elseif first == "+" then
			local rest = tok:sub(2)
			if rest ~= "" then
				table.insert(include, rest)
			end
		elseif tok ~= "" then
			table.insert(include, tok)
		end
	end

	return include, exclude
end

function M.search(include, exclude) -- returns array of {path, title} or nil
	local clients = vim.lsp.get_clients({ name = "mdx" })
	if #clients == 0 then
		vim.notify("mdx: LSP client not attached", vim.log.levels.ERROR)
		return nil
	end

	local response = clients[1]:request_sync("mdx/searchByTags", { include = include, exclude = exclude }, 2000, 0)

	if not response then
		vim.notify("mdx: searchByTags timed out", vim.log.levels.ERROR)
		return nil
	end

	if response.err then
		local msg = type(response.err) == "table" and response.err.message or tostring(response.err)
		vim.notify("mdx: searchByTags error: " .. msg, vim.log.levels.ERROR)
		return nil
	end

	return response.result
end

return M
