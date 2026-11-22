package server

import (
	"net/http"
	"os"
	"testing"
)

// TestMain makes every client request in this package use its own
// connection and close it with the response.
//
// A client that keeps connections alive can dial a second connection while
// the first is busy, give the request to the first when it frees up, and
// leave the second in its pool unused. The server holds that connection in
// the new state, and http.Server.Shutdown waits for a new connection for
// more than 5 seconds before it closes it. The servers under test get a 5
// second deadline to shut down, so one unused connection made Shutdown
// return "context deadline exceeded". With keep-alives off, the transport
// has no pool: a dialed connection serves the request that asked for it or
// is closed.
func TestMain(m *testing.M) {
	http.DefaultTransport = &http.Transport{DisableKeepAlives: true}
	os.Exit(m.Run())
}
