package web

import (
	"testing"
	"time"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/store"
)

func TestMontarMatriz(t *testing.T) {
	agora := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	ha := func(d time.Duration) time.Time { return agora.Add(-d) }

	produtos := []config.Produto{
		{Slug: "x", Escopos: config.Escopos{{Nome: "api"}, {Nome: "front"}}},
		{Slug: "y", Escopos: config.Escopos{{Nome: "app"}, {Nome: "api"}}},
	}
	d := dadosPainel{
		Issues: []store.Issue{
			{ID: "1", ProductSlug: "x", StateType: "started", ScopeLabel: "api", UpdatedAt: ha(time.Hour)},
			{ID: "2", ProductSlug: "x", StateType: "started", ScopeLabel: "", UpdatedAt: ha(time.Hour)},          // sem label
			{ID: "3", ProductSlug: "x", StateType: "started", ScopeLabel: "mobile", UpdatedAt: ha(time.Hour)},    // escopo que x não tem
			{ID: "4", ProductSlug: "x", StateType: "started", ScopeLabel: "api", UpdatedAt: ha(100 * time.Hour)}, // label api, mas PR no front
			{ID: "5", ProductSlug: "y", StateType: "started", ScopeLabel: "app", UpdatedAt: ha(100 * time.Hour)},
			{ID: "6", ProductSlug: "y", StateType: "completed", ScopeLabel: "api", UpdatedAt: ha(100 * time.Hour)},
		},
		PRs: []store.PR{
			{ID: 10, ProductSlug: "x", Scope: "front", State: "open", RiskLevel: "alto", UpdatedAt: ha(90 * time.Hour)},
			{ID: 11, ProductSlug: "x", Scope: "api", State: "open", Draft: true, RiskLevel: "alto", UpdatedAt: ha(2 * time.Hour)},
		},
		Vinculos: []store.Vinculo{{IssueID: "4", PRID: 10}},
		NaoVistos: []store.Evento{
			{ProductSlug: "x", Scope: "api"},
			{ProductSlug: "x", Scope: "api"},
			{ProductSlug: "x", Scope: "mobile"},
		},
	}

	m := montarMatriz(produtos, d, 72*time.Hour, agora)

	// Colunas na ordem de primeira aparição, sem repetir "api".
	if len(m.Escopos) != 3 || m.Escopos[0] != "api" || m.Escopos[1] != "front" || m.Escopos[2] != "app" {
		t.Fatalf("colunas = %v, esperava [api front app]", m.Escopos)
	}

	x := m.Linhas[0]
	api, front := x.Celulas[0], x.Celulas[1]
	if api.EmAndamento != 1 {
		t.Errorf("x/api em andamento = %d, esperava 1 (a issue 4 foi para o front pelo PR)", api.EmAndamento)
	}
	if api.AguardandoReview != 0 || api.AltoPendentes != 0 {
		t.Errorf("x/api: rascunho não aguarda review: %+v", api)
	}
	if !api.UltimaAtividade.Equal(ha(time.Hour)) {
		t.Errorf("x/api última atividade = %v, esperava a issue de 1h atrás", api.UltimaAtividade)
	}
	if front.EmAndamento != 1 || front.AguardandoReview != 1 || front.AltoPendentes != 1 {
		t.Errorf("x/front = %+v; esperava 1 issue, 1 PR e risco alto", front)
	}
	if !front.Parada {
		t.Errorf("x/front: última atividade há 90h > 72h com issue em andamento, deveria estar parada")
	}
	if api.Parada {
		t.Errorf("x/api teve atividade há 1h, não está parada")
	}
	if x.Celulas[2].Existe {
		t.Errorf("x não tem o escopo app")
	}
	if api.NaoVistas != 2 || front.NaoVistas != 0 || x.AClassificar.NaoVistas != 1 {
		t.Errorf("não vistas: api=%d front=%d a classificar=%d; esperava 2, 0, 1",
			api.NaoVistas, front.NaoVistas, x.AClassificar.NaoVistas)
	}
	if x.AClassificar.EmAndamento != 2 {
		t.Errorf("x/a classificar = %d, esperava 2 (sem label + escopo desconhecido)", x.AClassificar.EmAndamento)
	}

	y := m.Linhas[1]
	if app := y.Celulas[2]; app.EmAndamento != 1 || !app.Parada {
		t.Errorf("y/app = %+v; esperava 1 em andamento e parada (100h)", app)
	}
	if yAPI := y.Celulas[0]; yAPI.EmAndamento != 0 || yAPI.Parada {
		t.Errorf("y/api = %+v; issue concluída não conta e sem andamento não fica parada", yAPI)
	}
}
