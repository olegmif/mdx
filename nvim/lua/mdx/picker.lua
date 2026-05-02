local M = {}

function M.open(opts, on_select)
	local ok, pickers = pcall(require, "telescope.pickers")
	if not ok then
		vim.notify("mdx: telescope.nvim is required", vim.log.levels.ERROR)
		return
	end
	local finders = require("telescope.finders")
	local conf = require("telescope.config").values
	local actions = require("telescope.actions")
	local action_state = require("telescope.actions.state")
	local insert = require("mdx.insert")

	local entries = require("mdx.notes").list()
	if not entries then
		return -- notify уже сделан в notes.list
	end
	if #entries == 0 then
		vim.notify("mdx: no notes in index", vim.log.levels.INFO)
		return
	end

	pickers
		.new({}, {
			prompt_title = "mdx: insert link",
			finder = finders.new_table({
				results = entries,
				entry_maker = function(entry)
					local display_path = insert.to_display_path(entry.path)
					return {
						value = entry,
						-- path нужен telescope-previewer'у, чтобы он знал
						-- какой файл показывать в правом окне.
						path = entry.path,
						display = string.format("%s (%s)", entry.title, display_path),
						ordinal = entry.title .. " " .. entry.path,
					}
				end,
			}),
			sorter = conf.generic_sorter({}),
			previewer = conf.file_previewer({}),
			attach_mappings = function(prompt_bufnr, _)
				actions.select_default:replace(function()
					local selection = action_state.get_selected_entry()
					actions.close(prompt_bufnr)
					if selection and on_select then
						on_select(selection.value)
					end
				end)
				return true
			end,
		})
		:find()
end

-- Picker по списку доступных SQL-запросов. on_select(query_entry) получает
-- { name, path, sql, description }.
function M.sql_queries(queries, on_select)
	local ok, pickers = pcall(require, "telescope.pickers")
	if not ok then
		vim.notify("mdx: telescope.nvim is required", vim.log.levels.ERROR)
		return
	end
	local finders = require("telescope.finders")
	local conf = require("telescope.config").values
	local actions = require("telescope.actions")
	local action_state = require("telescope.actions.state")

	pickers
		.new({}, {
			prompt_title = "mdx: sql queries",
			finder = finders.new_table({
				results = queries,
				entry_maker = function(entry)
					local display = entry.name
					if entry.description then
						display = string.format("%s — %s", entry.name, entry.description)
					end
					return {
						value = entry,
						path = entry.path,
						display = display,
						ordinal = entry.name .. " " .. (entry.description or ""),
					}
				end,
			}),
			sorter = conf.generic_sorter({}),
			previewer = conf.file_previewer({}),
			attach_mappings = function(prompt_bufnr, _)
				actions.select_default:replace(function()
					local selection = action_state.get_selected_entry()
					actions.close(prompt_bufnr)
					if selection and on_select then
						on_select(selection.value)
					end
				end)
				return true
			end,
		})
		:find()
end

