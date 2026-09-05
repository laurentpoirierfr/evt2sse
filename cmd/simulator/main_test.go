package main

import (
	"strings"
	"testing"
)

func TestBuildPayload(t *testing.T) {
	p := buildPayload(`{"seq":{{n}},"ts":"{{ts}}"}`, 7)
	if !strings.Contains(p, `"seq":7`) {
		t.Errorf("compteur non remplacé: %s", p)
	}
	if !strings.Contains(p, `"ts":"`) {
		t.Errorf("horodatage non remplacé: %s", p)
	}
}

func TestBuildPayloadDefault(t *testing.T) {
	p := buildPayload(defaultPayload, 3)
	if !strings.Contains(p, `"seq":3`) {
		t.Errorf("compteur non remplacé: %s", p)
	}
}
