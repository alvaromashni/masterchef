package web

import (
	"testing"
	"time"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/store"
)

func TestMontarMudancas(t *testing.T) {
	t1 := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	produtos := []config.Produto{
		{Slug: "x", Escopos: config.Escopos{{Nome: "api"}, {Nome: "front"}}},
		{Slug: "y", Escopos: config.Escopos{{Nome: "app"}}}, // sem eventos: some da página
	}
	eventos := []store.Evento{
		{ID: 1, OccurredAt: t1, ProductSlug: "x", Scope: "front", Ref: "f1"},
		{ID: 2, OccurredAt: t1, ProductSlug: "x", Scope: "api", Ref: "a1"},
		{ID: 3, OccurredAt: t2, ProductSlug: "x", Scope: "api", Ref: "a2"},
		{ID: 4, OccurredAt: t1, ProductSlug: "x", Scope: "", Ref: "c1"},
		{ID: 5, OccurredAt: t1, ProductSlug: "x", Scope: "mobile", Ref: "c2"}, // escopo desconhecido
	}

	m := montarMudancas(produtos, eventos)

	if m.Total != 5 || !m.VistoAte.Equal(t2) {
		t.Errorf("total = %d, vistoAte = %v; esperava 5 e %v", m.Total, m.VistoAte, t2)
	}
	if len(m.Produtos) != 1 {
		t.Fatalf("esperava só o produto x (y não tem eventos), veio %d", len(m.Produtos))
	}
	escopos := m.Produtos[0].Escopos
	if len(escopos) != 3 || escopos[0].Nome != "api" || escopos[1].Nome != "front" || escopos[2].Nome != "" {
		t.Fatalf("grupos fora de ordem: %+v", escopos)
	}
	if escopos[0].Eventos[0].Ref != "a2" {
		t.Errorf("em api o mais novo (a2) deveria vir primeiro, veio %s", escopos[0].Eventos[0].Ref)
	}
	if len(escopos[2].Eventos) != 2 {
		t.Errorf("a classificar deveria juntar sem escopo + escopo desconhecido: %+v", escopos[2].Eventos)
	}
}
