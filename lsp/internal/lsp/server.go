package lsp

import (
	"database/sql"

	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp/server"
)

func New(conn *sql.DB) *server.Server {
	handler := protocol.Handler{}
	return server.NewServer(&handler, "mdx", false)
}
