package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/laurentpoirierfr/evt2sse/internal/relay"
)

func TestBroadcast(t *testing.T) {
	srv := New(nil, "evt2sse")

	a := make(chan []byte, 8)
	b := make(chan []byte, 8)
	srv.mu.Lock()
	srv.clients["a"] = a
	srv.clients["b"] = b
	srv.mu.Unlock()

	srv.broadcast(relay.Notify{Channel: "evt2sse", Payload: `{"k":1}`})

	for _, ch := range []chan []byte{a, b} {
		select {
		case evt := <-ch:
			if len(evt) == 0 {
				t.Fatal("événement vide reçu")
			}
		default:
			t.Fatal("client n'a pas reçu l'événement")
		}
	}
}

func TestChannelsAPI(t *testing.T) {
	r := relay.New("postgres://u:p@h/db?sslmode=disable", "evt2sse")
	srv := New(r, "evt2sse")
	s := httptest.NewServer(srv.Handler())
	defer s.Close()

	resp, err := http.Get(s.URL + "/api/channels")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("statut inattendu: %d", resp.StatusCode)
	}

	var out struct {
		Default  string   `json:"default"`
		Channels []string `json:"channels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Default != "evt2sse" {
		t.Fatalf("default inattendu: %q", out.Default)
	}
	if !reflect.DeepEqual(out.Channels, []string{}) {
		t.Fatalf("canaux inattendus: %v", out.Channels)
	}
}

func TestHistoryAndReplay(t *testing.T) {
	srv := New(nil, "evt2sse")

	for i := 1; i <= 3; i++ {
		srv.broadcast(relay.Notify{Channel: "evt2sse", Payload: `{"i":` + string(rune('0'+i)) + `}`})
	}

	srv.mu.Lock()
	got := srv.replayAfter(1)
	contiguous := got.contiguous
	count := len(got.frames)
	from := got.from
	srv.mu.Unlock()

	if !contiguous {
		t.Fatal("reprise contiguë attendue")
	}
	if count != 2 {
		t.Fatalf("2 événements manqués attendus, got %d", count)
	}
	if from != 1 {
		t.Fatalf("from=1 attendu, got %d", from)
	}
	joined := string(got.frames[0]) + string(got.frames[1])
	if !strings.Contains(joined, `"id":2`) || !strings.Contains(joined, `"id":3`) {
		t.Fatalf("rejeu incorrect: %q", joined)
	}

	// Trou dans l'historique => reprise non contiguë.
	srv.mu.Lock()
	srv.history = append([]historyEntry(nil), srv.history[len(srv.history)-1:]...)
	got2 := srv.replayAfter(1)
	srv.mu.Unlock()

	if got2.contiguous {
		t.Fatal("reprise non contiguë attendue après purge d'historique")
	}
}

func TestHistoryBounded(t *testing.T) {
	srv := New(nil, "evt2sse")
	for i := 0; i < historyRetention+50; i++ {
		srv.broadcast(relay.Notify{Channel: "evt2sse", Payload: "x"})
	}
	if len(srv.history) != historyRetention {
		t.Fatalf("historique borné attendu (%d), got %d", historyRetention, len(srv.history))
	}
	first := srv.history[0].id
	if first != int64(51) {
		t.Fatalf("le plus ancien événement conservé doit être id 51, got %d", first)
	}
}

func TestParseLastEventID(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"abc", 0},
		{"-3", 0},
		{"123", 123},
		{"9999999999", 9999999999},
		{"123extra", 0},
	}
	for _, c := range cases {
		if got := parseLastEventID(c.in); got != c.want {
			t.Fatalf("parseLastEventID(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestOpsEndpoints(t *testing.T) {
	// Relais non démarré, non connecté : readiness doit répondre 503.
	r := relay.New("postgres://u:p@h/db?sslmode=disable", "evt2sse")
	srv := New(r, "evt2sse")
	s := httptest.NewServer(srv.Handler())
	defer s.Close()

	// Liveness : toujours la même réponse et 200.
	resp, err := http.Get(s.URL + "/ops/liveness")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("liveness: statut %d attendu 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Readiness : pas connecté -> 503.
	resp, err = http.Get(s.URL + "/ops/readiness")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readiness: statut %d attendu 503", resp.StatusCode)
	}
	resp.Body.Close()

	// Info : métadonnées de build exposées.
	resp, err = http.Get(s.URL + "/ops/info")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var info map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info["name"] != "evt2sse" {
		t.Fatalf("info: name=%q attendu evt2sse", info["name"])
	}
}
