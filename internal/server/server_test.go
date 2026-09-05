package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
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
