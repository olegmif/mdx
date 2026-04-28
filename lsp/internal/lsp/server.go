package lsp

import (
	"database/sql"
	"sync/atomic"

	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp/server"
)

type Server struct {
	conn     *sql.DB
	shutdown atomic.Bool
}

func New(conn *sql.DB) *server.Server {
	s := &Server{conn: conn}
	handler := protocol.Handler{
		Initialize:  s.onInitialize,
		Initialized: s.onInitialized,
		Shutdown:    s.onShutdown,
		Exit:        s.onExit,
	}
	return server.NewServer(&handler, "mdx", false)
}
