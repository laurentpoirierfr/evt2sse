package main

import (
	"context"
	"flag"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/laurentpoirierfr/evt2sse/internal/relay"
	"github.com/laurentpoirierfr/evt2sse/internal/server"
)

const defaultChannel = "evt2sse"

func main() {
	var (
		addr    = flag.String("addr", ":8080", "adresse HTTP de l'écouteur")
		connStr = flag.String("db", "", "chaîne de connexion PostgreSQL")
		channel = flag.String("channel", defaultChannel, "canal PostgreSQL NOTIFY/LISTEN par défaut")
	)
	flag.Parse()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	stop := make(chan struct{})
	go func() {
		<-sig
		close(stop)
	}()

	r := relay.New(connString(*connStr), *channel)
	if err := startWithRetry(r, stop); err != nil {
		log.Printf("evt2sse: arrêt avant connexion à la base (%v)", err)
		return
	}
	defer r.Close()

	srv := server.New(r, *channel)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.Start(ctx)

	// Timeouts HTTP : WriteTimeout reste à 0 car le flux SSE est de longue
	// durée ; les coupures de client sont détectées par l'écriture (heartbeat).
	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		<-stop // déclenché par le signal d'arrêt
		cancel()
		shutdownCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("evt2sse prêt sur %s (canal: %s)", *addr, *channel)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// startWithRetry connecte le relais à PostgreSQL avec backoff exponentiel +
// jitter, sans abandonner tant que la base est indisponible (infrastructure
// parfois capricieuse) ou que le processus n'est pas stoppé.
func startWithRetry(r *relay.Relay, stop <-chan struct{}) error {
	for attempt := 0; ; attempt++ {
		if err := r.Start(); err == nil {
			return nil
		} else {
			delay := backoffDelay(attempt)
			log.Printf("evt2sse: base indisponible (%v) — nouvelle tentative dans %s", err, delay)
			select {
			case <-stop:
				return context.Canceled
			case <-time.After(delay):
			}
		}
	}
}

// backoffDelay : 1s, 2s, 4s, … borné à 30 s, jitter ±20 %.
func backoffDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	base := time.Second
	for i := 0; i < attempt && base < 30*time.Second; i++ {
		base *= 2
	}
	if base > 30*time.Second {
		base = 30 * time.Second
	}
	jitter := base / 5
	d := base - jitter + time.Duration(rand.Int64N(int64(2*jitter)+1))
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

func connString(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	for _, env := range []string{"PGURL", "DATABASE_URL"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
}
