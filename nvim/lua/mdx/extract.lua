local M = {}

-- Каталог нового файла: каталог текущего буфера, либо cwd если буфер без имени.
local function target_dir()
	local d = vim.fn.expand("%:p:h")
	if d == "" then
		return vim.fn.getcwd()
	end
	return d
end

function M.extract()
	local config = require("mdx").config

	local s = vim.fn.getpos("'<")
	local e = vim.fn.getpos("'>")
	if s[2] == 0 or e[2] == 0 then
		vim.notify("mdx: no visual selection", vim.log.levels.WARN)
		return
	end

	local mode = vim.fn.visualmode()
	if mode == "" then
		vim.notify("mdx: no visual selection", vim.log.levels.WARN)
		return
	end

	local lines = vim.fn.getregion(s, e, { type = mode })
	if #lines == 0 or (#lines == 1 and lines[1] == "") then
		vim.notify("mdx: empty selection", vim.log.levels.WARN)
		return
	end

	local filename = require("mdx.filename").get(config)
	if not filename then
		return
	end

	local target = target_dir() .. "/" .. filename

	if vim.fn.filereadable(target) == 1 then
		vim.notify("mdx: file already exists: " .. target, vim.log.levels.ERROR)
		return
	end

	-- Записываем выделенный фрагмент в новый файл.
	if vim.fn.writefile(lines, target) ~= 0 then
		vim.notify("mdx: failed to write " .. target, vim.log.levels.ERROR)
		return
	end

	-- Формируем ссылку через тот же хелпер, что и :MdxInsertLink.
	local insert = require("mdx.insert")
	local title = filename:gsub("%.md$", "")
	local link = insert.format_link(target, title)

	-- Заменяем visual-выделение на ссылку через регистр z (default не трогаем).
	vim.fn.setreg("z", link, "c")
	vim.cmd('normal! gv"zp')

	-- Открываем новый файл в вертикальном split.
	vim.cmd("vsplit " .. vim.fn.fnameescape(target))
end

return M
