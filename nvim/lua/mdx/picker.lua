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

function M.tag_search(opts, on_select) -- on_select(note_entry)
	vim.notify("mdx: tag_search not implemented yet", vim.log.levels.INFO)
end

return M
