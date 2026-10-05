package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Stream is a Server-Sent Events response in progress. It owns the invariants
// every buildtall SSE handler otherwise hand-rolls:
//
//   - the ResponseWriter must implement http.Flusher end-to-end, which the
//     shared middleware chain guarantees (see Handler and the pinning test
//     TestHandler_SupportsStreaming);
//   - the server's write deadline must be cleared per-request, because a
//     long-lived stream outlasts any sane WriteTimeout;
//   - X-Accel-Buffering must be "no", because nginx buffers proxied responses
//     by default and would hold events (and the headers themselves) upstream
//     indefinitely;
//   - every write flushes immediately, and write errors surface to the caller
//     so a dead client ends the handler.
//
// A Stream is not safe for concurrent writers: the handler's single select
// loop must own all Send/Heartbeat calls, multiplexing its data sources and
// heartbeat ticker into that one goroutine.
type Stream struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewStream prepares w for Server-Sent Events: it asserts flushability,
// clears the write deadline, sets the event-stream headers, and commits the
// 200 status. On failure it writes the error response itself and returns the
// error, so a handler simply returns.
func NewStream(w http.ResponseWriter) (*Stream, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return nil, fmt.Errorf("response writer does not implement http.Flusher")
	}

	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return nil, fmt.Errorf("clearing write deadline: %w", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	return &Stream{w: w, flusher: flusher}, nil
}

// Send writes one named event with data marshaled as JSON and flushes it. A
// write error means the client is gone; the handler should return.
func (s *Stream) Send(event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling %s event: %w", event, err)
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return fmt.Errorf("writing %s event: %w", event, err)
	}
	s.flusher.Flush()
	return nil
}

// Heartbeat writes an SSE comment line and flushes it, keeping an idle stream
// alive through proxies that cut silent upstreams. A write error means the
// client is gone; the handler should return.
func (s *Stream) Heartbeat() error {
	if _, err := fmt.Fprint(s.w, ": keepalive\n\n"); err != nil {
		return fmt.Errorf("writing heartbeat: %w", err)
	}
	s.flusher.Flush()
	return nil
}
