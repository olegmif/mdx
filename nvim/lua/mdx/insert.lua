local M = {}

local function split_path(p)
	local parts = {}
	for part in p:gmatch("[^/]+") do
		table.insert(parts, part)
	end
	return parts
end

local function compute_rel(source_dir, target_path)
	source_dir = vim.fs.normalize(source_dir)
	target_path = vim.fs.normalize(target_path)

	local base = split_path(source_dir)
	local target = split_path(target_path)

	local common = 0
	while common < #base and common < #target and base[common + 1] == target[common + 1] do
		common = common + 1
	end

	local result = {}
	for _ = common + 1, #base do
		table.insert(result, "..")
	end
	for i = common + 1, #target do
		table.insert(result, target[i])
	end

	if #result == 0 then
		return "."
	end
	return table.concat(result, "/")
end

function M.format_link(target_path, source_dir, title) -- returns string
	local rel = compute_rel(source_dir, target_path)
	if not rel:match("^%.") and not rel:match("^/") then
		rel = "./" .. rel
	end
	return "[" .. title .. "](" .. rel .. ")"
end

function M.at_cursor(link_string)
	vim.api.nvim_put({ link_string }, "c", true, true)
end

return M
