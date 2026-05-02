package lsp

import (
	"database/sql"
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp/server"
)

type Server struct {
	conn     *sql.DB
	shutdown atomic.Bool
	mu       sync.Mutex
}

// mdxHandler оборачивает стандартный glsp protocol.Handler,
// перехватывая кастомные методы (mdx/*) перед делегированием.
type mdxHandler struct {
	base   *protocol.Handler
	server *Server
}

func (h *mdxHandler) Handle(ctx *glsp.Context) (any, bool, bool, error) {
	switch ctx.Method {
	case "mdx/listNotes":
		result, err := h.server.onListNotes(ctx)
		return result, true, true, err
	case "mdx/searchByTags":
		var p struct {
			Include []string `json:"include"`
			Exclude []string `json:"exclude"`
		}
		if err := json.Unmarshal(ctx.Params, &p); err != nil {
			return nil, true, true, err
		}
		result, err := h.server.onSearchByTags(ctx, p.Include, p.Exclude)
		return result, true, true, err
	case "mdx/query":
		var p struct {
			SQL  string `json:"sql"`
			Args []any  `json:"args"`
		}
		if err := json.Unmarshal(ctx.Params, &p); err != nil {
			return nil, true, true, err
		}
		result, err := h.server.onQuery(ctx, p.SQL, p.Args)
		return result, true, true, err
	default:
		return h.base.Handle(ctx)
	}
}

func New(conn *sql.DB) *server.Server {
	s := &Server{conn: conn}
	base := &protocol.Handler{
		Initialize:          s.onInitialize,
		Initialized:         s.onInitialized,
		Shutdown:            s.onShutdown,
		Exit:                s.onExit,
		TextDocumentDidOpen: s.onDidOpen,
		TextDocumentDidSave: s.onDidSave,
	}
	return server.NewServer(&mdxHandler{base: base, server: s}, "mdx", false)
}
