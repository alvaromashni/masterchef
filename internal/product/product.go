// Package product lê os Markdown que o dono escreve no repo central
// (PRODUCT.md e DECISIONS.md) e os transforma em HTML para as telas.
//
// Formato esperado (seção 11 do CONTEXT.md):
//
//	<central_repo_path>/produtos/<slug>/PRODUCT.md     uma seção "## <escopo>" por escopo
//	<central_repo_path>/produtos/<slug>/DECISIONS.md
//
// Arquivo ou seção ausente nunca é erro: a tela mostra um aviso amigável.
package product

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// Documento é um trecho de Markdown pronto para a tela: ou tem HTML, ou
// tem um aviso explicando por que não há conteúdo.
type Documento struct {
	HTML  template.HTML // vazio quando há Aviso
	Aviso string        // ex.: "Arquivo ... não encontrado"
}

// Leitor sabe onde fica o repo central.
type Leitor struct {
	raiz string // central_repo_path
	md   goldmark.Markdown
}

// NewLeitor cria um leitor para o repo central em raiz.
func NewLeitor(raiz string) *Leitor {
	return &Leitor{
		raiz: raiz,
		// GFM = o Markdown do GitHub (tabelas, listas de tarefa, links
		// automáticos). HTML cru dentro do Markdown NÃO é renderizado
		// (padrão do goldmark), o que evita injetar scripts na página.
		md: goldmark.New(goldmark.WithExtensions(extension.GFM)),
	}
}

// SecaoDoEscopo devolve a seção "## <escopo>" do PRODUCT.md do produto.
func (l *Leitor) SecaoDoEscopo(slug, escopo string) Documento {
	caminho := l.caminho(slug, "PRODUCT.md")
	conteudo, aviso := lerArquivo(caminho)
	if aviso != "" {
		return Documento{Aviso: aviso}
	}
	secao, ok := ExtrairSecao(conteudo, escopo)
	if !ok {
		return Documento{Aviso: fmt.Sprintf("O PRODUCT.md não tem uma seção \"## %s\" (%s).", escopo, caminho)}
	}
	return l.renderizar(secao, caminho)
}

// Decisoes devolve o DECISIONS.md inteiro do produto.
func (l *Leitor) Decisoes(slug string) Documento {
	caminho := l.caminho(slug, "DECISIONS.md")
	conteudo, aviso := lerArquivo(caminho)
	if aviso != "" {
		return Documento{Aviso: aviso}
	}
	return l.renderizar(conteudo, caminho)
}

func (l *Leitor) caminho(slug, arquivo string) string {
	return filepath.Join(l.raiz, "produtos", slug, arquivo)
}

func (l *Leitor) renderizar(markdown, caminho string) Documento {
	if strings.TrimSpace(markdown) == "" {
		return Documento{Aviso: fmt.Sprintf("Nada escrito ainda em %s.", caminho)}
	}
	var buf bytes.Buffer
	if err := l.md.Convert([]byte(markdown), &buf); err != nil {
		return Documento{Aviso: fmt.Sprintf("Não foi possível ler o Markdown de %s: %v", caminho, err)}
	}
	// template.HTML diz ao html/template "isto já é HTML seguro, não escape".
	// É seguro aqui porque o goldmark não deixa passar HTML cru.
	return Documento{HTML: template.HTML(buf.String())}
}

// lerArquivo devolve o conteúdo ou um aviso legível (nunca um erro).
func lerArquivo(caminho string) (string, string) {
	dados, err := os.ReadFile(caminho)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Sprintf("Arquivo %s não encontrado. Crie-o no repo central para ver o conteúdo aqui.", caminho)
	}
	if err != nil {
		return "", fmt.Sprintf("Não foi possível ler %s: %v", caminho, err)
	}
	return string(dados), ""
}

// ExtrairSecao devolve o texto da seção "## <titulo>" (sem a linha do
// título), até o próximo "## " ou o fim do arquivo. Subtítulos "###"
// fazem parte da seção. O título é comparado sem diferenciar maiúsculas
// e ignorando espaços nas pontas.
//
// Linhas dentro de blocos de código (```) não contam como título, para
// que um exemplo de Markdown dentro do texto não corte a seção.
func ExtrairSecao(markdown, titulo string) (string, bool) {
	alvo := strings.ToLower(strings.TrimSpace(titulo))
	var dentro, achou, emCodigo bool
	var linhas []string

	for _, linha := range strings.Split(markdown, "\n") {
		if strings.HasPrefix(strings.TrimSpace(linha), "```") {
			emCodigo = !emCodigo
		}
		if !emCodigo && strings.HasPrefix(linha, "## ") {
			nome := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(linha, "## ")))
			if dentro {
				break // começou a próxima seção
			}
			if nome == alvo {
				dentro, achou = true, true
				continue
			}
		}
		if dentro {
			linhas = append(linhas, linha)
		}
	}
	return strings.TrimSpace(strings.Join(linhas, "\n")), achou
}
