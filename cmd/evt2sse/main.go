package main

import (
	"context"
	"flag"
	"log"
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

	r := relay.New(connString(*connStr), *channel)
	if err := r.Start(); err != nil {
		log.Fatalf("impossible de démarrer le relais: %v", err)
	}
	defer r.Close()

	srv := server.New(r, *channel)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.Start(ctx)

	httpSrv := &http.Server{
		Addr:    *addr,
		Handler: srv.Handler(),
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		cancel()
		shutdownCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("evt2sse prêt sur %s (canal: %s)", *addr, *channel)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
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
