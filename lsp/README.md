# mdx (Go binary)

Binary part of the `mdx` project: a CLI for batch indexing and an LSP server
that re-indexes files on open and save. Indexes markdown files (frontmatter,
outgoing links, tags) into a SQLite database.

For the project rationale and architecture see `../docs/strategy.md`. For
milestone breakdowns see `../docs/m0.md` and `../docs/m1.md`. For the
embeddings/semantic-search subsystem see `../docs/embeddings.md`,
`../docs/M0_embeddings.md` and `../docs/M1_embeddings.md`.

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
- `--ignore PATH` — override ignore-file location (default: `$XDG_CONFIG_HOME/mdx/ignore`,
  fallback `~/.config/mdx/ignore`; can also be set via `MDX_IGNORE`). See
  [Ignore file](#ignore-file) below.
- `--exclude NAME` — additional directory base names to skip (in addition to
  the built-in list: `.git`, `node_modules`, `.venv`, `target`, `.cache`,
  `dist`, `build`, `vendor`). Repeatable or comma-separated.
- `-q`, `--quiet` — suppress the summary line.

The summary line printed at the end:

```
scanned: N, errors: M, elapsed: T
```

Per-file errors are written to stderr but never abort the run.

### Ignore file

Plain text, one path per line. Blank lines and lines starting with `#` are
ignored. Each path is either absolute (`/var/log`) or starts with `~`/`~/`
(expanded against the current user's home directory). Relative paths are
rejected with a warning to stderr.

Match semantics: prefix on the normalized absolute path. A file is skipped
when its path equals an entry exactly or is a strict subtree (path begins
with `<entry>` followed by the path separator). String-prefix collisions
(`/foo/barbaz` against entry `/foo/bar`) are not matched.

Effect on `mdx scan`: the walker does not descend into ignored directories
and does not feed ignored files into the indexing pipeline.

Effect on `mdx lsp`: `textDocument/didOpen` and `textDocument/didSave` for
files under an ignored prefix return early — no upsert, no diagnostics
published. The check happens after URI→path conversion, so opening such a
file in the editor is silent. The ignore file is read once at server
startup; restart `mdx lsp` to pick up edits.

Already-indexed records under ignored prefixes are **not** removed by
`scan` or by `lsp`; run `mdx gc` to drop them (see
[Garbage-collect orphan rows](#garbage-collect-orphan-rows)).

Example `~/.config/mdx/ignore`:

```
# transient state, never indexed
~/.local/state
/var/log
```

## Garbage-collect orphan rows

```
./bin/mdx gc
```

Iterates over every `notes` row in the database. For each row it checks
two conditions:

- the file referenced by `path` no longer exists on disk;
- the path falls under a prefix in `~/.config/mdx/ignore`.

Rows that meet either condition are deleted. Foreign-key `ON DELETE
CASCADE` drops the associated `links` (where the deleted note is the
source), `tags` and `embeddings` rows. Incoming links — rows in `links`
whose `target_path` points at the deleted note — are kept; they
correctly represent broken links in surviving notes.

`gc` takes no path arguments and has no notion of "scan roots" or
excluded directory names: a file that exists on disk and is not under an
ignore prefix is kept, regardless of where it lives. Stat errors other
than "file not found" (typically permission issues) are reported to
stderr and the row is preserved.

### Qdrant cleanup phase

After the SQLite phase, `gc` attempts to bring the Qdrant collection in
line with the surviving `notes` rows. The phase is best-effort:

- It runs only when an embedding config is loadable (resolved by the
  same `--embedding-config` / `MDX_EMBEDDING_CONFIG` / XDG /
  `~/.config/mdx/embedding.yaml` chain as `mdx embed`). A missing file
  is **not** an error — `gc` skips this phase silently and the summary
  ends with `qdrant: skipped`. A parse/validation error logs a warning
  to stderr and the phase is also skipped.
- It scrolls every point in the collection and deletes points whose
  `payload.path` is missing from `notes` after the SQLite phase. Points
  with no `path` in payload are treated as orphans and deleted too.
- Each `mdx gc` run is a full sync, not a delta from the previous run:
  any point left over from earlier failed runs (or inserted by some
  other path) is caught and removed by the next clean run.
- Any Qdrant failure (server unreachable, collection missing, HTTP
  non-2xx) is logged to stderr and the summary ends with
  `qdrant: error`. The exit code remains 0 — the SQLite phase is the
  primary contract of `gc` and must not be blocked by Qdrant
  availability.

Flags: `--db`, `--ignore`, `--embedding-config`, `-q`.

The summary line:

```
removed: N, kept: M, qdrant_removed: K, qdrant_kept: L, elapsed: T
```

When the Qdrant phase did not run, the `qdrant_removed`/`qdrant_kept`
pair is replaced with `qdrant: skipped` (no config) or `qdrant: error`
(config present but the phase failed).

## Compute embeddings (semantic search)

```
./bin/mdx embed
```

For every model in the embedding config and every note in the database,
this computes an embedding via an external HTTP API and upserts it into
Qdrant as a named vector. Re-runs are idempotent: a note with an
unchanged `content_hash` is skipped; a note whose `content_hash` differs
from the recorded one is re-embedded.

Preconditions:

1. `mdx scan` has been run, so the `notes` table is populated.
2. A Qdrant instance is reachable (default `http://127.0.0.1:6333`).
3. An embedding server speaking one of the supported protocols is
   reachable (default `http://127.0.0.1:8888`). Supported
   `endpoint_kind` values: `openai`, `llama-cpp`, `tei`.

### Minimal config

`~/.config/mdx/embedding.yaml`:

```yaml
qdrant_url: http://127.0.0.1:6333
collection: mdx

models:
  - name: qwen3-embedding-4b
    endpoint: http://127.0.0.1:8888/v1/embeddings
    endpoint_kind: openai
    api_model_name: Qwen/Qwen3-Embedding-4B
    dim: 2560
    distance: cosine
    query_prefix: "Instruct: Given a web search query, retrieve relevant passages that answer the query\nQuery: "
    document_prefix: ""
    batch_size: 16
    default_for_search: true
```

Multiple `models` entries become multiple named vectors in the same
collection. With more than one model, exactly one must be marked
`default_for_search: true`. Only `distance: cosine` is supported on M0.

The config path is resolved as: `--embedding-config` flag →
`MDX_EMBEDDING_CONFIG` env → `$XDG_CONFIG_HOME/mdx/embedding.yaml` →
`~/.config/mdx/embedding.yaml`. A missing file is a hard error: `mdx
embed` does nothing useful without it.

### Flags

- `--db PATH` — override database location (shared with `scan`/`gc`/`lsp`).
- `--embedding-config PATH` — override embedding config location.
- `--model NAME` — limit the run to a single model from the config.
  The Qdrant collection schema still reflects all configured models.
- `--all` — ignore the `embeddings` table and recompute every note.
  Useful after changing `query_prefix`/`document_prefix`/`api_model_name`
  without renaming the model.
- `-q`, `--quiet` — suppress the summary line.

The summary line:

```
embedded: N, skipped: M, failed: K, elapsed: T
```

Per-batch errors (file unreadable, embedding API non-2xx, Qdrant
upsert failure, SQLite record failure) are written to stderr and
counted in `failed`; they do not abort the run.

### Limitations

- Removing a note from disk does not remove its point from Qdrant. M0
  has no garbage-collection path for the vector store; extending
  `mdx gc` is tracked in `../docs/embeddings.md` (open questions).
  Until then, a stale point can be removed manually via the Qdrant
  API or by recreating the collection.
- A note longer than the model's context window is not chunked and is
  passed verbatim; truncation is the model's responsibility.

## Search the corpus

```
./bin/mdx search <query>...
```

Embeds the query with the same model that produced the indexed vectors
(applying the model's `query_prefix`), runs k-NN against Qdrant, and
prints matching note paths sorted by descending score. Multiple
positional arguments are joined with a single space, so quoting is
optional:

```
./bin/mdx search qdrant configuration
./bin/mdx search "qdrant configuration"
./bin/mdx search --format json --limit 3 "embeddings strategy" | jq '.[0]'
```

Preconditions:

1. `mdx embed` has been run at least once for the chosen model, so the
   Qdrant collection exists and contains points. Searching against an
   absent collection returns a clear HTTP error from Qdrant.
2. The same Qdrant instance and embedding server used by `mdx embed`
   are reachable. `mdx search` does not touch SQLite.

### Flags

- `--db PATH` — inherited from `scan`/`embed`; ignored by `search`
  (kept for flag uniformity across subcommands).
- `--embedding-config PATH` — override embedding config location.
- `--model NAME` — pick a specific model from the config. When omitted,
  the model marked `default_for_search: true` is used; if there is only
  one model in the config, that one is used regardless of the flag.
  A config with multiple models and no `default_for_search` is rejected
  by `LoadEmbedding` before search runs, so this branch is unreachable
  with a valid config.
- `--limit N` — maximum number of results (default `20`).
- `--format text|json` — output format (default `text`). `text` writes
  one path per line and nothing else, suited for `xargs`/`fzf`/`vim`
  pipelines. `json` writes a single array of `{path, score, title}`
  objects; `title` is omitted for notes without a `title:` field in
  frontmatter. An unknown value is rejected with an error.

An empty result is an empty stdout for `text` or `[]` for `json`; this
is not an error, so pipelines stay well-behaved.

## Run LSP server

```
./bin/mdx lsp
```

Speaks LSP over stdin/stdout. Intended to be launched by an editor; not
useful to run directly.

Flags:

- `--db PATH` — override database location (shared with `scan`).
- `--log PATH` — override log file location.
- `--ignore PATH` — override ignore-file location (default: `$XDG_CONFIG_HOME/mdx/ignore`,
  fallback `~/.config/mdx/ignore`; can also be set via `MDX_IGNORE`). Files
  under an ignored prefix are skipped on `didOpen`/`didSave`.

Default log path:

1. `$XDG_STATE_HOME/mdx/lsp.log` if set,
2. otherwise `$HOME/.local/state/mdx/lsp.log`.

Set `MDX_LOG=debug` to raise the log level from `INFO` to `DEBUG`.

The server reacts to `textDocument/didOpen` and `textDocument/didSave`:
each event re-indexes the file (writing to the same SQLite database that
`mdx scan` uses) and publishes diagnostics for broken links — links whose
target file does not exist on disk. Files under a prefix listed in the
ignore file are skipped silently. The server does not react to
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

Schema (version 2) — four tables: `notes`, `links`, `tags`, `embeddings`.
See `internal/db/schema.sql`. The `embeddings` table records per-note
per-model `(content_hash, embedded_at)` and is consulted by `mdx embed`
to skip work; the actual vectors live in Qdrant.

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
internal/cli/     scan / gc / lsp / embed / search command runners
internal/config/  user-level config: ignore file, embedding.yaml
internal/db/      SQLite open, migrations, queries (incl. embeddings table)
internal/embed/   embedding API client, Qdrant client, point id (UUID v5)
internal/index/   per-file indexing (used by scan and by LSP handlers)
internal/lsp/     LSP server: handlers, diagnostics, URI helpers
internal/parse/   frontmatter, links, tags parsers
internal/scan/    filesystem walker, stat, content hash
```
