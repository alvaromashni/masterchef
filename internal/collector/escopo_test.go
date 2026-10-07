package collector

import "testing"

func TestEscopoDaLabel(t *testing.T) {
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
			if got := EscopoDaLabel(c.labels); got != c.quer {
				t.Errorf("EscopoDaLabel(%v) = %q, esperava %q", c.labels, got, c.quer)
			}
		})
	}
}