-- Picker по результатам выполненного запроса. rows — массив map'ов,
-- каждый должен содержать поле "path". on_select(row) при Enter.
function M.sql_results(query_name, rows, on_select)
	local ok, pickers = pcall(require, "telescope.pickers")
	if not ok then
		vim.notify("mdx: telescope.nvim is required", vim.log.levels.ERROR)
		return
	end
	local finders = require("telescope.finders")
	local conf = require("telescope.config").values
	local actions = require("telescope.actions")
	local action_state = require("telescope.actions.state")
	local insert = require("mdx.insert")

	pickers
		.new({}, {
			prompt_title = string.format("mdx: %s (%d)", query_name, #rows),
			finder = finders.new_table({
				results = rows,
				entry_maker = function(row)
					local title = row.title and row.title ~= "" and row.title
						or vim.fn.fnamemodify(row.path, ":t:r")
					local display_path = insert.to_display_path(row.path)
					return {
						value = row,
						path = row.path,
						display = string.format("%s (%s)", title, display_path),
						ordinal = title .. " " .. row.path,
					}
				end,
			}),
			sorter = conf.generic_sorter({}),
			previewer = conf.file_previewer({}),
			attach_mappings = function(prompt_bufnr, _)
				actions.select_default:replace(function()
					local selection = action_state.get_selected_entry()
					actions.close(prompt_bufnr)
					if selection and on_select then
						on_select(selection.value)
					end
				end)
				return true
			end,
		})
		:find()
end

-- Picker по списку Lua-скриптов. on_select(script_entry) получает
-- { name, path, source, description }.
function M.pick_scripts(scripts, on_select)
	local ok, pickers = pcall(require, "telescope.pickers")
	if not ok then
		vim.notify("mdx: telescope.nvim is required", vim.log.levels.ERROR)
		return
	end
	local finders = require("telescope.finders")
	local conf = require("telescope.config").values
	local actions = require("telescope.actions")
	local action_state = require("telescope.actions.state")

	pickers
		.new({}, {
			prompt_title = "mdx: scripts",
			finder = finders.new_table({
				results = scripts,
				entry_maker = function(entry)
					local display = entry.name
					if entry.description then
						display = string.format("%s — %s", entry.name, entry.description)
					end
					return {
						value = entry,
						path = entry.path,
						display = display,
						ordinal = entry.name .. " " .. (entry.description or ""),
					}
				end,
			}),
			sorter = conf.generic_sorter({}),
			previewer = conf.file_previewer({}),
			attach_mappings = function(prompt_bufnr, _)
				actions.select_default:replace(function()
					local selection = action_state.get_selected_entry()
					actions.close(prompt_bufnr)
					if selection and on_select then
						on_select(selection.value)
					end
				end)
				return true
			end,
		})
		:find()
end

-- Picker для отображения списка строк (без действия на Enter).
-- Используется :MdxQuery для показа результата работы скрипта.
function M.pick_lines(title, lines)
	local ok, pickers = pcall(require, "telescope.pickers")
	if not ok then
		vim.notify("mdx: telescope.nvim is required", vim.log.levels.ERROR)
		return
	end
	local finders = require("telescope.finders")
	local sorters = require("telescope.sorters")
	local actions = require("telescope.actions")

	pickers
		.new({}, {
			prompt_title = string.format("mdx: %s", title),
			finder = finders.new_table({
				results = lines,
				entry_maker = function(line)
					return {
						value = line,
						display = line,
						ordinal = line,
					}
				end,
			}),
			sorter = sorters.get_generic_fuzzy_sorter({}),
			attach_mappings = function(prompt_bufnr, _)
				actions.select_default:replace(function()
					actions.close(prompt_bufnr)
				end)
				return true
			end,
		})
		:find()
end

function M.tag_search(opts, on_select) -- on_select(note_entry)
	local ok, pickers = pcall(require, "telescope.pickers")
	if not ok then
		vim.notify("mdx: telescope.nvim is required", vim.log.levels.ERROR)
		return
	end
	local finders = require("telescope.finders")
	local sorters = require("telescope.sorters")
	local conf = require("telescope.config").values
	local actions = require("telescope.actions")
	local action_state = require("telescope.actions.state")
	local tags = require("mdx.tags")
	local insert = require("mdx.insert")

	pickers
		.new({}, {
			prompt_title = "mdx: tag search",
			finder = finders.new_dynamic({
				fn = function(prompt)
					local include, exclude = tags.parse_query(prompt)
					return tags.search(include, exclude) or {}
				end,
				entry_maker = function(entry)
					local display_path = insert.to_display_path(entry.path)
					return {
						value = entry,
						path = entry.path,
						display = string.format("%s (%s)", entry.title, display_path),
						ordinal = entry.title .. " " .. entry.path,
					}
				end,
			}),
			-- No-op sorter: фильтрация и порядок задаются сервером в `tags.search`,
			-- telescope не должен поверх ещё раз fuzzy-матчить prompt против
			-- ordinal'а — это даёт двойную фильтрацию и ложные совпадения по пути.
			sorter = sorters.empty(),
			previewer = conf.file_previewer({}),
			attach_mappings = function(prompt_bufnr, _)
				actions.select_default:replace(function()
					local selection = action_state.get_selected_entry()
					actions.close(prompt_bufnr)
					if selection and on_select then
						on_select(selection.value)
					end
				end)
				return true
			end,
		})
		:find()
end

return M
