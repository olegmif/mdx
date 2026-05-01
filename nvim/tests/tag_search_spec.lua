vim.opt.rtp:prepend(vim.fn.expand("~/projects/mdx/nvim"))

local tags = require("mdx.tags")

describe("mdx.tags.parse_query", function()
	local cases = {
		-- базовые: пустота, пробелы, незавершённый токен
		{ name = "nil prompt → пустые массивы", prompt = nil, include = {}, exclude = {} },
		{ name = "пустая строка → пустые массивы", prompt = "", include = {}, exclude = {} },
		{ name = "только пробелы → пустые массивы", prompt = "   ", include = {}, exclude = {} },
		{ name = "одиночный токен без пробела не применяется", prompt = "mdx", include = {}, exclude = {} },

		-- обычный include через trailing space
		{ name = "одиночный токен с trailing space", prompt = "mdx ", include = { "mdx" }, exclude = {} },
		{ name = "два токена → AND-include", prompt = "mdx go ", include = { "mdx", "go" }, exclude = {} },

		-- префиксы + и -
		{ name = "+token синонимичен голому токену", prompt = "mdx +go ", include = { "mdx", "go" }, exclude = {} },
		{ name = "-token уходит в exclude", prompt = "mdx -draft ", include = { "mdx" }, exclude = { "draft" } },
		{ name = "только exclude — допустимо", prompt = "-draft ", include = {}, exclude = { "draft" } },

		-- whitespace-схлопывание
		{ name = "множественные пробелы схлопываются", prompt = "  mdx   go  ", include = { "mdx", "go" }, exclude = {} },

		-- одиночные + и - после расщепления
		{ name = "голый '+' после префикса отбрасывается", prompt = "mdx + ", include = { "mdx" }, exclude = {} },
		{ name = "голый '-' после префикса отбрасывается", prompt = "mdx - ", include = { "mdx" }, exclude = {} },

		-- wildcards: *
		{ name = "wildcard * сохраняется в токене", prompt = "mdx* ", include = { "mdx*" }, exclude = {} },
		{ name = "exact + wildcard в одном prompt'е", prompt = "go mdx* ", include = { "go", "mdx*" }, exclude = {} },
		{ name = "+ перед wildcard — синоним голого", prompt = "+*tmp* ", include = { "*tmp*" }, exclude = {} },
		{ name = "wildcard в exclude", prompt = "-mdx* ", include = {}, exclude = { "mdx*" } },
		{ name = "одиночный * — валидный токен", prompt = "* ", include = { "*" }, exclude = {} },

		-- правило trailing-space действует и для wildcard-токенов
		{ name = "незавершённый -* не применяется", prompt = "-*", include = {}, exclude = {} },
		{ name = "незавершённый mdx* не применяется", prompt = "mdx*", include = {}, exclude = {} },

		-- wildcards: ? и [...]
		{ name = "single-char wildcard ? пробрасывается", prompt = "t?g ", include = { "t?g" }, exclude = {} },
		{ name = "character class пробрасывается", prompt = "tag[12] ", include = { "tag[12]" }, exclude = {} },
	}

	for _, tc in ipairs(cases) do
		it(tc.name, function()
			local got_inc, got_exc = tags.parse_query(tc.prompt)
			assert.are.same(tc.include, got_inc)
			assert.are.same(tc.exclude, got_exc)
		end)
	end
end)
