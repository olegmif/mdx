-- :MdxSql — picker по заранее заготовленным SQL-запросам.
--
-- Запросы хранятся как отдельные .sql файлы в каталоге `query_dir`
-- (по умолчанию ~/.config/mdx/queries). Имя файла без `.sql` —
-- название запроса в picker'е. Опциональная первая строка вида
-- `-- description: ...` подмешивается как описание.
--
-- На запрос накладывается серверный фильтр SELECT/WITH-only, остальное
-- сервер отклонит до выполнения.

local M = {}

local DEFAULT_DIR = "~/.config/mdx/queries"

-- Резолвим каталог запросов. Возвращает абсолютный путь либо nil,
-- если каталог не существует.
function M.resolve_dir(config)
	local dir = (config and config.query_dir) or DEFAULT_DIR
	dir = vim.fn.expand(dir)
	if vim.fn.isdirectory(dir) ~= 1 then
		return nil, dir
	end
	return dir
end

-- Парсим содержимое файла. Возвращает { sql, description } —
-- description вытаскивается из первой строки `-- description: ...`,
-- если она есть.
function M.parse_query_file(content)
	local description
	local first_line = content:match("^([^\n]*)")
	if first_line then
		description = first_line:match("^%s*%-%-%s*description:%s*(.+)$")
		if description then
			description = description:gsub("%s+$", "")
		end
	end
	return {
		sql = content,
		description = description,
	}
end

-- Список доступных запросов. Возвращает массив
-- { name, path, sql, description }, отсортированный по name.
function M.list(config)
	local dir = M.resolve_dir(config)
	if not dir then
		return {}
	end

	local files = vim.fn.glob(dir .. "/*.sql", false, true)
	local result = {}
	for _, path in ipairs(files) do
		local name = vim.fn.fnamemodify(path, ":t:r")
		local lines = vim.fn.readfile(path)
		local content = table.concat(lines, "\n")
		local parsed = M.parse_query_file(content)
		table.insert(result, {
			name = name,
			path = path,
			sql = parsed.sql,
			description = parsed.description,
		})
	end
	table.sort(result, function(a, b)
		return a.name < b.name
	end)
	return result
end

-- Запустить выбранный запрос и открыть picker результатов.
function M.run(query)
	local rows = require("mdx.query").run(query.sql)
	if not rows then
		return -- вызов уже notify'нул
	end
	if #rows == 0 then
		vim.notify(
			string.format("mdx: query %q returned no rows", query.name),
			vim.log.levels.INFO
		)
		return
	end
	-- Каждая строка должна содержать поле "path" — иначе picker не сможет
	-- открыть заметку. Проверяем по первой строке.
	if rows[1].path == nil then
		vim.notify(
			"mdx: query result must include a 'path' column",
			vim.log.levels.ERROR
		)
		return
	end
	require("mdx.picker").sql_results(query.name, rows, function(row)
		vim.cmd.edit(vim.fn.fnameescape(row.path))
	end)
end

-- Точка входа :MdxSql — picker запросов, при выборе запускает запрос.
function M.open(config)
	local queries = M.list(config)
	if #queries == 0 then
		local _, dir = M.resolve_dir(config)
		vim.notify(
			"mdx: no queries in " .. (dir or DEFAULT_DIR),
			vim.log.levels.INFO
		)
		return
	end
	require("mdx.picker").sql_queries(queries, function(query)
		M.run(query)
	end)
end

return M
