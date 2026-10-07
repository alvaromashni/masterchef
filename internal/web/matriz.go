package web

import (
	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/store"
)

// Matriz é o modelo da página "/": linhas = produtos, colunas = escopos
// mais a coluna "a classificar" no fim.
type Matriz struct {
	Escopos []string // nomes das colunas de escopo (sem "a classificar")
	Linhas  []LinhaMatriz
}

// LinhaMatriz é um produto e suas células, na mesma ordem de Matriz.Escopos.
type LinhaMatriz struct {
	Produto      config.Produto
	Celulas      []Celula
	AClassificar Celula
}

// Celula é o cruzamento produto × escopo.
type Celula struct {
	// Existe é falso quando o produto não tem esse escopo configurado
	// (ex.: produto Y não tem "infra"); a tela mostra um traço.
	Existe      bool
	EmAndamento int
}

// montarMatriz cruza a config com as contagens do banco. É uma função pura
// (sem banco, sem HTTP) para ser fácil de testar.
func montarMatriz(produtos []config.Produto, contagens []store.ContagemCelula) Matriz {
	var m Matriz

	// Colunas: todos os escopos de todos os produtos, na ordem em que
	// aparecem pela primeira vez no config.yaml.
	vistos := map[string]bool{}
	for _, p := range produtos {
		for _, e := range p.Escopos {
			if !vistos[e.Nome] {
				vistos[e.Nome] = true
				m.Escopos = append(m.Escopos, e.Nome)
			}
		}
	}

	// Índice "produto -> escopo -> quantidade" para consulta rápida.
	porProduto := map[string]map[string]int{}
	for _, c := range contagens {
		if porProduto[c.ProductSlug] == nil {
			porProduto[c.ProductSlug] = map[string]int{}
		}
		porProduto[c.ProductSlug][c.Escopo] += c.EmAndamento
	}

	for _, p := range produtos {
		linha := LinhaMatriz{Produto: p, AClassificar: Celula{Existe: true}}
		doProduto := map[string]bool{}
		for _, e := range p.Escopos {
			doProduto[e.Nome] = true
		}

		for _, escopo := range m.Escopos {
			linha.Celulas = append(linha.Celulas, Celula{
				Existe:      doProduto[escopo],
				EmAndamento: porProduto[p.Slug][escopo],
			})
		}

		// Vai para "a classificar" o que não tem label de escopo ("") e o que
		// tem uma label de um escopo que este produto não configurou
		// (ex.: "scope:mobile" num produto sem mobile). Assim nenhuma issue
		// some da tela por causa de uma label errada.
		for escopo, n := range porProduto[p.Slug] {
			if !doProduto[escopo] {
				linha.AClassificar.EmAndamento += n
			}
		}
		m.Linhas = append(m.Linhas, linha)
	}
	return m
}
