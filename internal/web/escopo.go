package web

import (
	"sort"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/escopo"
	"github.com/alvaromashni/masterchef/internal/product"
	"github.com/alvaromashni/masterchef/internal/risk"
	"github.com/alvaromashni/masterchef/internal/store"
)

// limiteEventosRecentes é quantos eventos a página de escopo mostra.
const limiteEventosRecentes = 20

// PaginaEscopo é o modelo de "/p/{produto}/{escopo}" (seção 11 do CONTEXT.md).
type PaginaEscopo struct {
	Produto config.Produto
	Escopo  string // vazio = "a classificar"

	IssuesPorEstado []GrupoEstado
	PRsAbertos      []store.PR
	Eventos         []store.Evento
	Product         product.Documento // seção "## <escopo>" do PRODUCT.md
}

// GrupoEstado são as issues de um mesmo estado do Linear (ex.: "In Review").
type GrupoEstado struct {
	Estado string
	Issues []store.Issue
}

// ordemDoTipo põe primeiro o que está em andamento e por último o que acabou.
var ordemDoTipo = map[string]int{"started": 0, "unstarted": 1, "backlog": 2, "triage": 3, "completed": 4, "canceled": 5}

// celulaDe devolve em qual coluna da matriz um escopo cai neste produto:
// o próprio nome, se o produto tem esse escopo, ou "" (a classificar).
func celulaDe(p config.Produto, nome string) string {
	for _, e := range p.Escopos {
		if e.Nome == nome {
			return nome
		}
	}
	return aClassificar
}

// montarPaginaEscopo filtra issues, PRs e eventos da célula produto × escopo.
// Usa a mesma regra de escopo da matriz, para as duas telas sempre baterem.
func montarPaginaEscopo(p config.Produto, alvo string, d dadosPainel, eventosDoProduto []store.Evento, doc product.Documento) PaginaEscopo {
	pagina := PaginaEscopo{Produto: p, Escopo: alvo, Product: doc}

	escoposPorIssue := escoposDosPRsPorIssue(d)
	porEstado := map[string][]store.Issue{}
	tipoDoEstado := map[string]string{}
	for _, i := range d.Issues {
		if i.ProductSlug != p.Slug {
			continue
		}
		for _, e := range escopo.DaIssue(escoposPorIssue[i.ID], i.ScopeLabel) {
			if celulaDe(p, e) == alvo {
				porEstado[i.StateName] = append(porEstado[i.StateName], i)
				tipoDoEstado[i.StateName] = i.StateType
				break // uma issue aparece uma vez só na página
			}
		}
	}
	for estado, issues := range porEstado {
		sort.Slice(issues, func(a, b int) bool { return issues[a].UpdatedAt.After(issues[b].UpdatedAt) })
		pagina.IssuesPorEstado = append(pagina.IssuesPorEstado, GrupoEstado{Estado: estado, Issues: issues})
	}
	sort.Slice(pagina.IssuesPorEstado, func(a, b int) bool {
		ea, eb := pagina.IssuesPorEstado[a].Estado, pagina.IssuesPorEstado[b].Estado
		if ordemDoTipo[tipoDoEstado[ea]] != ordemDoTipo[tipoDoEstado[eb]] {
			return ordemDoTipo[tipoDoEstado[ea]] < ordemDoTipo[tipoDoEstado[eb]]
		}
		return ea < eb
	})

	for _, pr := range d.PRs {
		if pr.ProductSlug == p.Slug && pr.State == "open" && celulaDe(p, pr.Scope) == alvo {
			pagina.PRsAbertos = append(pagina.PRsAbertos, pr)
		}
	}
	// Mesma ordem da fila de review: maior risco primeiro, depois o mais antigo.
	sort.SliceStable(pagina.PRsAbertos, func(a, b int) bool {
		pa, pb := risk.Peso(pagina.PRsAbertos[a].RiskLevel), risk.Peso(pagina.PRsAbertos[b].RiskLevel)
		if pa != pb {
			return pa > pb
		}
		return pagina.PRsAbertos[a].CreatedAt.Before(pagina.PRsAbertos[b].CreatedAt)
	})

	for _, e := range eventosDoProduto {
		if celulaDe(p, e.Scope) == alvo && len(pagina.Eventos) < limiteEventosRecentes {
			pagina.Eventos = append(pagina.Eventos, e)
		}
	}
	return pagina
}
