// Command simulator est un outil de test pour evt2sse : il écoute le flux SSE
// et affiche les événements reçus sur la console, et peut émettre en parallèle
// des événements simulés via le package client.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/laurentpoirierfr/evt2sse/pkg/client"
)

const defaultPayload = `{"simulator":true,"seq":{{n}},"time":"{{ts}}"}`

type config struct {
	url      string
	channel  string
	payload  string
	send     bool
	interval time.Duration
	duration time.Duration
	count    int
	noColor  bool
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.url, "url", "http://localhost:8080", "URL du serveur evt2sse")
	flag.StringVar(&cfg.channel, "channel", "evt2sse", "canal par défaut")
	flag.StringVar(&cfg.payload, "payload", defaultPayload, "modèle de payload ({{n}} et {{ts}} remplacés)")
	flag.BoolVar(&cfg.send, "send", true, "émettre aussi des événements (false = écoute seule)")
	flag.DurationVar(&cfg.interval, "interval", 2*time.Second, "intervalle entre deux envois")
	flag.DurationVar(&cfg.duration, "duration", 0, "durée totale d'exécution (0 = jusqu'à Ctrl+C)")
	flag.IntVar(&cfg.count, "count", 0, "nombre d'envois (0 = illimité)")
	flag.BoolVar(&cfg.noColor, "no-color", false, "désactive les couleurs ANSI")
	flag.Parse()

	useColor = !cfg.noColor
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, red("simulator:"), err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		stop()
	}()

	if cfg.duration > 0 {
		ctx, stop = context.WithTimeout(ctx, cfg.duration)
		defer stop()
	}

	cli := client.New(cfg.url, client.WithDefaultChannel(cfg.channel))

	fmt.Printf("%s evt2sse simulator — %s\n", cyan("▶"), cfg.url)
	if st, err := cli.Status(ctx); err == nil {
		fmt.Printf("  %s serveur: canal=%s clients=%d db=%v\n",
			dim("ℹ"), st.Channel, st.Clients, st.Connected)
	} else {
		fmt.Printf("  %s statut indisponible: %v\n", yellow("⚠"), err)
	}

	stream := cli.Listen(ctx)
	defer stream.Close()

	var sent, received atomic.Int64

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-stream.Events():
				if !ok {
					return
				}
				received.Add(1)
				printEvent(evt, cfg.channel)
			case n, ok := <-stream.Errs():
				if ok && ctx.Err() == nil {
					fmt.Fprintf(os.Stderr, "%s %v\n", yellow("⚠"), n)
				}
			}
		}
	}()

	if cfg.send {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(cfg.interval)
			defer ticker.Stop()
			for i := 1; ; i++ {
				if cfg.count > 0 && i > cfg.count {
					fmt.Printf("%s émissions terminées (%d envoyés)\n", dim("ℹ"), cfg.count)
					return
				}
				if i > 1 {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
					}
				}
				payload := buildPayload(cfg.payload, i)
				if err := cli.Send(ctx, cfg.channel, payload); err != nil {
					fmt.Fprintf(os.Stderr, "%s [send] %v\n", red("✗"), err)
					continue
				}
				sent.Add(1)
				fmt.Printf("%s envoyé #%d → %s\n", green("→"), i, cfg.channel)
			}
		}()
	} else {
		fmt.Printf("%s écoute seule (activez l'envoi avec -send=true)\n", dim("ℹ"))
	}

	fmt.Printf("%s Ctrl+C pour arrêter\n", dim("ℹ"))

	// Laisse la connexion SSE s'établir avant le premier envoi, sinon le tout
	// premier événement peut partir avant l'enregistrement de l'auditeur.
	time.Sleep(600 * time.Millisecond)
	<-ctx.Done()

	fmt.Printf("%s arrêt...\n", dim("ℹ"))
	wg.Wait()
	fmt.Printf("%s %d envoyé(s), %d reçu(s)\n", cyan("■"), sent.Load(), received.Load())
	return nil
}

func printEvent(evt client.Event, defChannel string) {
	ch := evt.Channel
	if ch == "" {
		ch = defChannel
	}
	ts := evt.Time
	if ts.IsZero() {
		ts = time.Now()
	}
	id := strconv.FormatInt(evt.ID, 10)
	fmt.Printf("%s %s  [%s] %s\n",
		green("▸ reçu"), dim("#"+id), cyan(ch), evt.Payload)
}

// buildPayload remplace {{n}} (compteur) et {{ts}} (horodatage RFC3339Nano).
func buildPayload(tpl string, n int) string {
	s := strings.ReplaceAll(tpl, "{{n}}", strconv.Itoa(n))
	return strings.ReplaceAll(s, "{{ts}}", time.Now().UTC().Format(time.RFC3339Nano))
}

var useColor = true

func paint(code, s string) string {
	if !useColor {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func red(s string) string    { return paint("31", s) }
func green(s string) string  { return paint("32", s) }
func yellow(s string) string { return paint("33", s) }
func cyan(s string) string   { return paint("36", s) }
func dim(s string) string    { return paint("2", s) }
