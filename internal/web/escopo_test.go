package web

import (
	"testing"
	"time"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/product"
	"github.com/alvaromashni/masterchef/internal/store"
)

func TestMontarPaginaEscopo(t *testing.T) {
	agora := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	p := config.Produto{Slug: "x", Escopos: config.Escopos{{Nome: "api"}, {Nome: "front"}}}

	d := dadosPainel{
		Issues: []store.Issue{
			{ID: "1", Identifier: "X-1", ProductSlug: "x", StateName: "Done", StateType: "completed", ScopeLabel: "api"},
			{ID: "2", Identifier: "X-2", ProductSlug: "x", StateName: "In Progress", StateType: "started", ScopeLabel: "api", UpdatedAt: agora.Add(-time.Hour)},
			{ID: "3", Identifier: "X-3", ProductSlug: "x", StateName: "In Progress", StateType: "started", ScopeLabel: "api", UpdatedAt: agora},
			{ID: "4", Identifier: "X-4", ProductSlug: "x", StateName: "Todo", StateType: "unstarted", ScopeLabel: "front"}, // label front...
			{ID: "5", Identifier: "X-5", ProductSlug: "x", StateName: "Todo", StateType: "unstarted", ScopeLabel: "mobile"},
			{ID: "6", Identifier: "Y-1", ProductSlug: "y", StateName: "Todo", StateType: "unstarted", ScopeLabel: "api"}, // outro produto
		},
		PRs: []store.PR{
			{ID: 10, ProductSlug: "x", Scope: "api", State: "open", RiskLevel: "baixo"},
			{ID: 11, ProductSlug: "x", Scope: "api", State: "open", RiskLevel: "alto"},
			{ID: 12, ProductSlug: "x", Scope: "api", State: "merged", RiskLevel: "alto"},
			{ID: 13, ProductSlug: "x", Scope: "front", State: "open", RiskLevel: "alto"},
		},
		Vinculos: []store.Vinculo{{IssueID: "4", PRID: 10}}, // ...mas tem PR na api: vai para api
	}
	eventos := []store.Evento{
		{ID: 1, ProductSlug: "x", Scope: "api", Ref: "e-api"},
		{ID: 2, ProductSlug: "x", Scope: "front", Ref: "e-front"},
		{ID: 3, ProductSlug: "x", Scope: "", Ref: "e-sem"},
		{ID: 4, ProductSlug: "x", Scope: "mobile", Ref: "e-mobile"},
	}

	api := montarPaginaEscopo(p, "api", d, eventos, product.Documento{})

	var estados []string
	for _, g := range api.IssuesPorEstado {
		estados = append(estados, g.Estado)
	}
	// started primeiro, depois unstarted, depois completed.
	if len(estados) != 3 || estados[0] != "In Progress" || estados[1] != "Todo" || estados[2] != "Done" {
		t.Fatalf("grupos = %v, esperava [In Progress Todo Done]", estados)
	}
	if g := api.IssuesPorEstado[0].Issues; g[0].Identifier != "X-3" || g[1].Identifier != "X-2" {
		t.Errorf("dentro do grupo, a mais recente primeiro: %v, %v", g[0].Identifier, g[1].Identifier)
	}
	if g := api.IssuesPorEstado[1].Issues; len(g) != 1 || g[0].Identifier != "X-4" {
		t.Errorf("X-4 deveria estar na api pelo PR vinculado: %+v", g)
	}
	if len(api.PRsAbertos) != 2 || api.PRsAbertos[0].ID != 11 {
		t.Errorf("PRs abertos da api, alto primeiro: %+v", api.PRsAbertos)
	}
	if len(api.Eventos) != 1 || api.Eventos[0].Ref != "e-api" {
		t.Errorf("eventos da api = %+v", api.Eventos)
	}

	sem := montarPaginaEscopo(p, aClassificar, d, eventos, product.Documento{})
	if len(sem.IssuesPorEstado) != 1 || sem.IssuesPorEstado[0].Issues[0].Identifier != "X-5" {
		t.Errorf("a classificar deveria ter só X-5 (escopo mobile desconhecido): %+v", sem.IssuesPorEstado)
	}
	if len(sem.Eventos) != 2 {
		t.Errorf("a classificar deveria ter os eventos sem escopo e de escopo desconhecido: %+v", sem.Eventos)
	}
}
