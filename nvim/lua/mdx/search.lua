local M = {}

-- M.parse декодирует stdout из `mdx search --format json` в массив
-- hit-объектов вида { path = string, score = number, title = string? }.
-- nil/пустая строка — допустимый «пустой результат». Невалидный JSON или
-- JSON-объект на верхнем уровне возвращают nil и непустую строку ошибки.
function M.parse(stdout)
	if stdout == nil or stdout == "" then
		return {}, nil
	end
	local ok, decoded = pcall(vim.json.decode, stdout)
	if not ok then
		return nil, tostring(decoded)
	end
	if type(decoded) ~= "table" then
		return nil, "expected JSON array, got " .. type(decoded)
	end
	-- Lua не различает array и object на уровне таблиц; считаем object'ом
	-- любую таблицу с хотя бы одним нечисловым ключом.
	for k in pairs(decoded) do
		if type(k) ~= "number" then
			return nil, "expected JSON array, got object"
		end
		break
	end
	return decoded, nil
end

function M.run(query)
	vim.notify("mdx: search not implemented", vim.log.levels.INFO)
end

return M
