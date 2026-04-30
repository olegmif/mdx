local M = {}

local BUILTIN_EXCLUDE = { "/.git/", "/node_modules/" }

function M.collect(root, opts)
	if not root or root == "" then
		return {}
	end

	opts = opts or {}
	local exclude = opts.exclude or {}

	local function is_excluded(path)
		for _, p in ipairs(BUILTIN_EXCLUDE) do
			if path:find(p, 1, true) then
				return true
			end
		end
		for _, p in ipairs(exclude) do
			if path:find(p, 1, true) then
				return true
			end
		end
		return false
	end

	local paths = vim.fs.find(function(name)
		return name:match("%.md$") ~= nil
	end, { path = root, type = "file", limit = math.huge })

	local result = {}
	for _, path in ipairs(paths) do
		if not is_excluded(path) then
			table.insert(result, path)
		end
	end

	table.sort(result)
	return result
end

function M.title(path) -- returns string
	local function fallback()
		return (vim.fs.basename(path):gsub("%.md$", ""))
	end

	local f = io.open(path, "r")
	if not f then
		return fallback()
	end

	for line in f:lines() do
		local heading = line:match("^#%s+(.+)$")
		if heading then
			heading = heading:match("^%s*(.-)%s*$")
			if heading ~= "" then
				f:close()
				return heading
			end
		end
	end

	f:close()
	return fallback()
end

return M
