local M = {}

function M.format_link(path, title)
	local normalized = vim.fs.normalize(path)
	local home = vim.fs.normalize(vim.fn.expand("~"))

	local link_path
	if normalized == home then
		link_path = "~"
	elseif vim.startswith(normalized, home .. "/") then
		link_path = "~" .. normalized:sub(#home + 1)
	else
		link_path = normalized
	end

	return "[" .. title .. "](" .. link_path .. ")"
end

function M.at_cursor(link_string)
	return nil
end

return M
