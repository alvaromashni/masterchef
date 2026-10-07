package collector

import (
	"fmt"
	"time"

	"github.com/alvaromashni/masterchef/internal/store"
)

// EstadoAnterior é o que estava no banco ANTES de gravar o ciclo atual.
type EstadoAnterior struct {
	Issues map[string]store.Issue // por id
	PRs    map[int64]store.PR     // por id
}

// GerarEventos compara o estado anterior com o que acabou de chegar das APIs
// e devolve uma linha de evento para cada diferença relevante
// ("Como os eventos nascem", seção 9 do CONTEXT.md).
//
// É uma função pura: tudo que ela precisa vem pelos parâmetros, inclusive
// os escopos de cada issue (já calculados com os vínculos) e o horário.
//
// Regras:
//   - issue que não existia → issue_created
//   - issue com outro nome de estado → issue_state_changed ("A → B")
//   - PR que não existia → pr_opened (ou pr_merged / pr_closed, se já chegou assim)
//   - PR que mudou de estado → pr_merged, pr_closed, ou pr_opened se foi reaberto
//   - PR no mesmo estado, mas com updated_at diferente → pr_updated (ex.: novo push)
//
// Uma issue que aparece em duas células (PRs em dois escopos) gera um
// evento em cada célula, para que as duas mostrem a mudança.
func GerarEventos(antes EstadoAnterior, issues []store.Issue, prs []store.PR, escoposDaIssue map[string][]string, quando time.Time) []store.Evento {
	var eventos []store.Evento

	for _, nova := range issues {
		kind, resumo := mudancaDeIssue(antes.Issues, nova)
		if kind == "" {
			continue
		}
		escopos := escoposDaIssue[nova.ID]
		if len(escopos) == 0 {
			escopos = []string{""} // a classificar
		}
		for _, e := range escopos {
			eventos = append(eventos, store.Evento{
				OccurredAt:  quando,
				ProductSlug: nova.ProductSlug,
				Scope:       e,
				Kind:        kind,
				Ref:         nova.Identifier,
				Summary:     resumo,
				URL:         nova.URL,
			})
		}
	}

	for _, novo := range prs {
		kind, resumo := mudancaDePR(antes.PRs, novo)
		if kind == "" {
			continue
		}
		eventos = append(eventos, store.Evento{
			OccurredAt:  quando,
			ProductSlug: novo.ProductSlug,
			Scope:       novo.Scope,
			Kind:        kind,
			Ref:         fmt.Sprintf("%s#%d", novo.Repo, novo.Number),
			Summary:     resumo,
			URL:         novo.URL,
		})
	}
	return eventos
}

// mudancaDeIssue devolve o tipo de evento e o resumo, ou "" se nada relevante mudou.
func mudancaDeIssue(antes map[string]store.Issue, nova store.Issue) (string, string) {
	velha, existia := antes[nova.ID]
	switch {
	case !existia:
		return store.EventoIssueCriada, fmt.Sprintf("Nova issue: %s (%s)", nova.Title, nova.StateName)
	case velha.StateName != nova.StateName:
		return store.EventoIssueMudouEstado, fmt.Sprintf("%s → %s", velha.StateName, nova.StateName)
	}
	// Mudou título, descrição ou label, mas não o estado: não é "relevante"
	// para a tela de mudanças.
	return "", ""
}

// mudancaDePR devolve o tipo de evento e o resumo, ou "" se nada mudou.
func mudancaDePR(antes map[int64]store.PR, novo store.PR) (string, string) {
	velho, existia := antes[novo.ID]

	if !existia || velho.State != novo.State {
		switch novo.State {
		case "merged":
			return store.EventoPRMergeado, "Mergeado: " + novo.Title
		case "closed":
			return store.EventoPRFechado, "Fechado sem merge: " + novo.Title
		default:
			if existia {
				return store.EventoPRAberto, "Reaberto: " + novo.Title
			}
			return store.EventoPRAberto, "Aberto: " + novo.Title
		}
	}

	if !velho.UpdatedAt.Equal(novo.UpdatedAt) {
		return store.EventoPRAtualizado, "Atualizado: " + novo.Title
	}
	return "", ""
}
