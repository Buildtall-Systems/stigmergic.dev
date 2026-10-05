package http

import (
	"log/slog"
	"net/http"

	btkmw "github.com/buildtall-systems/buildtall/btk/http/middleware"
)

// Handler applies the shared middleware chain. Request time bounds are the
// server's Read/Write/IdleTimeout, not a TimeoutHandler wrapper: the stdlib
// timeout writer implements no Flusher and no escape hatch, which structurally
// breaks SSE, while a server write deadline can be cleared per-request via
// ResponseController where streaming requires it.
func Handler(mux http.Handler, logger *slog.Logger) http.Handler {
	handler := mux
	handler = btkmw.Recovery(logger)(handler)
	handler = btkmw.LoggingMiddleware(logger)(handler)
	handler = btkmw.SecurityHeaders(handler)
	return handler
}
