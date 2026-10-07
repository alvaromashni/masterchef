package collector

import (
	"sort"
	"strings"
)

// prefixoLabelEscopo é o prefixo das labels do Linear que dizem o escopo de
// uma issue, ex.: "scope:api" (seção 8 do CONTEXT.md).
const prefixoLabelEscopo = "scope:"

// EscopoDaLabel devolve o escopo indicado pelas labels da issue, ou "" se
// nenhuma label "scope:<nome>" existir. Issue com "" vai para "a classificar".
//
// Por enquanto (Fase 1) só a label decide. Na Fase 2 um PR vinculado passa
// a ter prioridade sobre ela.
//
// Se houver mais de uma label de escopo, escolhemos a primeira em ordem
// alfabética: o resultado fica igual a cada sync, não importa em que ordem
// o Linear devolveu as labels.
func EscopoDaLabel(labels []string) string {
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
