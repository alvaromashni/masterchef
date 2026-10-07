package collector

import (
	"fmt"
	"slices"
	"testing"
)

func TestVincular(t *testing.T) {
	issues := []IssueRef{
		{ID: "i1", Identifier: "ABC-1", Produto: "x"},
		{ID: "i2", Identifier: "ABC-2", Produto: "x",
			AttachmentURLs: []string{"https://github.com/o/x-api/pull/9/"}},
		{ID: "i12", Identifier: "ABC-12", Produto: "x"},
		{ID: "y1", Identifier: "ABC-1", Produto: "y"}, // mesmo identificador, outro produto
	}

	casos := []struct {
		nome string
		pr   PRRef
		quer []string // "issue->pr"
	}{
		{
			nome: "identificador no título",
			pr:   PRRef{ID: 1, Produto: "x", Title: "ABC-1: cria login"},
			quer: []string{"i1->1"},
		},
		{
			nome: "identificador no corpo",
			pr:   PRRef{ID: 2, Produto: "x", Title: "Login", Body: "Resolve ABC-12"},
			quer: []string{"i12->2"},
		},
		{
			nome: "branch em minúsculas (padrão do Linear)",
			pr:   PRRef{ID: 3, Produto: "x", Branch: "alvaro/abc-1-login"},
			quer: []string{"i1->3"},
		},
		{
			nome: "anexo do Linear com a URL do PR (ignora / no fim e maiúsculas)",
			pr:   PRRef{ID: 9, Produto: "x", URL: "https://github.com/O/x-api/pull/9", Title: "sem id"},
			quer: []string{"i2->9"},
		},
		{
			nome: "ABC-1 não casa dentro de ABC-12",
			pr:   PRRef{ID: 4, Produto: "x", Title: "ABC-12"},
			quer: []string{"i12->4"},
		},
		{
			nome: "várias issues no mesmo PR, sem repetir",
			pr:   PRRef{ID: 5, Produto: "x", Branch: "abc-1", Title: "ABC-1 e ABC-12", Body: "ABC-1"},
			quer: []string{"i1->5", "i12->5"},
		},
		{
			nome: "identificador inexistente não vincula",
			pr:   PRRef{ID: 6, Produto: "x", Title: "Atualiza SHA-256 e ABC-999"},
			quer: nil,
		},
		{
			nome: "só vincula issues do mesmo produto",
			pr:   PRRef{ID: 7, Produto: "y", Title: "ABC-1"},
			quer: []string{"y1->7"},
		},
		{
			nome: "título em minúsculas não vincula (regex do CONTEXT.md é maiúscula)",
			pr:   PRRef{ID: 8, Produto: "x", Title: "abc-1 login"},
			quer: nil,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			var got []string
			for _, v := range Vincular(issues, []PRRef{c.pr}) {
				got = append(got, fmt.Sprintf("%s->%d", v.IssueID, v.PRID))
			}
			slices.Sort(got)
			if !slices.Equal(got, c.quer) {
				t.Errorf("vínculos = %v, esperava %v", got, c.quer)
			}
		})
	}
}
