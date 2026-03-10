package nip98

import (
	"context"
	"encoding/json"
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

func RequireNIP98(publicBaseURL string, maxSkew time.Duration, allowedPubkeys []string) func(http.Handler) http.Handler {
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

			expectedURL := publicBaseURL + r.URL.Path
			if err := VerifyNIP98Event(event, expectedURL, r.Method, maxSkew); err != nil {
				writeAuthError(w, http.StatusUnauthorized, "NIP-98 verification failed: "+err.Error())
				return
			}

			if !IsPubkeyAllowed(event.PubKey, allowedPubkeys) {
				writeAuthError(w, http.StatusForbidden, "pubkey not allowed")
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
	_ = json.NewEncoder(w).Encode(errorResponse{Error: msg})
}
