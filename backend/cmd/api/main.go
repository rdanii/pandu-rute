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

	"pandu-rute/backend/internal/httpapi"
	"pandu-rute/backend/internal/otp"
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
//	PORT          listen port (default 8080)
//	OTP_BASE_URL  OTP base URL (default otp.DefaultBaseURL)
//
// It fails only on invalid configuration, never on OTP availability.
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
			Planner: planner,
			Logger:  slog.Default(),
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return srv, planner, nil
}

func envOr(getenv func(string) string, key, fallback string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return fallback
}
