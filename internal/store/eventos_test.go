package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestEventosEUltimaVisita(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "teste.db"), loggerSilencioso)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	t1 := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	err = s.SalvarSync(ctx, SyncResultado{Iniciado: t2, Eventos: []Evento{
		{OccurredAt: t1, ProductSlug: "x", Scope: "api", Kind: EventoPRAberto, Ref: "o/r#1", Summary: "Aberto", URL: "u"},
		{OccurredAt: t2, ProductSlug: "x", Scope: "", Kind: EventoIssueCriada, Ref: "ABC-1", Summary: "Nova", URL: "u"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	visita, err := s.UltimaVisita(ctx)
	if err != nil || !visita.IsZero() {
		t.Fatalf("sem visita ainda: %v, %v", visita, err)
	}
	todos, _ := s.EventosDesde(ctx, visita)
	if len(todos) != 2 || todos[0].Ref != "o/r#1" || todos[1].Scope != "" {
		t.Fatalf("eventos = %+v", todos)
	}

	if err := s.MarcarVistoAte(ctx, t1); err != nil {
		t.Fatal(err)
	}
	visita, _ = s.UltimaVisita(ctx)
	novos, _ := s.EventosDesde(ctx, visita)
	if !visita.Equal(t1) || len(novos) != 1 || novos[0].Ref != "ABC-1" {
		t.Errorf("depois de marcar visto até t1: visita=%v, novos=%+v", visita, novos)
	}
}
