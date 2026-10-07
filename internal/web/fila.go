package web

import (
	"regexp"
	"sort"
	"strings"

	"github.com/alvaromashni/masterchef/internal/risk"
	"github.com/alvaromashni/masterchef/internal/store"
)

// ItemFila é um PR da fila de review com as issues vinculadas.
type ItemFila struct {
	PR     store.PR
	Issues []IssueComCriterios
}

// IssueComCriterios é uma issue vinculada e seus critérios de aceite.
type IssueComCriterios struct {
	Issue     store.Issue
	Criterios []Criterio
}

// Criterio é uma linha de checkbox da descrição da issue ("- [ ] texto").
type Criterio struct {
	Texto   string
	Marcado bool
}

// montarFila lista os PRs abertos e não-rascunho, ordenados por risco
// (alto primeiro) e, no mesmo risco, pelo mais antigo primeiro: quem
// espera há mais tempo vem antes.
func montarFila(d dadosPainel) []ItemFila {
	issuesPorID := map[string]store.Issue{}
	for _, i := range d.Issues {
		issuesPorID[i.ID] = i
	}
	issuesDoPR := map[int64][]string{}
	for _, v := range d.Vinculos {
		issuesDoPR[v.PRID] = append(issuesDoPR[v.PRID], v.IssueID)
	}

	var fila []ItemFila
	for _, p := range d.PRs {
		if !aguardaReview(p) {
			continue
		}
		item := ItemFila{PR: p}
		for _, id := range issuesDoPR[p.ID] {
			issue, ok := issuesPorID[id]
			if !ok {
				continue
			}
			item.Issues = append(item.Issues, IssueComCriterios{
				Issue:     issue,
				Criterios: extrairCriterios(issue.Description),
			})
		}
		// Ordem estável das issues dentro de um PR.
		sort.Slice(item.Issues, func(a, b int) bool {
			return item.Issues[a].Issue.Identifier < item.Issues[b].Issue.Identifier
		})
		fila = append(fila, item)
	}

	sort.SliceStable(fila, func(a, b int) bool {
		pa, pb := risk.Peso(fila[a].PR.RiskLevel), risk.Peso(fila[b].PR.RiskLevel)
		if pa != pb {
			return pa > pb
		}
		return fila[a].PR.CreatedAt.Before(fila[b].PR.CreatedAt)
	})
	return fila
}

// checkbox casa linhas de lista Markdown com caixa de seleção:
// "- [ ] texto", "* [x] texto", com ou sem recuo.
var checkbox = regexp.MustCompile(`^\s*[-*+]\s+\[([ xX])\]\s+(.+)$`)

// extrairCriterios pega os checkboxes da descrição da issue. Os critérios
// são só exibidos: o painel nunca marca nada no Linear.
func extrairCriterios(descricao string) []Criterio {
	var criterios []Criterio
	for _, linha := range strings.Split(descricao, "\n") {
		m := checkbox.FindStringSubmatch(strings.TrimRight(linha, "\r"))
		if m == nil {
			continue
		}
		criterios = append(criterios, Criterio{
			Texto:   strings.TrimSpace(m[2]),
			Marcado: m[1] != " ",
		})
	}
	return criterios
}
