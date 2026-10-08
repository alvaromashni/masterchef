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
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// Documento é um trecho de Markdown pronto para a tela: ou tem HTML, ou
// tem um aviso explicando por que não há conteúdo.
type Documento struct {
	HTML    template.HTML // vazio quando há Aviso
	Aviso   string        // ex.: "produtos/x/PRODUCT.md não foi encontrado no repo central."
	Caminho string        // caminho relativo ao repo central, para mostrar na tela
}

// Decisoes é o DECISIONS.md separado em decisões, uma por seção "## ".
// Uma seção "## 2026-09-28: Título" vira Data "2026-09-28" e Titulo "Título".
type Decisoes struct {
	Caminho string
	Aviso   string        // arquivo ausente ou vazio
	Intro   template.HTML // texto antes da primeira "## " (sem o título "# ")
	Itens   []Decisao
}

// Decisao é uma seção do DECISIONS.md.
type Decisao struct {
	Data   string // vazio se o título não começa com uma data
	Titulo string
	Ancora string // id para o índice "Nesta página"
	HTML   template.HTML
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
	relativo := Caminho(slug, "PRODUCT.md")
	conteudo, aviso := l.ler(relativo)
	if aviso != "" {
		return Documento{Aviso: aviso, Caminho: relativo}
	}
	secao, ok := ExtrairSecao(conteudo, escopo)
	if !ok {
		return Documento{Aviso: fmt.Sprintf("PRODUCT.md ainda não tem a seção \"## %s\". Adicione-a no repo central.", escopo), Caminho: relativo}
	}
	if strings.TrimSpace(secao) == "" {
		return Documento{Aviso: fmt.Sprintf("Nada escrito ainda na seção \"## %s\" do PRODUCT.md.", escopo), Caminho: relativo}
	}
	html, aviso := l.renderizar(secao, relativo)
	return Documento{HTML: html, Aviso: aviso, Caminho: relativo}
}

// Decisoes lê o DECISIONS.md do produto e separa as decisões.
func (l *Leitor) Decisoes(slug string) Decisoes {
	d := Decisoes{Caminho: Caminho(slug, "DECISIONS.md")}
	conteudo, aviso := l.ler(d.Caminho)
	if aviso != "" {
		d.Aviso = aviso
		return d
	}
	if strings.TrimSpace(conteudo) == "" {
		d.Aviso = fmt.Sprintf("Nada escrito ainda em %s.", d.Caminho)
		return d
	}

	intro, secoes := separarSecoes(conteudo)
	if d.Intro, d.Aviso = l.renderizar(intro, d.Caminho); d.Aviso != "" {
		return d
	}
	for i, s := range secoes {
		item := Decisao{Titulo: s.titulo, Ancora: fmt.Sprintf("decisao-%d", i+1)}
		if m := tituloComData.FindStringSubmatch(s.titulo); m != nil {
			item.Data, item.Titulo = m[1], m[2]
		}
		if item.HTML, d.Aviso = l.renderizar(s.corpo, d.Caminho); d.Aviso != "" {
			return d
		}
		d.Itens = append(d.Itens, item)
	}
	return d
}

// Caminho é o caminho de um arquivo do produto relativo ao repo central,
// como aparece na tela (ex.: "produtos/produto-x/PRODUCT.md").
func Caminho(slug, arquivo string) string {
	return path.Join("produtos", slug, arquivo)
}

// tituloComData casa "2026-09-28: Título" (também com " - " ou " — ").
var tituloComData = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\s*[:\-–—]\s*(.+)$`)

// renderizar converte Markdown em HTML. Texto vazio vira HTML vazio.
func (l *Leitor) renderizar(markdown, relativo string) (template.HTML, string) {
	if strings.TrimSpace(markdown) == "" {
		return "", ""
	}
	var buf bytes.Buffer
	if err := l.md.Convert([]byte(markdown), &buf); err != nil {
		return "", fmt.Sprintf("Não foi possível ler o Markdown de %s: %v", relativo, err)
	}
	// template.HTML diz ao html/template "isto já é HTML seguro, não escape".
	// É seguro aqui porque o goldmark não deixa passar HTML cru.
	return template.HTML(buf.String()), ""
}

// ler devolve o conteúdo ou um aviso legível (nunca um erro).
func (l *Leitor) ler(relativo string) (string, string) {
	dados, err := os.ReadFile(filepath.Join(l.raiz, filepath.FromSlash(relativo)))
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Sprintf("%s não foi encontrado no repo central.", relativo)
	}
	if err != nil {
		return "", fmt.Sprintf("Não foi possível ler %s: %v", relativo, err)
	}
	return string(dados), ""
}

type secao struct {
	titulo string
	corpo  string
}

// separarSecoes divide o Markdown nas seções "## ". O que vem antes da
// primeira é a introdução, sem as linhas de título "# ". Como em
// ExtrairSecao, linhas dentro de blocos de código não contam como título.
func separarSecoes(markdown string) (string, []secao) {
	var intro []string
	var secoes []secao
	var corpo []string
	emCodigo := false

	fechar := func() {
		if len(secoes) > 0 {
			secoes[len(secoes)-1].corpo = strings.TrimSpace(strings.Join(corpo, "\n"))
		}
		corpo = nil
	}
	for _, linha := range strings.Split(markdown, "\n") {
		if strings.HasPrefix(strings.TrimSpace(linha), "```") {
			emCodigo = !emCodigo
		}
		switch {
		case !emCodigo && strings.HasPrefix(linha, "## "):
			fechar()
			secoes = append(secoes, secao{titulo: strings.TrimSpace(strings.TrimPrefix(linha, "## "))})
		case len(secoes) == 0:
			if emCodigo || !strings.HasPrefix(linha, "# ") {
				intro = append(intro, linha)
			}
		default:
			corpo = append(corpo, linha)
		}
	}
	fechar()
	return strings.TrimSpace(strings.Join(intro, "\n")), secoes
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
