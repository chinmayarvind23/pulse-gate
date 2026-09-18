package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/chinmayarvind23/pulse_gate/apps/gateway/internal/admission"
	"github.com/chinmayarvind23/pulse_gate/apps/gateway/internal/config"
	"github.com/chinmayarvind23/pulse_gate/apps/gateway/internal/model"
	"github.com/chinmayarvind23/pulse_gate/apps/gateway/internal/signature"
	"github.com/chinmayarvind23/pulse_gate/apps/gateway/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

type App struct {
	cfg      config.Config
	redis    *redis.Client
	admitter *admission.RedisAdmitter
	metrics  *telemetry.Metrics
	mux      *http.ServeMux
}

// New constructs dependencies once per process. The handler remains stateless with
// respect to idempotency because Redis owns shared coordination, which preserves
// correctness when Kubernetes adds gateway replicas.
func New(cfg config.Config) (*App, error) {
	client := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, PoolSize: 128, MaxActiveConns: 128, MinIdleConns: 16, ContextTimeoutEnabled: true, DialTimeout: time.Second, ReadTimeout: 500 * time.Millisecond, WriteTimeout: 500 * time.Millisecond, PoolTimeout: 500 * time.Millisecond, MaxRetries: -1})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, err
	}

	reg := prometheus.NewRegistry()
	app := &App{cfg: cfg, redis: client, admitter: admission.New(client, cfg.Stream, cfg.IdempotencyTTL), metrics: telemetry.NewMetrics(reg), mux: http.NewServeMux()}
	app.admitter.MaxQueue = cfg.MaxQueue
	app.routes(reg)
	return app, nil
}

// routes separates provider admission from operational health and telemetry.
func (a *App) routes(reg *prometheus.Registry) {
	a.mux.HandleFunc("GET /", a.handleHome)
	a.mux.HandleFunc("GET /v1/status", a.handleStatus)
	a.mux.HandleFunc("GET /v1/decisions", a.handleDecisions)
	a.mux.HandleFunc("POST /v1/events", a.handleEvent)
	a.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	a.mux.HandleFunc("GET /readyz", a.handleReady)
	a.mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
}

// Handler exposes the transport for the HTTP server and contract tests.
func (a *App) Handler() http.Handler { return a.mux }

// Close releases pooled connections after requests finish draining.
func (a *App) Close() error { return a.redis.Close() }

// handleEvent performs only work required to safely admit an event. It intentionally
// does not score risk or call other business services because those dependencies would
// couple acknowledgement latency to downstream health and trigger provider retries.
func (a *App) handleEvent(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	defer func() { a.metrics.Duration.Observe(time.Since(started).Seconds()) }()

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, a.cfg.MaxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			a.respond(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			a.respond(w, http.StatusBadRequest, "could not read body")
		}
		return
	}
	if !signature.Valid(a.cfg.HMACSecret, body, r.Header.Get("X-PulseGate-Signature")) {
		a.respond(w, http.StatusUnauthorized, "invalid signature")
		return
	}
	event, err := model.Decode(body)
	if err != nil {
		a.respond(w, http.StatusBadRequest, err.Error())
		return
	}

	// Canonical bytes keep parser differences and JSON formatting out of the worker
	// contract. Authentication above still binds the original request bytes.
	payload, err := json.Marshal(event)
	if err != nil {
		a.respond(w, http.StatusBadRequest, "invalid event")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
	defer cancel()
	result, err := a.admitter.Admit(ctx, event.EventID, payload)
	if err != nil {
		if errors.Is(err, admission.ErrConflict) {
			a.respond(w, http.StatusConflict, "event_id already used with different data")
			return
		}
		a.metrics.AdmissionErrors.Inc()
		w.Header().Set("Retry-After", "1")
		a.respond(w, http.StatusServiceUnavailable, "admission unavailable")
		return
	}
	if result.Duplicate {
		a.metrics.Duplicates.Inc()
		w.Header().Set("X-PulseGate-Duplicate", "true")
	}
	a.respond(w, http.StatusAccepted, "")
}

// handleReady stops routing traffic when shared admission state is unavailable.
func (a *App) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 300*time.Millisecond)
	defer cancel()
	if err := a.redis.Ping(ctx).Err(); err != nil {
		a.respond(w, http.StatusServiceUnavailable, "redis unavailable")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// respond keeps status labels bounded and error responses consistent.
func (a *App) respond(w http.ResponseWriter, code int, message string) {
	a.metrics.Requests.WithLabelValues(strconv.Itoa(code)).Inc()
	if message == "" {
		w.WriteHeader(code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
