local M = {}

local URL_SCHEMES = {
	"http://",
	"https://",
	"mailto:",
	"ftp://",
	"tel:",
	"#",
}

local function is_url_or_anchor(target)
	for _, scheme in ipairs(URL_SCHEMES) do
		if vim.startswith(target, scheme) then
			return true
		end
	end
	return false
end

function M.target_to_path(target, source_dir)
	if not target or target == "" then
		return nil
	end

	if is_url_or_anchor(target) then
		return nil
	end

	-- ~/foo или одиночное ~
	if target == "~" or vim.startswith(target, "~/") then
		local home = vim.fn.expand("~")
		return vim.fs.normalize(home .. target:sub(2))
	end

	-- абсолютный путь
	if vim.startswith(target, "/") then
		return vim.fs.normalize(target)
	end

	-- относительный путь
	return vim.fs.normalize(source_dir .. "/" .. target)
end

return M
