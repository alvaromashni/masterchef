package collector

import (
	"regexp"
	"strings"
)

// identificadorIssue casa identificadores do Linear como ABC-123
// (seção 8 do CONTEXT.md). \b evita casar no meio de palavras.
var identificadorIssue = regexp.MustCompile(`\b[A-Z][A-Z0-9]+-\d+\b`)

// IssueRef é o que o vínculo precisa saber de uma issue.
type IssueRef struct {
	ID             string
	Identifier     string // ex.: ABC-123
	Produto        string
	AttachmentURLs []string // anexos do Linear (ex.: link do PR)
}

// PRRef é o que o vínculo precisa saber de um PR.
type PRRef struct {
	ID      int64
	Produto string
	URL     string
	Branch  string
	Title   string
	Body    string // vazio para PRs que vieram do banco (o corpo não é guardado)
}

// Vinculo liga uma issue a um PR.
type Vinculo struct {
	IssueID string
	PRID    int64
}

// Vincular devolve todos os pares issue ↔ PR. Um PR está vinculado a uma
// issue do MESMO produto quando:
//
//   - o identificador da issue aparece na branch, no título ou no corpo; ou
//   - a issue tem no Linear um anexo apontando para a URL do PR.
//
// Só identificadores de issues que existem contam: "SHA-256" no texto não
// vira vínculo porque não há issue SHA-256.
func Vincular(issues []IssueRef, prs []PRRef) []Vinculo {
	// Índices para não comparar tudo com tudo: "produto|ABC-1" -> id da issue.
	porIdentificador := map[string]string{}
	for _, i := range issues {
		porIdentificador[i.Produto+"|"+i.Identifier] = i.ID
	}
	// "produto|url" -> ids de issues com anexo apontando para aquela URL.
	porAnexo := map[string][]string{}
	for _, i := range issues {
		for _, u := range i.AttachmentURLs {
			chave := i.Produto + "|" + normalizarURL(u)
			porAnexo[chave] = append(porAnexo[chave], i.ID)
		}
	}

	var vinculos []Vinculo
	vistos := map[Vinculo]bool{}
	adicionar := func(v Vinculo) {
		if !vistos[v] {
			vistos[v] = true
			vinculos = append(vinculos, v)
		}
	}

	for _, pr := range prs {
		for _, ident := range identificadoresNoPR(pr) {
			if issueID, ok := porIdentificador[pr.Produto+"|"+ident]; ok {
				adicionar(Vinculo{IssueID: issueID, PRID: pr.ID})
			}
		}
		for _, issueID := range porAnexo[pr.Produto+"|"+normalizarURL(pr.URL)] {
			adicionar(Vinculo{IssueID: issueID, PRID: pr.ID})
		}
	}
	return vinculos
}

// identificadoresNoPR acha os identificadores na branch, no título e no corpo.
//
// A branch é convertida para maiúsculas antes, porque o Linear sugere nomes
// de branch em minúsculas (ex.: "alvaro/abc-123-tela-login"). Título e corpo
// ficam como estão: lá o identificador costuma vir em maiúsculas, e
// converter tudo aumentaria os falsos positivos.
func identificadoresNoPR(pr PRRef) []string {
	texto := strings.ToUpper(pr.Branch) + "\n" + pr.Title + "\n" + pr.Body
	return identificadorIssue.FindAllString(texto, -1)
}

// normalizarURL ignora diferenças irrelevantes: maiúsculas e "/" no fim.
func normalizarURL(u string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(u)), "/")
}
