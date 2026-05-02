-- :MdxQuery и :MdxQueryInsert — picker по пользовательским Lua-скриптам.
--
-- Скрипты лежат как .lua файлы в `script_dir` (по умолчанию
-- ~/.config/mdx/scripts). Имя файла без `.lua` — название в picker'е.
-- Опциональная первая строка `-- description: ...` подмешивается
-- как описание.
--
-- Выполнение: содержимое файла загружается через loadstring, окружение
-- содержит глобальный `mdx` (см. mdx.script_api) и `vim`. Sandbox'а нет —
-- скрипты пишутся пользователем в своём же конфиге.
--
-- Возвращаемое значение приводится к строке через `to_string`. :MdxQuery
-- разбивает её по \n и показывает в Telescope-picker'е (Enter ничего
-- не делает). :MdxQueryInsert вставляет в текущий буфер в позицию курсора.

local M = {}

local DEFAULT_DIR = "~/.config/mdx/scripts"

function M.resolve_dir(config)
	local dir = (config and config.script_dir) or DEFAULT_DIR
	dir = vim.fn.expand(dir)
	if vim.fn.isdirectory(dir) ~= 1 then
		return nil, dir
	end
	return dir
end

function M.parse_script_file(content)
	local description
	local first_line = content:match("^([^\n]*)")
	if first_line then
		description = first_line:match("^%s*%-%-%s*description:%s*(.+)$")
		if description then
			description = description:gsub("%s+$", "")
		end
	end
	return {
		source = content,
		description = description,
	}
end

function M.list(config)
	local dir = M.resolve_dir(config)
	if not dir then
		return {}
	end

	local files = vim.fn.glob(dir .. "/*.lua", false, true)
	local result = {}
	for _, path in ipairs(files) do
		local name = vim.fn.fnamemodify(path, ":t:r")
		local lines = vim.fn.readfile(path)
		local content = table.concat(lines, "\n")
		local parsed = M.parse_script_file(content)
		table.insert(result, {
			name = name,
			path = path,
			source = parsed.source,
			description = parsed.description,
		})
	end
	table.sort(result, function(a, b)
		return a.name < b.name
	end)
	return result
end

-- Выполнить скрипт. Возвращает (result, err). result — то, что вернул
-- скрипт (любой Lua-тип); err — строка с описанием ошибки или nil.
function M.exec(script)
	local fn, err = loadstring(script.source, "mdx-script:" .. script.name)
	if not fn then
		return nil, "load error: " .. tostring(err)
	end

	local env = setmetatable({
		mdx = require("mdx.script_api"),
		vim = vim,
	}, { __index = _G })
	setfenv(fn, env)

	local ok, result = pcall(fn)
	if not ok then
		return nil, "error: " .. tostring(result)
	end
	return result, nil
end

-- Привести результат к строке.
--   string  → как есть
--   nil     → ""
--   массив строк → join("\n")
--   таблица → vim.inspect
--   прочее  → tostring
function M.to_string(result)
	if type(result) == "string" then
		return result
	end
	if result == nil then
		return ""
	end
	if type(result) == "table" then
		local all_strings = #result > 0
		for _, v in ipairs(result) do
			if type(v) ~= "string" then
				all_strings = false
				break
			end
		end
		if all_strings then
			return table.concat(result, "\n")
		end
		return vim.inspect(result)
	end
	return tostring(result)
end

local function open_picker(config, on_select)
	local scripts = M.list(config)
	if #scripts == 0 then
		local _, dir = M.resolve_dir(config)
		vim.notify(
			"mdx: no scripts in " .. (dir or DEFAULT_DIR),
			vim.log.levels.INFO
		)
		return
	end
	require("mdx.picker").pick_scripts(scripts, on_select)
end

local function run(script)
	local result, err = M.exec(script)
	if err then
		vim.notify("mdx: " .. err, vim.log.levels.ERROR)
		return nil
	end
	return M.to_string(result)
end

-- :MdxQuery — выполнить выбранный скрипт, показать результат в picker'е.
function M.query(config)
	open_picker(config, function(script)
		local text = run(script)
		if text == nil then
			return
		end
		if text == "" then
			vim.notify(
				string.format("mdx: %s returned empty result", script.name),
				vim.log.levels.INFO
			)
			return
		end
		local lines = vim.split(text, "\n", { plain = true })
		require("mdx.picker").pick_lines(script.name, lines)
	end)
end

-- :MdxQueryInsert — выполнить выбранный скрипт, вставить результат в курсор.
function M.query_insert(config)
	open_picker(config, function(script)
		local text = run(script)
		if text == nil then
			return
		end
		if text == "" then
			vim.notify(
				string.format("mdx: %s returned empty result", script.name),
				vim.log.levels.INFO
			)
			return
		end
		local lines = vim.split(text, "\n", { plain = true })
		vim.api.nvim_put(lines, "c", true, true)
	end)
end

return M
