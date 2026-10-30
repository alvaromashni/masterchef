package web

import (
	"context"
	"net/url"
	"sort"
	"strings"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/contexto"
	"github.com/alvaromashni/masterchef/internal/escopo"
	"github.com/alvaromashni/masterchef/internal/store"
)

// feitasVisiveis é quantas issues concluídas aparecem abertas no roadmap;
// as demais ficam atrás de um "ver todas".
const feitasVisiveis = 10

// Roadmap são as issues do projeto no Linear separadas em três colunas:
// o que já foi feito, o que está em andamento e o que falta fazer
// (decisão do Alvinho em 2026-10-30: o roadmap são as issues do Linear).
type Roadmap struct {
	EmAndamento []ItemRoadmap
	AFazer      []ItemRoadmap
	Feito       []ItemRoadmap

	Total      int // sem as canceladas
	Percentual int // feitas / total, de 0 a 100
}

// ItemRoadmap é uma issue com os escopos (repos) em que ela cai.
type ItemRoadmap struct {
	store.Issue
	Escopos []string // vazio = a classificar
}

// FeitoVisivel e FeitoResto separam a coluna "Feito" para a tela.
func (r Roadmap) FeitoVisivel() []ItemRoadmap { return r.Feito[:min(len(r.Feito), feitasVisiveis)] }
func (r Roadmap) FeitoResto() []ItemRoadmap   { return r.Feito[min(len(r.Feito), feitasVisiveis):] }

// montarRoadmap separa as issues do produto pelo tipo de estado do Linear.
// Canceladas ficam de fora: não são trabalho feito nem por fazer.
func montarRoadmap(p config.Produto, d dadosPainel) Roadmap {
	var r Roadmap
	escoposPorIssue := escoposDosPRsPorIssue(d)
	for _, i := range d.Issues {
		if i.ProductSlug != p.Slug || i.StateType == "canceled" {
			continue
		}
		item := ItemRoadmap{Issue: i}
		for _, e := range escopo.DaIssue(escoposPorIssue[i.ID], i.ScopeLabel) {
			if c := celulaDe(p, e); c != aClassificar {
				item.Escopos = append(item.Escopos, c)
			}
		}
		r.Total++
		switch i.StateType {
		case "completed":
			r.Feito = append(r.Feito, item)
		case "started":
			r.EmAndamento = append(r.EmAndamento, item)
		default: // unstarted, backlog, triage
			r.AFazer = append(r.AFazer, item)
		}
	}
	if r.Total > 0 {
		r.Percentual = len(r.Feito) * 100 / r.Total
	}

	// Em andamento e feito: o que mexeu por último primeiro.
	maisRecente := func(itens []ItemRoadmap) {
		sort.Slice(itens, func(a, b int) bool { return itens[a].UpdatedAt.After(itens[b].UpdatedAt) })
	}
	maisRecente(r.EmAndamento)
	maisRecente(r.Feito)
	// A fazer: "Todo" (unstarted) antes do backlog; dentro de cada um, a
	// issue mais antiga primeiro, que costuma ser a próxima da fila.
	sort.Slice(r.AFazer, func(a, b int) bool {
		ta, tb := ordemDoTipo[r.AFazer[a].StateType], ordemDoTipo[r.AFazer[b].StateType]
		if ta != tb {
			return ta < tb
		}
		return r.AFazer[a].UpdatedAt.Before(r.AFazer[b].UpdatedAt)
	})
	return r
}

// ResumoProjeto é uma linha da tela "/projetos".
type ResumoProjeto struct {
	Produto config.Produto
	Roadmap Roadmap
}

// PaginaProjeto é o modelo de "/p/{produto}".
type PaginaProjeto struct {
	Produto  config.Produto
	Produtos []config.Produto // para as abas entre projetos
	Roadmap  Roadmap
	Contexto []ContextoRepo
}

// ContextoRepo é a lista de documentos da pasta docs/ de um repo do projeto.
type ContextoRepo struct {
	Escopo string
	Repo   string
	Docs   []DocLink
	Aviso  string // erro ao buscar no GitHub ou pasta vazia
}

// DocLink é um documento no índice, com o endereço dele no painel.
type DocLink struct {
	contexto.Doc
	Link  string
	Atual bool // o documento aberto agora (página do documento)
}

// PaginaDoc é o modelo de "/p/{produto}/docs/{escopo}/{caminho...}".
type PaginaDoc struct {
	Produto  config.Produto
	Escopo   string
	Repo     string
	Pagina   contexto.Pagina
	Contexto []ContextoRepo
}

// linkDoc monta o endereço de um documento no painel. O caminho no repo
// começa com "docs/", que tiramos da URL para ela ficar mais curta.
func linkDoc(produto, escopo, caminho string) string {
	dentro := strings.TrimPrefix(caminho, "docs/")
	partes := strings.Split(dentro, "/")
	for i, p := range partes {
		partes[i] = url.PathEscape(p)
	}
	return "/p/" + produto + "/docs/" + escopo + "/" + strings.Join(partes, "/")
}

// montarContexto busca o índice de docs/ de cada repo do produto. Um repo
// que falha vira um aviso na tela, sem derrubar a página inteira.
func montarContexto(ctx context.Context, docs *contexto.Leitor, p config.Produto, atual string) []ContextoRepo {
	var repos []ContextoRepo
	for _, e := range p.Escopos {
		c := ContextoRepo{Escopo: e.Nome, Repo: e.Repo}
		lista, err := docs.Indice(ctx, e.Repo)
		switch {
		case err != nil:
			c.Aviso = "Não foi possível ler a pasta docs/ no GitHub: " + err.Error()
		case len(lista) == 0:
			c.Aviso = "Nenhum Markdown em docs/ na branch principal."
		}
		for _, d := range lista {
			c.Docs = append(c.Docs, DocLink{
				Doc:   d,
				Link:  linkDoc(p.Slug, e.Nome, d.Caminho),
				Atual: e.Nome+"/"+d.Caminho == atual,
			})
		}
		repos = append(repos, c)
	}
	return repos
}
