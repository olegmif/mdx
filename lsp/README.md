# mdx (Go binary)

Binary part of the `mdx` project: a CLI today, an LSP server later. Indexes
markdown files (frontmatter, outgoing links, tags) into a SQLite database.

For the project rationale and architecture see `../docs/strategy.md`. For the
M0 milestone breakdown see `../docs/m0.md`.

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
internal/cli/     scan pipeline (Run + per-file orchestration)
internal/db/      SQLite open, migrations, queries
internal/parse/   frontmatter, links, tags parsers
internal/scan/    filesystem walker, stat, content hash
```
