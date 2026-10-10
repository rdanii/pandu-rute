// Package httpapi contains the HTTP handlers and routing for the Pandu Rute API.
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"pandu-rute/backend/internal/tripplan"
)

// DefaultPlanTimeout bounds a trip-plan request. It must stay below the
// server's WriteTimeout (10s in cmd/api) so an error response can still be
// written when planning runs out of time.
const DefaultPlanTimeout = 8 * time.Second

// Dependencies are the collaborators of the HTTP handlers. Zero values are
// safe: no Planner means trip plans fail with ROUTING_UNAVAILABLE.
type Dependencies struct {
	Planner     tripplan.Planner
	Logger      *slog.Logger
	Now         func() time.Time
	PlanTimeout time.Duration
}

// NewRouter returns the API's HTTP handler with no routing engine configured.
func NewRouter() http.Handler {
	return NewRouterWith(Dependencies{})
}

// NewRouterWith returns the API's HTTP handler with all routes registered.
func NewRouterWith(d Dependencies) http.Handler {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.PlanTimeout <= 0 {
		d.PlanTimeout = DefaultPlanTimeout
	}

	tp := &tripPlanHandler{
		service: tripplan.NewService(d.Planner),
		logger:  d.Logger,
		now:     d.Now,
		timeout: d.PlanTimeout,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.Handle("POST /v1/trip-plans", tp)
	return mux
}
