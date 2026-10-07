package web

import (
	"testing"
	"time"
)

func TestHaQuanto(t *testing.T) {
	agora := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	casos := []struct {
		antes time.Duration
		quer  string
	}{
		{30 * time.Second, "agora há pouco"},
		{5 * time.Minute, "há 5 min"},
		{3 * time.Hour, "há 3 h"},
		{47 * time.Hour, "há 47 h"},
		{72 * time.Hour, "há 3 dias"},
	}
	for _, c := range casos {
		if got := haQuanto(agora, agora.Add(-c.antes)); got != c.quer {
			t.Errorf("haQuanto(-%v) = %q, esperava %q", c.antes, got, c.quer)
		}
	}
	if got := haQuanto(agora, time.Time{}); got != "nunca" {
		t.Errorf("data zero = %q, esperava nunca", got)
	}
}
