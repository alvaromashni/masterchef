package collector

import (
	"strings"
	"testing"
	"time"

	"github.com/alvaromashni/masterchef/internal/store"
)

func TestGerarEventos(t *testing.T) {
	agora := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	ontem := agora.Add(-24 * time.Hour)

	issue := func(id, estado string) store.Issue {
		return store.Issue{ID: id, Identifier: "ABC-" + id, ProductSlug: "x", Title: "Issue " + id, StateName: estado, URL: "u" + id}
	}
	pr := func(id int64, estado string, atualizado time.Time) store.PR {
		return store.PR{ID: id, Repo: "o/x-api", Number: int(id), ProductSlug: "x", Scope: "api", Title: "PR", State: estado, URL: "https://github.com/pr", UpdatedAt: atualizado}
	}

	antes := EstadoAnterior{
		Issues: map[string]store.Issue{
			"1": issue("1", "Todo"),
			"2": issue("2", "In Progress"),
		},
		PRs: map[int64]store.PR{
			10: pr(10, "open", ontem),
			11: pr(11, "open", ontem),
			12: pr(12, "open", ontem),
			13: pr(13, "closed", ontem),
			14: pr(14, "open", ontem),
		},
	}

	casos := []struct {
		nome    string
		issues  []store.Issue
		prs     []store.PR
		escopos map[string][]string
		quer    []string // "kind ref escopo resumo", na ordem
	}{
		{
			nome:   "issue nova vai para a classificar se não tem escopo",
			issues: []store.Issue{issue("3", "Backlog")},
			quer:   []string{"issue_created ABC-3 [] Nova issue: Issue 3 (Backlog)"},
		},
		{
			nome:   "mudança de estado",
			issues: []store.Issue{issue("1", "In Progress")},
			quer:   []string{"issue_state_changed ABC-1 [] Todo → In Progress"},
		},
		{
			nome:   "issue sem mudança de estado não gera evento",
			issues: []store.Issue{issue("2", "In Progress")},
			quer:   nil,
		},
		{
			nome:    "issue em dois escopos gera um evento em cada",
			issues:  []store.Issue{issue("1", "Done")},
			escopos: map[string][]string{"1": {"api", "front"}},
			quer: []string{
				"issue_state_changed ABC-1 [api] Todo → Done",
				"issue_state_changed ABC-1 [front] Todo → Done",
			},
		},
		{
			nome: "PR novo aberto",
			prs:  []store.PR{pr(20, "open", agora)},
			quer: []string{"pr_opened o/x-api#20 [api] Aberto: PR"},
		},
		{
			nome: "PR novo que já chegou mergeado",
			prs:  []store.PR{pr(21, "merged", agora)},
			quer: []string{"pr_merged o/x-api#21 [api] Mergeado: PR"},
		},
		{
			nome: "PR mergeado",
			prs:  []store.PR{pr(10, "merged", agora)},
			quer: []string{"pr_merged o/x-api#10 [api] Mergeado: PR"},
		},
		{
			nome: "PR fechado sem merge",
			prs:  []store.PR{pr(11, "closed", agora)},
			quer: []string{"pr_closed o/x-api#11 [api] Fechado sem merge: PR"},
		},
		{
			nome: "PR reaberto",
			prs:  []store.PR{pr(13, "open", agora)},
			quer: []string{"pr_opened o/x-api#13 [api] Reaberto: PR"},
		},
		{
			nome: "PR com push novo (mesmo estado, outra data)",
			prs:  []store.PR{pr(12, "open", agora)},
			quer: []string{"pr_updated o/x-api#12 [api] Atualizado: PR"},
		},
		{
			nome: "PR igual não gera evento",
			prs:  []store.PR{pr(14, "open", ontem)},
			quer: nil,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			eventos := GerarEventos(antes, c.issues, c.prs, c.escopos, agora)

			var got []string
			for _, e := range eventos {
				got = append(got, e.Kind+" "+e.Ref+" ["+e.Scope+"] "+e.Summary)
				if !e.OccurredAt.Equal(agora) || e.ProductSlug != "x" || e.URL == "" {
					t.Errorf("evento incompleto: %+v", e)
				}
			}
			if strings.Join(got, "\n") != strings.Join(c.quer, "\n") {
				t.Errorf("eventos:\n%s\nesperava:\n%s", strings.Join(got, "\n"), strings.Join(c.quer, "\n"))
			}
		})
	}
}
