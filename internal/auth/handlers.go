package auth

import (
	"encoding/json"
	"net/http"

	"github.com/nbd-wtf/go-nostr"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/logger"
	"github.com/Buildtall-Systems/stigmergic.dev/web/templates"
)

type verifyRequest struct {
	Event *nostr.Event `json:"event"`
}

type verifyResponse struct {
	Redirect string `json:"redirect,omitempty"`
	Error    string `json:"error,omitempty"`
	OK       bool   `json:"ok"`
}

func LoginHandler(serverURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := templates.Login(serverURL).Render(r.Context(), w); err != nil {
			logger.Log.Error("failed to render login page", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

func VerifyHandler(sm *SessionManager, allowedPubkeys []string, serverURL string) http.HandlerFunc {
	verifyURL := serverURL + "/auth/verify"

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req verifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, verifyResponse{Error: "invalid request body"})
			return
		}

		if req.Event == nil {
			writeJSON(w, http.StatusBadRequest, verifyResponse{Error: "missing event"})
			return
		}

		if err := VerifyNIP98Event(req.Event, verifyURL, "POST"); err != nil {
			logger.Log.Warn("NIP-98 verification failed", "error", err, "pubkey", req.Event.PubKey)
			writeJSON(w, http.StatusUnauthorized, verifyResponse{Error: "authentication failed: " + err.Error()})
			return
		}

		if !IsPubkeyAllowed(req.Event.PubKey, allowedPubkeys) {
			logger.Log.Warn("pubkey not in allowlist", "pubkey", req.Event.PubKey)
			writeJSON(w, http.StatusForbidden, verifyResponse{Error: "pubkey not authorized"})
			return
		}

		sm.SetSessionCookie(w, r, req.Event.PubKey)
		logger.Log.Info("authentication successful", "pubkey", req.Event.PubKey)

		redirect := r.URL.Query().Get("redirect")
		if redirect == "" {
			redirect = "/"
		}

		writeJSON(w, http.StatusOK, verifyResponse{OK: true, Redirect: redirect})
	}
}

func LogoutHandler(sm *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		sm.ClearSessionCookie(w)
		http.Redirect(w, r, LoginPath, http.StatusSeeOther)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.Log.Error("failed to encode JSON response", "error", err)
	}
}
