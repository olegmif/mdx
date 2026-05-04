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

-- M.invoke шлёт `mdx search --format json --limit N <query>` через
-- vim.system с timeout. Callback on_done(hits, err) вызывается из
-- vim.schedule_wrap. hits == nil сигнализирует об ошибке; err содержит
-- человекочитаемое сообщение, готовое для vim.notify.
function M.invoke(query, on_done)
	local cfg = require("mdx").config.search
	local cmd = {
		cfg.mdx_bin,
		"search",
		"--format",
		"json",
		"--limit",
		tostring(cfg.limit),
		query,
	}
	vim.system(cmd, { text = true, timeout = cfg.timeout_ms }, vim.schedule_wrap(function(obj)
		if obj.code ~= 0 then
			local stderr = (obj.stderr and obj.stderr ~= "") and obj.stderr or "exit code " .. tostring(obj.code)
			on_done(nil, "search failed: " .. stderr)
			return
		end
		local hits, err = M.parse(obj.stdout)
		if not hits then
			on_done(nil, "failed to parse search output: " .. err)
			return
		end
		on_done(hits, nil)
	end))
end

local function show(query)
	M.invoke(query, function(hits, err)
		if err then
			vim.notify("mdx: " .. err, vim.log.levels.ERROR)
			return
		end
		if #hits == 0 then
			vim.notify("mdx: no results", vim.log.levels.INFO)
			return
		end
		require("mdx.picker").search_results(hits, function(hit)
			vim.cmd.edit(vim.fn.fnameescape(hit.path))
		end)
	end)
end

function M.run(query)
	if query and query ~= "" then
		show(query)
		return
	end
	vim.ui.input({ prompt = "MdxSearch: " }, function(input)
		if not input or input == "" then
			return
		end
		show(input)
	end)
end

return M
