local M = {}
local function detect_root(opts)
	local root_opt = opts.root
	if type(root_opt) == "function" then
		return root_opt()
	elseif type(root_opt) == "string" then
		return root_opt
	end

	local detected = vim.fs.root(0, { ".git", ".mdx", "stylua.toml" })
	return detected or vim.fn.getcwd()
end

local function filename_without_ext(path)
	return (vim.fs.basename(path):gsub("%.md$", ""))
end
local function entry_title(path, mode)
	if mode == "filename" then
		return filename_without_ext(path)
	end
	-- "auto" и "title" сейчас совпадают: H1 если есть, иначе имя файла.
	return require("mdx.notes").title(path)
end

function M.open(opts, on_select)
	opts = opts or {}

	local ok, pickers = pcall(require, "telescope.pickers")
	if not ok then
		vim.notify("mdx: telescope.nvim is required for picker", vim.log.levels.ERROR)
		return
	end
	local finders = require("telescope.finders")
	local conf = require("telescope.config").values
	local actions = require("telescope.actions")
	local action_state = require("telescope.actions.state")

	local root = detect_root(opts)
	local paths = require("mdx.notes").collect(root, { exclude = opts.exclude })

	if #paths == 0 then
		vim.notify("mdx: no notes found in " .. root, vim.log.levels.INFO)
		return
	end

	local entries = {}
	for _, path in ipairs(paths) do
		table.insert(entries, {
			path = path,
			title = entry_title(path, opts.link_text or "auto"),
		})
	end

	pickers
		.new({}, {
			prompt_title = "mdx: insert link",
			finder = finders.new_table({
				results = entries,
				entry_maker = function(entry)
					local rel = vim.fs.relpath(root, entry.path) or entry.path
					return {
						value = entry,
						display = string.format("%s (%s)", entry.title, rel),
						ordinal = entry.title .. " " .. rel,
					}
				end,
			}),
			sorter = conf.generic_sorter({}),
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
