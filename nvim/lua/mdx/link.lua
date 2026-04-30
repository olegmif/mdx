local M = {}

function M.under_cursor()
	local row, col = unpack(vim.api.nvim_win_get_cursor(0))
	row = row - 1

	local node = vim.treesitter.get_node({ pos = { row, col }, ignore_injections = false })
	if not node then
		return nil
	end

	while node and node:type() ~= "inline_link" do
		node = node:parent()
	end
	if not node then
		return nil
	end

	local text_node, dest_node
	for child in node:iter_children() do
		local t = child:type()
		if t == "link_text" then
			text_node = child
		elseif t == "link_destination" then
			dest_node = child
		end
	end
	if not text_node or not dest_node then
		return nil
	end

	local bufnr = vim.api.nvim_get_current_buf()
	local _, start_col, _, end_col = node:range()

	return {
		text = vim.treesitter.get_node_text(text_node, bufnr),
		target = vim.treesitter.get_node_text(dest_node, bufnr),
		start = start_col,
		finish = end_col,
	}
end

return M
