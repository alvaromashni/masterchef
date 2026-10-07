package web

import (
	"testing"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/store"
)

func TestMontarMatriz(t *testing.T) {
	produtos := []config.Produto{
		{Slug: "x", Escopos: config.Escopos{{Nome: "api"}, {Nome: "front"}}},
		{Slug: "y", Escopos: config.Escopos{{Nome: "app"}, {Nome: "api"}}},
	}
	contagens := []store.ContagemCelula{
		{ProductSlug: "x", Escopo: "api", EmAndamento: 2},
		{ProductSlug: "x", Escopo: "", EmAndamento: 1},       // sem label
		{ProductSlug: "x", Escopo: "mobile", EmAndamento: 3}, // escopo que x não tem
		{ProductSlug: "y", Escopo: "app", EmAndamento: 4},
	}

	m := montarMatriz(produtos, contagens)

	// Colunas na ordem de primeira aparição, sem repetir "api".
	if got := len(m.Escopos); got != 3 || m.Escopos[0] != "api" || m.Escopos[1] != "front" || m.Escopos[2] != "app" {
		t.Fatalf("colunas = %v, esperava [api front app]", m.Escopos)
	}

	x := m.Linhas[0]
	if x.Celulas[0].EmAndamento != 2 || !x.Celulas[0].Existe {
		t.Errorf("x/api = %+v", x.Celulas[0])
	}
	if x.Celulas[2].Existe {
		t.Errorf("x não tem o escopo app, a célula deveria não existir")
	}
	if x.AClassificar.EmAndamento != 4 {
		t.Errorf("x/a classificar = %d, esperava 4 (1 sem label + 3 com escopo desconhecido)", x.AClassificar.EmAndamento)
	}

	y := m.Linhas[1]
	if y.Celulas[1].Existe {
		t.Errorf("y não tem o escopo front")
	}
	if y.Celulas[2].EmAndamento != 4 {
		t.Errorf("y/app = %d, esperava 4", y.Celulas[2].EmAndamento)
	}
	if y.AClassificar.EmAndamento != 0 {
		t.Errorf("y/a classificar = %d, esperava 0", y.AClassificar.EmAndamento)
	}
}
