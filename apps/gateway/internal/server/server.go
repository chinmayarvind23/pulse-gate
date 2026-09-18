package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/yourname/pulsegate/apps/gateway/internal/admission"
	"github.com/yourname/pulsegate/apps/gateway/internal/config"
	"github.com/yourname/pulsegate/apps/gateway/internal/model"
	"github.com/yourname/pulsegate/apps/gateway/internal/signature"
	"github.com/yourname/pulsegate/apps/gateway/internal/telemetry"
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
	client := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, PoolSize: 128, MinIdleConns: 16})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	reg := prometheus.NewRegistry()
	app := &App{cfg: cfg, redis: client, admitter: admission.New(client, cfg.Stream, cfg.IdempotencyTTL), metrics: telemetry.NewMetrics(reg), mux: http.NewServeMux()}
	app.routes(reg)
	return app, nil
}

func (a *App) routes(reg *prometheus.Registry) {
	a.mux.HandleFunc("POST /v1/events", a.handleEvent)
	a.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	a.mux.HandleFunc("GET /readyz", a.handleReady)
	a.mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
}

func (a *App) Handler() http.Handler { return a.mux }
func (a *App) Close() error          { return a.redis.Close() }

// handleEvent performs only work required to safely admit an event. It intentionally
// does not score risk or call other business services because those dependencies would
// couple acknowledgement latency to downstream health and trigger provider retries.
func (a *App) handleEvent(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	defer func() { a.metrics.Duration.Observe(time.Since(started).Seconds()) }()

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, a.cfg.MaxBodyBytes))
	if err != nil {
		a.respond(w, http.StatusRequestEntityTooLarge, "request body too large")
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

	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()
	result, err := a.admitter.Admit(ctx, event.EventID, body)
	if err != nil {
		a.metrics.AdmissionErrors.Inc()
		a.respond(w, http.StatusServiceUnavailable, "admission unavailable")
		return
	}
	if result.Duplicate {
		a.metrics.Duplicates.Inc()
		w.Header().Set("X-PulseGate-Duplicate", "true")
	}
	a.respond(w, http.StatusAccepted, "")
}

func (a *App) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 300*time.Millisecond)
	defer cancel()
	if err := a.redis.Ping(ctx).Err(); err != nil {
		a.respond(w, http.StatusServiceUnavailable, "redis unavailable")
		return
	}
	w.WriteHeader(http.StatusOK)
}

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
