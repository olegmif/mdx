# mdx (Go binary)

Binary part of the `mdx` project: a CLI for batch indexing and an LSP server
that re-indexes files on open and save. Indexes markdown files (frontmatter,
outgoing links, tags) into a SQLite database.

For the project rationale and architecture see `../docs/strategy.md`. For
milestone breakdowns see `../docs/m0.md` and `../docs/m1.md`.

## Build

```
make build
```

Produces `./bin/mdx`. Tested with Go 1.26+.

## Run

```
./bin/mdx scan [path...]
```

With no arguments, scans `$HOME`. Multiple roots can be passed.

Flags:

- `--db PATH` — override database location.
- `--exclude NAME` — additional directory base names to skip (in addition to
  the built-in list: `.git`, `node_modules`, `.venv`, `target`, `.cache`,
  `dist`, `build`, `vendor`). Repeatable or comma-separated.
- `-q`, `--quiet` — suppress the summary line.

The summary line printed at the end:

```
scanned: N, errors: M, elapsed: T
```

Per-file errors are written to stderr but never abort the run.

## Run LSP server

```
./bin/mdx lsp
```

Speaks LSP over stdin/stdout. Intended to be launched by an editor; not
useful to run directly.

Flags:

- `--db PATH` — override database location (shared with `scan`).
- `--log PATH` — override log file location.

Default log path:

1. `$XDG_STATE_HOME/mdx/lsp.log` if set,
2. otherwise `$HOME/.local/state/mdx/lsp.log`.

Set `MDX_LOG=debug` to raise the log level from `INFO` to `DEBUG`.

The server reacts to `textDocument/didOpen` and `textDocument/didSave`:
each event re-indexes the file (writing to the same SQLite database that
`mdx scan` uses) and publishes diagnostics for broken links — links whose
target file does not exist on disk. The server does not react to
`didChange`; the database stays in sync only at open and save. For bulk
indexing of files outside the editor, run `mdx scan`.

### Neovim configuration

Minimal `lspconfig` snippet (Neovim 0.11+ API):

```lua
vim.lsp.config.mdx = {
    cmd       = { "/absolute/path/to/mdx", "lsp" },
    filetypes = { "markdown" },
    root_dir  = "/",
}

vim.api.nvim_create_autocmd("FileType", {
    pattern = "markdown",
    callback = function()
        vim.lsp.enable("mdx")
    end,
})
```

`root_dir = "/"` is intentional: mdx is filesystem-wide and has no notion
of project root.

### Known limitation

When the editor closes the LSP connection by closing stdin without sending
an explicit `shutdown`/`exit` sequence, the trailing `server stopped` log
record may race with process exit and not be written. The server still
terminates correctly; only the log entry is lost. The explicit shutdown
path (used by neovim on `:q`) writes a clean `shutdown` record.

## Database

Default path:

1. `$MDX_DB` if set,
2. otherwise `$XDG_DATA_HOME/mdx/mdx.db`,
3. otherwise `$HOME/.local/share/mdx/mdx.db`.

The directory is created on first run. SQLite is opened in WAL mode, so the
file is safe to read with `sqlite3` while a scan is in progress.

Schema (version 1) — three tables: `notes`, `links`, `tags`. See
`internal/db/schema.sql`.

## Tests

```
make test
```

Runs unit tests for every package and an end-to-end test that scans a
fixture tree under `internal/cli/testdata/fixtures` and asserts the
resulting database contents.

## Layout

```
cmd/mdx/          entry point and cobra wiring
internal/cli/     scan and LSP command runners
internal/db/      SQLite open, migrations, queries
internal/index/   per-file indexing (used by scan and by LSP handlers)
internal/lsp/     LSP server: handlers, diagnostics, URI helpers
internal/parse/   frontmatter, links, tags parsers
internal/scan/    filesystem walker, stat, content hash
```
