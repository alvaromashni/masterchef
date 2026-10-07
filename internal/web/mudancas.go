package web

import (
	"sort"
	"time"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/store"
)

// Mudancas é o modelo da página "/mudancas".
type Mudancas struct {
	Produtos []GrupoProduto
	Total    int
	// VistoAte é a data do evento mais recente mostrado. O botão "marcar
	// como visto" grava esse valor (e não "agora"): um evento que chegar
	// enquanto a página está aberta continua como não visto.
	VistoAte time.Time
}

// GrupoProduto junta os eventos de um produto, separados por escopo.
type GrupoProduto struct {
	Produto config.Produto
	Escopos []GrupoEscopo
}

// GrupoEscopo são os eventos de uma célula, do mais novo para o mais antigo.
type GrupoEscopo struct {
	Nome    string // vazio = "a classificar"
	Eventos []store.Evento
}

// montarMudancas agrupa os eventos por produto e escopo, na ordem do
// config.yaml, com "a classificar" no fim de cada produto.
func montarMudancas(produtos []config.Produto, eventos []store.Evento) Mudancas {
	m := Mudancas{Total: len(eventos)}

	// porCelula["produto"]["escopo"] -> eventos
	porCelula := map[string]map[string][]store.Evento{}
	for _, e := range eventos {
		if porCelula[e.ProductSlug] == nil {
			porCelula[e.ProductSlug] = map[string][]store.Evento{}
		}
		porCelula[e.ProductSlug][e.Scope] = append(porCelula[e.ProductSlug][e.Scope], e)
		if e.OccurredAt.After(m.VistoAte) {
			m.VistoAte = e.OccurredAt
		}
	}

	for _, p := range produtos {
		celulas := porCelula[p.Slug]
		if len(celulas) == 0 {
			continue
		}
		grupo := GrupoProduto{Produto: p}

		conhecidos := map[string]bool{}
		for _, e := range p.Escopos {
			conhecidos[e.Nome] = true
			if evs := celulas[e.Nome]; len(evs) > 0 {
				grupo.Escopos = append(grupo.Escopos, GrupoEscopo{Nome: e.Nome, Eventos: maisNovosPrimeiro(evs)})
			}
		}

		// Sem escopo ou com escopo que o produto não tem: "a classificar",
		// a mesma regra da matriz.
		var semEscopo []store.Evento
		for nome, evs := range celulas {
			if !conhecidos[nome] {
				semEscopo = append(semEscopo, evs...)
			}
		}
		if len(semEscopo) > 0 {
			grupo.Escopos = append(grupo.Escopos, GrupoEscopo{Nome: aClassificar, Eventos: maisNovosPrimeiro(semEscopo)})
		}
		m.Produtos = append(m.Produtos, grupo)
	}
	return m
}

// maisNovosPrimeiro devolve uma cópia ordenada do mais recente para o mais
// antigo (desempate pelo id, que cresce na ordem de gravação).
func maisNovosPrimeiro(eventos []store.Evento) []store.Evento {
	copia := append([]store.Evento(nil), eventos...)
	sort.SliceStable(copia, func(a, b int) bool {
		if !copia[a].OccurredAt.Equal(copia[b].OccurredAt) {
			return copia[a].OccurredAt.After(copia[b].OccurredAt)
		}
		return copia[a].ID > copia[b].ID
	})
	return copia
}
