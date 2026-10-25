// Command server runs the webhook endpoint.
//
//	WEBHOOK_SECRET=$(openssl rand -hex 32) go run ./cmd/server
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trisall7/sms-booking-agent/internal/agent"
	"github.com/trisall7/sms-booking-agent/internal/booking"
	"github.com/trisall7/sms-booking-agent/internal/sms"
)

func main() {
	secret := os.Getenv("WEBHOOK_SECRET")
	if len(secret) < 32 {
		log.Fatal("WEBHOOK_SECRET must be set to at least 32 characters (see .env.example)")
	}

	cal := booking.NewCalendar(booking.DemoSlots(time.Now(), time.Local))
	ag := agent.New(cal, getenv("BUSINESS_NAME", "Demo Studio"), time.Now)

	mux := http.NewServeMux()
	mux.Handle("/webhooks/sms", sms.NewHandler([]byte(secret), ag, sms.LogSender{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              ":" + getenv("PORT", "8080"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
