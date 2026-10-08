package web

import (
	"testing"
	"time"

	"github.com/alvaromashni/masterchef/internal/store"
)

func TestHaQuanto(t *testing.T) {
	agora := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	casos := []struct {
		antes time.Duration
		quer  string
	}{
		{30 * time.Second, "agora há pouco"},
		{5 * time.Minute, "há 5 min"},
		{3 * time.Hour, "há 3h"},
		{23 * time.Hour, "há 23h"},
		{47 * time.Hour, "há 1d"},
		{72 * time.Hour, "há 3d"},
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

func TestLerFalhaSync(t *testing.T) {
	if f := lerFalhaSync(""); f != nil {
		t.Errorf("sem erro deveria dar nil, veio %+v", f)
	}
	f := lerFalhaSync("Ciclo de 2026-10-07T22:52:00Z:\nproduto-x: Forbidden\nrepo/api: HTTP 403\n")
	if f == nil || !f.Quando.Equal(time.Date(2026, 10, 7, 22, 52, 0, 0, time.UTC)) || len(f.Erros) != 2 || f.Erros[0] != "produto-x: Forbidden" {
		t.Errorf("falha = %+v", f)
	}
}

func TestHoraDoEvento(t *testing.T) {
	agora := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	casos := []struct {
		t    time.Time
		quer string
	}{
		{time.Date(2026, 10, 7, 9, 5, 0, 0, time.Local), "09:05"},
		{time.Date(2026, 10, 6, 19, 20, 0, 0, time.Local), "ontem 19:20"},
		{time.Date(2026, 10, 1, 8, 0, 0, 0, time.Local), "1 out"},
	}
	for _, c := range casos {
		if got := horaDoEvento(agora, c.t); got != c.quer {
			t.Errorf("horaDoEvento(%v) = %q, esperava %q", c.t, got, c.quer)
		}
	}
}

func TestTextoDoEvento(t *testing.T) {
	e := store.Evento{Kind: store.EventoPRFechado, Summary: "Fechado sem merge: cache de saldo"}
	if rotuloEvento(e) != "PR fechado" || textoEvento(e) != "cache de saldo (sem merge)" {
		t.Errorf("rótulo %q, texto %q", rotuloEvento(e), textoEvento(e))
	}
	e = store.Evento{Kind: store.EventoPRAberto, Summary: "Reaberto: login"}
	if rotuloEvento(e) != "PR reaberto" || textoEvento(e) != "login" {
		t.Errorf("rótulo %q, texto %q", rotuloEvento(e), textoEvento(e))
	}
	if refCurta("alvaromashni/api#42") != "api#42" || refCurta("ABC-1") != "ABC-1" {
		t.Errorf("refCurta errada")
	}
}
