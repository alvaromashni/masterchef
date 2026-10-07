// Package escopo tem a regra que decide em qual(is) coluna(s) da matriz
// uma issue aparece (seção 8 do CONTEXT.md). Fica num pacote próprio porque
// o coletor e a parte web usam a mesma regra.
package escopo

import (
	"sort"
	"strings"
)

// prefixoLabelEscopo é o prefixo das labels do Linear que dizem o escopo de
// uma issue, ex.: "scope:api" (seção 8 do CONTEXT.md).
const prefixoLabelEscopo = "scope:"

// DaLabel devolve o escopo indicado pelas labels da issue, ou "" se
// nenhuma label "scope:<nome>" existir. Issue com "" vai para "a classificar".
//
// Se houver mais de uma label de escopo, escolhemos a primeira em ordem
// alfabética: o resultado fica igual a cada sync, não importa em que ordem
// o Linear devolveu as labels.
func DaLabel(labels []string) string {
	var escopos []string
	for _, l := range labels {
		// Comparação sem diferenciar maiúsculas: "Scope:API" vale como "scope:api".
		l = strings.ToLower(strings.TrimSpace(l))
		if nome, ok := strings.CutPrefix(l, prefixoLabelEscopo); ok && nome != "" {
			escopos = append(escopos, strings.TrimSpace(nome))
		}
	}
	if len(escopos) == 0 {
		return ""
	}
	sort.Strings(escopos)
	return escopos[0]
}

// DaIssue aplica a regra completa de escopo de uma issue:
//
//  1. se a issue tem PRs vinculados, os escopos são os dos repos desses PRs
//     (uma issue com PRs em dois escopos aparece nas duas células);
//  2. senão, se tem label scope:<nome>, usa a label;
//  3. senão, devolve [""], que significa "a classificar".
//
// escoposDosPRs pode ter repetidos (dois PRs no mesmo repo); o resultado não.
func DaIssue(escoposDosPRs []string, scopeLabel string) []string {
	if len(escoposDosPRs) > 0 {
		var unicos []string
		vistos := map[string]bool{}
		for _, e := range escoposDosPRs {
			if !vistos[e] {
				vistos[e] = true
				unicos = append(unicos, e)
			}
		}
		sort.Strings(unicos)
		return unicos
	}
	return []string{scopeLabel}
}
