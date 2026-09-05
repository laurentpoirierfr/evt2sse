package relay

import (
	"reflect"
	"testing"
	"time"
)

func TestQuoteLiteral(t *testing.T) {
	cases := map[string]string{
		"simple":      "'simple'",
		"it's":        "'it''s'",
		`a"b`:         `'a"b'`,
		"\\slash":     "'\\slash'",
		"déjà — utf8": "'déjà — utf8'",
	}
	for in, want := range cases {
		if got := quoteLiteral(in); got != want {
			t.Errorf("quoteLiteral(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewNoChannels(t *testing.T) {
	r := New("postgres://u:p@h/db?sslmode=disable", "evt2sse")
	if r.DefaultChannel() != "evt2sse" {
		t.Fatalf("canal par défaut inattendu: %q", r.DefaultChannel())
	}
	if len(r.Channels()) != 0 {
		t.Fatalf("canaux attendus vides, got %v", r.Channels())
	}
	if r.Connected() {
		t.Fatal("relais non démarré ne doit pas être connecté")
	}
}

func TestUnsubscribeUnknown(t *testing.T) {
	r := New("postgres://u:p@h/db?sslmode=disable", "evt2sse")
	if err := r.UnsubscribeChannel("inconnu"); err != nil {
		t.Fatalf("désabonnement inconnu ne doit pas échouer: %v", err)
	}
	if !reflect.DeepEqual(r.Channels(), []string{}) {
		t.Fatalf("canaux inattendus: %v", r.Channels())
	}
}

func TestBackoffDelay(t *testing.T) {
	if d := backoffDelay(0); d < 800*time.Millisecond || d > 1200*time.Millisecond {
		t.Fatalf("backoffDelay(0) hors bornes [800ms,1200ms]: %v", d)
	}
	// La valeur bornée aux alentours de 30s ne doit jamais l'excéder malgré le jitter.
	for i := 0; i < 100; i++ {
		if d := backoffDelay(50); d <= 0 || d > 30*time.Second {
			t.Fatalf("backoffDelay(50) = %v (attendu (0, 30s])", d)
		}
	}
}

func TestIdemStore(t *testing.T) {
	d := newIdemStore(4, time.Minute)
	d.add("a")
	if !d.seen("a") {
		t.Fatal("id ajouté doit être vu")
	}
	if d.seen("b") {
		t.Fatal("id inconnu ne doit pas être vu")
	}
	// Borné : l'éviction des plus anciens maintient la taille ≤ max.
	for i := 0; i < 20; i++ {
		d.add(string(rune('a' + i)))
	}
	if len(d.ids) > 4 {
		t.Fatalf("idemStore non borné: %d ids", len(d.ids))
	}
	// Expiration.
	d2 := newIdemStore(4, 1*time.Millisecond)
	d2.add("x")
	time.Sleep(2 * time.Millisecond)
	d2.purgeLocked()
	if len(d2.ids) != 0 {
		t.Fatalf("ids expirés non purgés: %d", len(d2.ids))
	}
}
