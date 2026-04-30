vim.opt.rtp:prepend(vim.fn.expand("~/projects/mdx/nvim"))

local resolve = require("mdx.resolve")

describe("mdx.resolve.target_to_path", function()
	it("относительный путь резолвится относительно source_dir", function()
		assert.equals("/projects/notes/foo.md", resolve.target_to_path("foo.md", "/projects/notes"))
	end)

	it("относительный путь с .. нормализуется", function()
		assert.equals("/projects/foo.md", resolve.target_to_path("../foo.md", "/projects/notes"))
	end)

	it("./foo.md эквивалентен foo.md", function()
		assert.equals("/projects/notes/foo.md", resolve.target_to_path("./foo.md", "/projects/notes"))
	end)

	it("абсолютный путь возвращается нормализованным", function()
		assert.equals("/etc/hosts", resolve.target_to_path("/etc/hosts", "/anywhere"))
	end)

	it("абсолютный путь с .. нормализуется", function()
		assert.equals("/etc/hosts", resolve.target_to_path("/etc/foo/../hosts", "/anywhere"))
	end)

	it("одиночный ~ разворачивается в $HOME", function()
		assert.equals(vim.fn.expand("~"), resolve.target_to_path("~", "/anywhere"))
	end)

	it("~/foo разворачивается в $HOME/foo", function()
		assert.equals(vim.fn.expand("~") .. "/foo.md", resolve.target_to_path("~/foo.md", "/anywhere"))
	end)

	it("URL-схемы возвращают nil", function()
		assert.is_nil(resolve.target_to_path("https://example.com", "/anywhere"))
		assert.is_nil(resolve.target_to_path("http://example.com", "/anywhere"))
		assert.is_nil(resolve.target_to_path("mailto:foo@bar.com", "/anywhere"))
		assert.is_nil(resolve.target_to_path("ftp://example.com", "/anywhere"))
		assert.is_nil(resolve.target_to_path("tel:+1234567890", "/anywhere"))
	end)

	it("якорь #section возвращает nil", function()
		assert.is_nil(resolve.target_to_path("#section", "/anywhere"))
	end)

	it("пустой target возвращает nil", function()
		assert.is_nil(resolve.target_to_path("", "/anywhere"))
		assert.is_nil(resolve.target_to_path(nil, "/anywhere"))
	end)
end)
