package web

import (
	"sort"
	"time"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/escopo"
	"github.com/alvaromashni/masterchef/internal/product"
	"github.com/alvaromashni/masterchef/internal/risk"
	"github.com/alvaromashni/masterchef/internal/store"
)

// limiteEventosRecentes é quantos eventos a página de escopo mostra.
const limiteEventosRecentes = 20

// janelaConcluidas é por quanto tempo uma issue concluída ou cancelada
// continua aparecendo na página de escopo (design da Fase 6: "Done (7 dias)").
const janelaConcluidas = 7 * 24 * time.Hour

// PaginaEscopo é o modelo de "/p/{produto}/{escopo}" (seção 11 do CONTEXT.md).
type PaginaEscopo struct {
	Produto config.Produto
	Escopo  string // vazio = "a classificar"
	Repo    string // repo do escopo no config.yaml (vazio em "a classificar")

	// Celula traz os mesmos números da célula na matriz (em andamento,
	// última atividade, parada), calculados pela mesma função.
	Celula Celula

	IssuesPorEstado []GrupoEstado
	PRsAbertos      []store.PR
	Eventos         []EventoVisto
	Product         product.Documento // seção "## <escopo>" do PRODUCT.md
}

// GrupoEstado são as issues de um mesmo estado do Linear (ex.: "In Review").
type GrupoEstado struct {
	Estado    string
	Concluido bool // completed ou canceled: só aparecem as dos últimos 7 dias
	Issues    []store.Issue
}

// EventoVisto é um evento com a marca de "ainda não visto".
type EventoVisto struct {
	store.Evento
	NaoVisto bool
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
func montarPaginaEscopo(p config.Produto, alvo string, d dadosPainel, eventosDoProduto []store.Evento, doc product.Documento, ultimaVisita time.Time, staleAfter time.Duration, agora time.Time) PaginaEscopo {
	pagina := PaginaEscopo{Produto: p, Escopo: alvo, Product: doc}
	for _, e := range p.Escopos {
		if e.Nome == alvo {
			pagina.Repo = e.Repo
		}
	}

	// Os números do topo vêm da própria matriz. Com um produto só, as
	// colunas da matriz são exatamente os escopos dele, na mesma ordem.
	linha := montarMatriz([]config.Produto{p}, d, staleAfter, agora).Linhas[0]
	pagina.Celula = linha.AClassificar
	for i, e := range p.Escopos {
		if e.Nome == alvo {
			pagina.Celula = linha.Celulas[i]
		}
	}

	escoposPorIssue := escoposDosPRsPorIssue(d)
	porEstado := map[string][]store.Issue{}
	tipoDoEstado := map[string]string{}
	for _, i := range d.Issues {
		if i.ProductSlug != p.Slug {
			continue
		}
		concluida := i.StateType == "completed" || i.StateType == "canceled"
		if concluida && agora.Sub(i.UpdatedAt) > janelaConcluidas {
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
		tipo := tipoDoEstado[estado]
		pagina.IssuesPorEstado = append(pagina.IssuesPorEstado, GrupoEstado{
			Estado:    estado,
			Concluido: tipo == "completed" || tipo == "canceled",
			Issues:    issues,
		})
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
			pagina.Eventos = append(pagina.Eventos, EventoVisto{Evento: e, NaoVisto: e.OccurredAt.After(ultimaVisita)})
		}
	}
	return pagina
}
