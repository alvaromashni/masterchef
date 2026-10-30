package github

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
)

// PastaDocs é a pasta, na raiz de cada repo, de onde o painel lê o
// contexto do projeto (decisão do Alvinho em 2026-10-30).
const PastaDocs = "docs"

// Doc é um arquivo Markdown dentro da pasta docs/ de um repo.
type Doc struct {
	Caminho string // relativo à raiz do repo, ex.: "docs/arquitetura/banco.md"
	Tamanho int
}

// ListDocs devolve os arquivos .md da pasta docs/ (e subpastas) da branch
// principal do repo, em ordem alfabética. Repo sem docs/ devolve lista vazia.
//
// São duas chamadas: uma para descobrir a branch principal e outra para a
// "árvore" inteira do repo (o endpoint git/trees com recursive=1 lista
// todos os arquivos de uma vez, sem precisar descer pasta por pasta).
func (c *Client) ListDocs(ctx context.Context, repo string) ([]Doc, error) {
	var info struct {
		DefaultBranch string `json:"default_branch"`
	}
	if _, err := c.get(ctx, fmt.Sprintf("%s/repos/%s", c.baseURL, repo), &info); err != nil {
		return nil, fmt.Errorf("lendo dados do repo %s: %w", repo, err)
	}

	var arvore struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"` // blob = arquivo, tree = pasta
			Size int    `json:"size"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	endereco := fmt.Sprintf("%s/repos/%s/git/trees/%s?recursive=1", c.baseURL, repo, url.PathEscape(info.DefaultBranch))
	if _, err := c.get(ctx, endereco, &arvore); err != nil {
		return nil, fmt.Errorf("listando arquivos de %s: %w", repo, err)
	}
	if arvore.Truncated {
		// Só acontece em repos enormes (mais de 100 mil arquivos); avisamos
		// no log em vez de falhar, porque o que veio ainda serve.
		c.logger.Warn("lista de arquivos do GitHub veio truncada", "repo", repo)
	}

	var docs []Doc
	for _, item := range arvore.Tree {
		if item.Type == "blob" && strings.HasPrefix(item.Path, PastaDocs+"/") && ehMarkdown(item.Path) {
			docs = append(docs, Doc{Caminho: item.Path, Tamanho: item.Size})
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Caminho < docs[j].Caminho })
	return docs, nil
}

// ReadDoc devolve o conteúdo de um arquivo da branch principal do repo.
// A API de "contents" devolve o arquivo em base64 dentro do JSON.
func (c *Client) ReadDoc(ctx context.Context, repo, caminho string) ([]byte, error) {
	var arquivo struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	endereco := fmt.Sprintf("%s/repos/%s/contents/%s", c.baseURL, repo, escaparCaminho(caminho))
	if _, err := c.get(ctx, endereco, &arquivo); err != nil {
		return nil, fmt.Errorf("lendo %s de %s: %w", caminho, repo, err)
	}
	if arquivo.Encoding != "base64" {
		return nil, fmt.Errorf("lendo %s de %s: codificação %q inesperada", caminho, repo, arquivo.Encoding)
	}
	// O GitHub quebra o base64 em linhas de 60 caracteres; o decodificador
	// padrão não aceita as quebras, então tiramos antes.
	dados, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(arquivo.Content, "\n", ""))
	if err != nil {
		return nil, fmt.Errorf("decodificando %s de %s: %w", caminho, repo, err)
	}
	return dados, nil
}

func ehMarkdown(caminho string) bool {
	ext := strings.ToLower(path.Ext(caminho))
	return ext == ".md" || ext == ".markdown"
}

// escaparCaminho escapa cada parte do caminho, mantendo as barras.
func escaparCaminho(caminho string) string {
	partes := strings.Split(caminho, "/")
	for i, p := range partes {
		partes[i] = url.PathEscape(p)
	}
	return strings.Join(partes, "/")
}
