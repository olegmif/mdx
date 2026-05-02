local M = {}

function M.timestamp()
	return os.date("%Y%m%d%H%M%S") .. ".md"
end

-- Возвращает имя файла согласно конфигу.
-- Если config.prompt_filename = true — запрашивает у пользователя через vim.fn.input
-- с дефолтом-таймштампом; пустой ввод → таймштамп.
-- Если false — сразу возвращает таймштамп.
-- Возвращает nil, если пользователь прервал ввод (Ctrl-C).
function M.get(config)
	local default = M.timestamp()
	if not (config and config.prompt_filename) then
		return default
	end

	local ok, input = pcall(vim.fn.input, { prompt = "filename: ", default = default })
	if not ok then
		return nil
	end
	if input == "" then
		return default
	end
	if not input:match("%.md$") then
		input = input .. ".md"
	end
	return input
end

return M
