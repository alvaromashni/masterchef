package web

import (
	"time"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/escopo"
	"github.com/alvaromashni/masterchef/internal/risk"
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

// Celula é o cruzamento produto × escopo (seção 11 do CONTEXT.md).
type Celula struct {
	// Existe é falso quando o produto não tem esse escopo configurado
	// (ex.: produto Y não tem "infra"); a tela mostra um traço.
	Existe bool

	EmAndamento       int       // issues com estado do tipo "started"
	AguardandoReview  int       // PRs abertos e que não são rascunho
	RiscoAltoPendente bool      // algum desses PRs tem risco alto
	UltimaAtividade   time.Time // mudança mais recente de issue ou PR da célula
	Parada            bool      // sem atividade há mais que stale_after, com issues em andamento
}

// aClassificar é a chave interna da coluna "a classificar".
const aClassificar = ""

// dadosPainel é tudo que vem do banco para montar as telas.
type dadosPainel struct {
	Issues   []store.Issue
	PRs      []store.PR
	Vinculos []store.Vinculo
}

// aguardaReview diz se um PR está na fila de review: aberto e não rascunho.
// (O painel não lê o estado das revisões no GitHub; "aberto e pronto" basta.)
func aguardaReview(p store.PR) bool {
	return p.State == "open" && !p.Draft
}

// montarMatriz cruza a config com os dados do banco. É uma função pura
// (sem banco, sem HTTP, "agora" vem de fora) para ser fácil de testar.
func montarMatriz(produtos []config.Produto, d dadosPainel, staleAfter time.Duration, agora time.Time) Matriz {
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

	// celulas["produto"]["escopo"] acumula os números de cada célula.
	celulas := map[string]map[string]*Celula{}
	celula := func(produto, nomeEscopo string) *Celula {
		if celulas[produto] == nil {
			celulas[produto] = map[string]*Celula{}
		}
		if celulas[produto][nomeEscopo] == nil {
			celulas[produto][nomeEscopo] = &Celula{}
		}
		return celulas[produto][nomeEscopo]
	}

	for _, p := range d.PRs {
		c := celula(p.ProductSlug, p.Scope)
		registrarAtividade(c, p.UpdatedAt)
		if aguardaReview(p) {
			c.AguardandoReview++
			if p.RiskLevel == risk.Alto {
				c.RiscoAltoPendente = true
			}
		}
	}

	escoposPorIssue := escoposDosPRsPorIssue(d)
	for _, i := range d.Issues {
		for _, e := range escopo.DaIssue(escoposPorIssue[i.ID], i.ScopeLabel) {
			c := celula(i.ProductSlug, e)
			registrarAtividade(c, i.UpdatedAt)
			if i.StateType == "started" {
				c.EmAndamento++
			}
		}
	}

	for _, p := range produtos {
		linha := LinhaMatriz{Produto: p, AClassificar: Celula{Existe: true}}
		doProduto := map[string]bool{}
		for _, e := range p.Escopos {
			doProduto[e.Nome] = true
		}

		for _, nome := range m.Escopos {
			c := Celula{}
			if v := celulas[p.Slug][nome]; v != nil && doProduto[nome] {
				c = *v
			}
			c.Existe = doProduto[nome]
			linha.Celulas = append(linha.Celulas, c)
		}

		// Vai para "a classificar" o que não tem escopo e o que tem uma label
		// de um escopo que este produto não configurou (ex.: "scope:mobile"
		// num produto sem mobile). Assim nenhuma issue some da tela.
		for nome, v := range celulas[p.Slug] {
			if !doProduto[nome] {
				somarCelula(&linha.AClassificar, *v)
			}
		}

		for i := range linha.Celulas {
			marcarParada(&linha.Celulas[i], staleAfter, agora)
		}
		marcarParada(&linha.AClassificar, staleAfter, agora)
		m.Linhas = append(m.Linhas, linha)
	}
	return m
}

// escoposDosPRsPorIssue devolve, para cada issue, os escopos dos PRs vinculados.
func escoposDosPRsPorIssue(d dadosPainel) map[string][]string {
	escopoDoPR := map[int64]string{}
	for _, p := range d.PRs {
		escopoDoPR[p.ID] = p.Scope
	}
	resultado := map[string][]string{}
	for _, v := range d.Vinculos {
		if e, ok := escopoDoPR[v.PRID]; ok {
			resultado[v.IssueID] = append(resultado[v.IssueID], e)
		}
	}
	return resultado
}

func registrarAtividade(c *Celula, t time.Time) {
	if t.After(c.UltimaAtividade) {
		c.UltimaAtividade = t
	}
}

func somarCelula(destino *Celula, c Celula) {
	destino.EmAndamento += c.EmAndamento
	destino.AguardandoReview += c.AguardandoReview
	destino.RiscoAltoPendente = destino.RiscoAltoPendente || c.RiscoAltoPendente
	registrarAtividade(destino, c.UltimaAtividade)
}

// marcarParada aplica a regra da seção 8: parada = última atividade mais
// antiga que stale_after E com issues em andamento.
func marcarParada(c *Celula, staleAfter time.Duration, agora time.Time) {
	c.Parada = c.EmAndamento > 0 && agora.Sub(c.UltimaAtividade) > staleAfter
}
