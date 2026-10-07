package escopo

import (
	"slices"
	"testing"
)

func TestDaLabel(t *testing.T) {
	casos := []struct {
		nome   string
		labels []string
		quer   string
	}{
		{"sem labels", nil, ""},
		{"sem label de escopo", []string{"bug", "urgente"}, ""},
		{"uma label de escopo", []string{"bug", "scope:api"}, "api"},
		{"maiúsculas e espaços", []string{" Scope:Front "}, "front"},
		{"prefixo sem nome é ignorado", []string{"scope:"}, ""},
		{"duas labels: a primeira em ordem alfabética", []string{"scope:infra", "scope:api"}, "api"},
		{"parecida mas não é escopo", []string{"scopes:api", "myscope:api"}, ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := DaLabel(c.labels); got != c.quer {
				t.Errorf("DaLabel(%v) = %q, esperava %q", c.labels, got, c.quer)
			}
		})
	}
}

func TestDaIssue(t *testing.T) {
	casos := []struct {
		nome  string
		prs   []string
		label string
		quer  []string
	}{
		{"PR vinculado vence a label", []string{"front"}, "api", []string{"front"}},
		{"PRs em dois escopos: aparece nos dois", []string{"infra", "api", "infra"}, "", []string{"api", "infra"}},
		{"sem PR usa a label", nil, "api", []string{"api"}},
		{"sem PR e sem label: a classificar", nil, "", []string{""}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := DaIssue(c.prs, c.label); !slices.Equal(got, c.quer) {
				t.Errorf("DaIssue(%v, %q) = %q, esperava %q", c.prs, c.label, got, c.quer)
			}
		})
	}
}
