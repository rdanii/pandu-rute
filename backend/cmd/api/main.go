// Command api runs the Pandu Rute HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pandu-rute/backend/internal/attribution"
	"pandu-rute/backend/internal/httpapi"
	"pandu-rute/backend/internal/otp"
	"pandu-rute/backend/internal/stops"
	"pandu-rute/backend/internal/tripplan"
)

func main() {
	srv, planner, err := newServer(os.Getenv)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	addr := srv.Addr

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		// OTP is not contacted here; if it is down, trip plans return
		// ROUTING_UNAVAILABLE while the API keeps running.
		log.Printf("listening on %s (OTP endpoint %s)", addr, planner.Endpoint())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Print("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
}

// newServer builds the HTTP server from environment variables:
//
//	PORT             listen port (default 8080)
//	OTP_BASE_URL     OTP base URL (default otp.DefaultBaseURL)
//	GTFS_STOPS_PATH  path to a TransJakarta GTFS stops.txt file (no default)
//
// It fails only on invalid OTP configuration. An unavailable OTP or a
// missing or unreadable stops file never prevents startup.
func newServer(getenv func(string) string) (*http.Server, *otp.Client, error) {
	planner, err := otp.New(otp.Config{
		BaseURL:      envOr(getenv, "OTP_BASE_URL", otp.DefaultBaseURL),
		Attributions: otp.PilotAttributions(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("OTP_BASE_URL: %w", err)
	}
	srv := &http.Server{
		Addr: ":" + envOr(getenv, "PORT", "8080"),
		Handler: httpapi.NewRouterWith(httpapi.Dependencies{
			Planner:          planner,
			Stops:            loadStops(getenv("GTFS_STOPS_PATH")),
			StopAttributions: []tripplan.Attribution{attribution.TransJakartaGTFS()},
			Logger:           slog.Default(),
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return srv, planner, nil
}

// loadStops loads the stop-search index. Failure only disables stop search;
// log lines carry counts and the file hash, never row content.
func loadStops(path string) *stops.Index {
	if path == "" {
		log.Print("stop search disabled: GTFS_STOPS_PATH is not set")
		return nil
	}
	idx, err := stops.LoadFile(path, otp.DefaultNamespace)
	if err != nil {
		log.Printf("stop search disabled: %v", err)
		return nil
	}
	st := idx.Stats()
	log.Printf("stop search enabled: %d stops loaded from %d rows (skipped %v; unknown location_type %d, dangling parent %d, non-station parent %d), sha256 %s",
		st.Loaded, st.Rows, st.Skipped, st.UnknownLocationType, st.DanglingParent, st.NonStationParent, idx.Version())
	return idx
}

func envOr(getenv func(string) string, key, fallback string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return fallback
}
