package relay

import (
	"reflect"
	"testing"
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
