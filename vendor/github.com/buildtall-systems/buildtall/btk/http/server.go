package http

import (
	"context"
	"net"
	"net/http"
	"time"
)

func RunServer(ctx context.Context, srv *http.Server) error {
	addr := srv.Addr
	if addr == "" {
		addr = ":http"
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return RunServerListener(ctx, srv, ln)
}

// RunServerListener serves an already-bound listener. A caller that must
// not announce readiness before the bind succeeds binds first, logs, then
// calls this, so no log line can claim an address the process does not
// hold.
func RunServerListener(ctx context.Context, srv *http.Server, ln net.Listener) error {
	errCh := make(chan error, 1)

	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
