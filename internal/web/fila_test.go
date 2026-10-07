package web

import (
	"slices"
	"testing"
	"time"

	"github.com/alvaromashni/masterchef/internal/store"
)

func TestExtrairCriterios(t *testing.T) {
	descricao := "Contexto da tarefa.\n\n" +
		"## Critérios de aceite\n" +
		"- [ ] retorna 200 com token válido\n" +
		"- [x] retorna 401 sem token\r\n" +
		"  * [X] item recuado com asterisco\n" +
		"- item comum, sem caixa\n" +
		"- [] caixa malformada\n"

	got := extrairCriterios(descricao)
	quer := []Criterio{
		{Texto: "retorna 200 com token válido", Marcado: false},
		{Texto: "retorna 401 sem token", Marcado: true},
		{Texto: "item recuado com asterisco", Marcado: true},
	}
	if !slices.Equal(got, quer) {
		t.Errorf("critérios = %+v\nesperava %+v", got, quer)
	}
	if extrairCriterios("") != nil {
		t.Error("descrição vazia deveria dar lista vazia")
	}
}

func TestMontarFila(t *testing.T) {
	dia := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	d := dadosPainel{
		PRs: []store.PR{
			{ID: 1, Number: 1, State: "open", RiskLevel: "baixo", CreatedAt: dia(1)},
			{ID: 2, Number: 2, State: "open", RiskLevel: "alto", CreatedAt: dia(5)},
			{ID: 3, Number: 3, State: "open", RiskLevel: "alto", CreatedAt: dia(2)},
			{ID: 4, Number: 4, State: "open", RiskLevel: "medio", CreatedAt: dia(3)},
			{ID: 5, Number: 5, State: "open", Draft: true, RiskLevel: "alto"}, // rascunho fica fora
			{ID: 6, Number: 6, State: "merged", RiskLevel: "alto"},            // mergeado fica fora
		},
		Issues: []store.Issue{
			{ID: "i1", Identifier: "ABC-1", Description: "- [ ] critério"},
		},
		Vinculos: []store.Vinculo{{IssueID: "i1", PRID: 3}},
	}

	fila := montarFila(d)

	var ordem []int
	for _, item := range fila {
		ordem = append(ordem, item.PR.Number)
	}
	// alto (mais antigo primeiro), depois medio, depois baixo.
	if quer := []int{3, 2, 4, 1}; !slices.Equal(ordem, quer) {
		t.Fatalf("ordem = %v, esperava %v", ordem, quer)
	}
	if len(fila[0].Issues) != 1 || fila[0].Issues[0].Criterios[0].Texto != "critério" {
		t.Errorf("PR #3 deveria trazer a issue ABC-1 com o critério: %+v", fila[0].Issues)
	}
}
