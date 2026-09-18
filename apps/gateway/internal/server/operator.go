package server

import (
	"context"
	"embed"
	"encoding/json"
	"github.com/chinmayarvind23/pulse_gate/apps/gateway/internal/signature"
	"net/http"
	"time"
)

//go:embed web/index.html
var webFiles embed.FS

// handleHome serves the operator client without exposing the signing secret.
func (a *App) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
	body, _ := webFiles.ReadFile("web/index.html")
	_, _ = w.Write(body)
}

// authorizedRead binds read authentication to the requested path, protecting decision data.
func (a *App) authorizedRead(w http.ResponseWriter, r *http.Request) bool {
	// Inspection responses are tied to the current credentials and queue state.
	w.Header().Set("Cache-Control", "no-store")
	if !signature.Valid(a.cfg.HMACSecret, []byte(r.URL.Path), r.Header.Get("X-PulseGate-Signature")) {
		a.respond(w, http.StatusUnauthorized, "invalid signature")
		return false
	}
	return true
}

// handleStatus exposes bounded queue state only to an authenticated operator.
func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !a.authorizedRead(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 300*time.Millisecond)
	defer cancel()
	queue, err := a.redis.XLen(ctx, a.cfg.Stream).Result()
	if err != nil {
		a.respond(w, 503, "redis unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"outstanding": queue, "admission": "ready"})
}

// handleDecisions returns a bounded recent window; results remain a stream rather than a ledger.
func (a *App) handleDecisions(w http.ResponseWriter, r *http.Request) {
	if !a.authorizedRead(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 300*time.Millisecond)
	defer cancel()
	rows, err := a.redis.XRevRangeN(ctx, a.cfg.ResultStream, "+", "-", 50).Result()
	if err != nil {
		a.respond(w, 503, "decisions unavailable")
		return
	}
	decisions := make([]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		if raw, ok := row.Values["decision"].(string); ok && json.Valid([]byte(raw)) {
			decisions = append(decisions, json.RawMessage(raw))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(decisions)
}
