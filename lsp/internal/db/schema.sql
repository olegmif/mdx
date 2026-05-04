CREATE TABLE schema_version (
  version INTEGER PRIMARY KEY
);

CREATE TABLE notes (
  path          TEXT PRIMARY KEY,
  inode         INTEGER,
  mtime         INTEGER NOT NULL,
  size          INTEGER NOT NULL,
  content_hash  TEXT NOT NULL,
  frontmatter   TEXT,
  title         TEXT
);

CREATE INDEX idx_notes_inode        ON notes(inode);
CREATE INDEX idx_notes_content_hash ON notes(content_hash);

CREATE TABLE links (
  source_path  TEXT NOT NULL,
  target_path  TEXT NOT NULL,
  raw_target   TEXT NOT NULL,
  line         INTEGER NOT NULL,
  col          INTEGER NOT NULL,
  FOREIGN KEY (source_path) REFERENCES notes(path) ON DELETE CASCADE
);

CREATE INDEX idx_links_source ON links(source_path);
CREATE INDEX idx_links_target ON links(target_path);

CREATE TABLE tags (
  path  TEXT NOT NULL,
  tag   TEXT NOT NULL,
  PRIMARY KEY (path, tag),
  FOREIGN KEY (path) REFERENCES notes(path) ON DELETE CASCADE
);

CREATE INDEX idx_tags_tag ON tags(tag);

CREATE TABLE embeddings (
  path          TEXT NOT NULL,
  model         TEXT NOT NULL,
  content_hash  TEXT NOT NULL,
  embedded_at   INTEGER NOT NULL,
  PRIMARY KEY (path, model),
  FOREIGN KEY (path) REFERENCES notes(path) ON DELETE CASCADE
);

CREATE INDEX idx_embeddings_model ON embeddings(model);
