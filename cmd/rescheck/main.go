// Command rescheck exerce la résilience du client et du serveur evt2sse :
// écoute avec auto-reconnexion, reprise à la reconnexion, et affichage des
// événements reçus. Utilitaire temporaire pour les tests de coupure.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/laurentpoirierfr/evt2sse/pkg/client"
)

func main() {
	c := client.New("http://localhost:8080",
		client.WithReconnectMinDelay(300*time.Millisecond),
		client.WithReconnectMaxDelay(3*time.Second),
		client.WithSendRetries(6),
	)
	ctx := context.Background()
	s := c.Listen(ctx)
	defer s.Close()

	go func() {
		for err := range s.Errs() {
			fmt.Println("ERR", err)
		}
	}()

	t0 := time.Now()
	if err := c.Send(ctx, "evt2sse", "listener-ready"); err != nil {
		fmt.Println("SEND-FAIL", err)
	}
	_ = os.Getpid()

	for ev := range s.Events() {
		fmt.Printf("EVT id=%d channel=%s payload=%s at=%s\n",
			ev.ID, ev.Channel, ev.Payload, time.Since(t0).Round(time.Millisecond))
	}
}