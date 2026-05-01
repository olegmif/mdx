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
	return nil
end

return M
