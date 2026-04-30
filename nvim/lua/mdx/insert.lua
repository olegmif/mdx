local M = {}

function M.to_display_path(path)
	local normalized = vim.fs.normalize(path)
	local home = vim.fs.normalize(vim.fn.expand("~"))

	if normalized == home then
		return "~"
	elseif vim.startswith(normalized, home .. "/") then
		return "~" .. normalized:sub(#home + 1)
	else
		return normalized
	end
end

function M.format_link(path, title)
	return "[" .. title .. "](" .. M.to_display_path(path) .. ")"
end

function M.at_cursor(link_string)
	vim.api.nvim_put({ link_string }, "c", true, true)
end

return M
