package nip98

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type contextKey string

const pubkeyContextKey contextKey = "nip98_pubkey"

func PubkeyFromContext(ctx context.Context) string {
	v, ok := ctx.Value(pubkeyContextKey).(string)
	if !ok {
		return ""
	}
	return v
}

type errorResponse struct {
	Error string `json:"error"`
}

// RequireNIP98 enforces NIP-98 HTTP auth: the signed event must match the
// full request URL (including query string) and method, carry a matching
// payload hash whenever the request has a body, fall within maxSkew of the
// server clock, and be presented at most once per replay-cache TTL. Only the
// keys in allowedPubkeys are admitted; an empty allowlist admits nobody.
func RequireNIP98(publicBaseURL string, maxSkew time.Duration, allowedPubkeys []string) func(http.Handler) http.Handler {
	return requireNIP98(publicBaseURL, maxSkew, func(pubkey string) bool {
		return IsPubkeyAllowed(pubkey, allowedPubkeys)
	})
}

// RequireNIP98AnyKey enforces the same NIP-98 verification as RequireNIP98
// but admits any key that signs validly. It authenticates without
// authorizing: the handler behind it must scope what the key from
// PubkeyFromContext may see or do.
func RequireNIP98AnyKey(publicBaseURL string, maxSkew time.Duration) func(http.Handler) http.Handler {
	return requireNIP98(publicBaseURL, maxSkew, func(string) bool { return true })
}

func requireNIP98(publicBaseURL string, maxSkew time.Duration, admitted func(pubkey string) bool) func(http.Handler) http.Handler {
	replay := NewReplayCache(ReplayTTLFactor * maxSkew)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeAuthError(w, http.StatusUnauthorized, "missing Authorization header")
				return
			}

			event, err := ParseNIP98FromHeader(authHeader)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "invalid NIP-98 header: "+err.Error())
				return
			}

			expectedURL := publicBaseURL + r.URL.RequestURI()
			if err = VerifyNIP98Event(event, expectedURL, r.Method, maxSkew); err != nil {
				writeAuthError(w, http.StatusUnauthorized, "NIP-98 verification failed: "+err.Error())
				return
			}

			var body []byte
			if r.Body != nil {
				body, err = io.ReadAll(r.Body)
				if err != nil {
					writeAuthError(w, http.StatusBadRequest, "reading request body: "+err.Error())
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
			if err = VerifyPayloadTag(event, body); err != nil {
				writeAuthError(w, http.StatusUnauthorized, "NIP-98 verification failed: "+err.Error())
				return
			}

			if !admitted(event.PubKey) {
				writeAuthError(w, http.StatusForbidden, "pubkey not allowed")
				return
			}

			if replay.Seen(event.ID) {
				writeAuthError(w, http.StatusUnauthorized, "NIP-98 event already used")
				return
			}

			ctx := context.WithValue(r.Context(), pubkeyContextKey, event.PubKey)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeAuthError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(errorResponse{Error: msg}); err != nil {
		slog.Error("failed to encode auth error response", "error", err)
	}
}
