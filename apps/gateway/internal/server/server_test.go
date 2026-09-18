package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"github.com/chinmayarvind23/hook-guard/apps/gateway/internal/config"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// TestOperatorBoundary protects operational data while keeping the static client public.
func TestOperatorBoundary(t *testing.T) {
	addr := os.Getenv("HOOKGUARD_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("Redis integration requires HOOKGUARD_TEST_REDIS_ADDR")
	}
	cfg := config.Config{RedisAddr: addr, HMACSecret: "test-secret", Stream: "test:operator:events", ResultStream: "test:operator:results", IdempotencyTTL: time.Minute, MaxBodyBytes: 65536, MaxQueue: 100}
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	for _, path := range []string{"/v1/status", "/v1/decisions"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, req)
		if w.Code != 401 {
			t.Fatalf("unauthenticated %s: %d", path, w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("unauthenticated %s response could be cached", path)
		}
		mac := hmac.New(sha256.New, []byte(cfg.HMACSecret))
		mac.Write([]byte(path))
		req = httptest.NewRequest("GET", path, nil)
		req.Header.Set("X-HookGuard-Signature", hex.EncodeToString(mac.Sum(nil)))
		w = httptest.NewRecorder()
		app.Handler().ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("authenticated %s: %d", path, w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("authenticated %s response could be cached", path)
		}
	}
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusOK || w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("operator page lacks security policy")
	}
	w = httptest.NewRecorder()
	app.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/missing", nil))
	if w.Code != 404 {
		t.Fatal("unknown path must be 404")
	}
}
