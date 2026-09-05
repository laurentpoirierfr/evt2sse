package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSend(t *testing.T) {
	var got sendRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/send" {
			t.Errorf("chemin inattendu: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("méthode inattendue: %s", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": got.Channel, "payload": got.Payload})
	}))
	defer srv.Close()

	c := New(srv.URL)
	ctx := context.Background()

	if err := c.Send(ctx, "chan1", "hello"); err != nil {
		t.Fatal(err)
	}
	if got.Channel != "chan1" || got.Payload != "hello" {
		t.Fatalf("requête inattendue: %+v", got)
	}

	if err := c.Send(ctx, "", "par défaut"); err != nil {
		t.Fatal(err)
	}
	if got.Channel != defaultChannel || got.Payload != "par défaut" {
		t.Fatalf("canal par défaut non appliqué: %+v", got)
	}
}

func TestSendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"error": "échec de notification"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	if err := c.Send(context.Background(), "c", "p"); err == nil || !strings.Contains(err.Error(), "échec") {
		t.Fatalf("erreur inattendue: %v", err)
	}
}

func TestSendJSON(t *testing.T) {
	var got sendRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	c := New(srv.URL)
	if err := c.SendJSON(context.Background(), "c", map[string]int{"n": 42}); err != nil {
		t.Fatal(err)
	}
	if got.Payload != `{"n":42}` {
		t.Fatalf("payload JSON inattendu: %s", got.Payload)
	}
}

func TestListen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)

		fmt.Fprint(w, "retry: 3000\n")
		fmt.Fprint(w, "event: ready\n")
		fmt.Fprint(w, "data: {\"channel\":\"evt2sse\",\"connected\":true}\n\n")
		fl.Flush()

		fmt.Fprint(w, "id: 1\n")
		fmt.Fprint(w, "data: {\"id\":1,\"channel\":\"chanA\",\"payload\":\"premier\",\"time\":\"2026-09-05T10:00:00Z\"}\n\n")
		fl.Flush()

		fmt.Fprint(w, "id: 2\n")
		fmt.Fprint(w, "data: {\"id\":2,\"channel\":\"chanA\",\"payload\":\"second\",\"time\":\"2026-09-05T10:00:01.5Z\"}\n\n")
		fl.Flush()
	}))
	defer srv.Close()

	c := New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := c.Listen(ctx, WithAutoReconnect(false))
	defer stream.Close()

	var got []Event
	for i := 0; i < 2; i++ {
		select {
		case e := <-stream.Events():
			got = append(got, e)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout en attendant l'événement %d", i+1)
		}
	}

	if len(got) != 2 {
		t.Fatalf("nb événements inattendu: %d", len(got))
	}
	if got[0].ID != 1 || got[0].Channel != "chanA" || got[0].Payload != "premier" {
		t.Fatalf("événement 1 inattendu: %+v", got[0])
	}
	want, _ := time.Parse(time.RFC3339Nano, "2026-09-05T10:00:01.5Z")
	if !got[1].Time.Equal(want) {
		t.Fatalf("horodatage inattendu: %v (want %v)", got[1].Time, want)
	}
}

func TestStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"channel": "evt2sse", "clients": 2, "last_id": 7, "connected": true})
	}))
	defer srv.Close()

	c := New(srv.URL)
	s, err := c.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Channel != "evt2sse" || s.Clients != 2 || s.LastID != 7 || !s.Connected {
		t.Fatalf("statut inattendu: %+v", s)
	}
}

func TestChannels(t *testing.T) {
	var lastMethod, lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastMethod, lastPath = r.Method, r.URL.Path
		if r.URL.Path == "/api/channels" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"default": "evt2sse", "channels": []string{"chanA", "chanB"}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": "chanB"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	ctx := context.Background()

	if err := c.Subscribe(ctx, "chanA"); err != nil {
		t.Fatal(err)
	}
	if lastMethod != http.MethodPost || lastPath != "/api/channels" {
		t.Fatalf("subscribe: %s %s", lastMethod, lastPath)
	}

	if err := c.Unsubscribe(ctx, "chanB"); err != nil {
		t.Fatal(err)
	}
	if lastMethod != http.MethodDelete || lastPath != "/api/channels/chanB" {
		t.Fatalf("unsubscribe: %s %s", lastMethod, lastPath)
	}

	chans, err := c.Channels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(chans) != 2 || chans[0] != "chanA" || chans[1] != "chanB" {
		t.Fatalf("canaux inattendus: %v", chans)
	}
}

func TestUnsubscribeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "channel manquant"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	if err := c.Unsubscribe(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "manquant") {
		t.Fatalf("erreur inattendue: %v", err)
	}
}

// TestListenIgnoresReady : l'événement "ready" ne doit pas être exposé en Event.
func TestListenIgnoresReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		fmt.Fprint(w, "event: ready\n")
		fmt.Fprint(w, "data: {\"channel\":\"evt2sse\",\"connected\":true}\n\n")
		fl.Flush()
		fmt.Fprint(w, "data: {\"id\":9,\"channel\":\"evt2sse\",\"payload\":\"le vrai message\",\"time\":\"2026-09-05T10:00:02Z\"}\n\n")
		fl.Flush()
	}))
	defer srv.Close()

	c := New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream := c.Listen(ctx, WithAutoReconnect(false))
	defer stream.Close()

	select {
	case e, ok := <-stream.Events():
		if !ok {
			t.Fatal("flux fermé avant réception du message")
		}
		if e.ID != 9 || e.Payload != "le vrai message" {
			t.Fatalf("événement inattendu: %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout en attendant le message")
	}
}
