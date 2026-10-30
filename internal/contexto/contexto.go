// Package contexto lê a pasta docs/ dos repos de cada projeto pelo GitHub
// e transforma os Markdown em HTML para a página do projeto.
//
// Nada é gravado no banco: os arquivos são buscados quando a página abre e
// ficam guardados em memória por alguns minutos (cache), para não gastar o
// limite de requisições do GitHub a cada clique.
package contexto

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/alvaromashni/masterchef/internal/github"
)

// Validade é quanto tempo uma lista ou um arquivo fica no cache.
const Validade = 5 * time.Minute

// Fonte é de onde vêm os arquivos. Na prática é o *github.Client; nos
// testes é uma fonte falsa. (É o mesmo papel de uma interface de
// repositório no Spring: o Leitor não sabe que do outro lado há HTTP.)
type Fonte interface {
	ListDocs(ctx context.Context, repo string) ([]github.Doc, error)
	ReadDoc(ctx context.Context, repo, caminho string) ([]byte, error)
}

// Doc é um item do índice de documentos de um repo.
type Doc struct {
	Caminho string // como no repo, ex.: "docs/arquitetura/banco.md"
	Pasta   string // subpasta dentro de docs/, ex.: "arquitetura" ("" = raiz)
	Nome    string // nome do arquivo sem extensão, ex.: "banco"
}

// Pagina é um documento pronto para a tela.
type Pagina struct {
	Caminho string
	Titulo  string // primeiro "# " do arquivo, ou o nome do arquivo
	HTML    template.HTML
}

// LinkDoc monta o endereço de um documento no painel. Fica do lado do
// chamador (pacote web) para este pacote não conhecer as rotas.
type LinkDoc func(repo, caminho string) string

// Leitor busca, guarda em cache e renderiza os documentos.
type Leitor struct {
	fonte Fonte
	agora func() time.Time // trocado nos testes para simular o tempo passando

	mu        sync.Mutex // o cache é usado por várias requisições ao mesmo tempo
	indices   map[string]entrada[[]Doc]
	conteudos map[string]entrada[[]byte]
}

// entrada é um valor no cache com a hora em que foi buscado. O [T] é um
// tipo genérico, como List<T> no Java.
type entrada[T any] struct {
	valor  T
	buscou time.Time
}

// NewLeitor cria um leitor que busca os arquivos em fonte.
func NewLeitor(fonte Fonte) *Leitor {
	return &Leitor{
		fonte:     fonte,
		agora:     time.Now,
		indices:   map[string]entrada[[]Doc]{},
		conteudos: map[string]entrada[[]byte]{},
	}
}

// Indice lista os documentos da pasta docs/ do repo.
func (l *Leitor) Indice(ctx context.Context, repo string) ([]Doc, error) {
	l.mu.Lock()
	e, ok := l.indices[repo]
	l.mu.Unlock()
	if ok && l.agora().Sub(e.buscou) < Validade {
		return e.valor, nil
	}

	arquivos, err := l.fonte.ListDocs(ctx, repo)
	if err != nil {
		return nil, err
	}
	docs := make([]Doc, 0, len(arquivos))
	for _, a := range arquivos {
		docs = append(docs, novoDoc(a.Caminho))
	}

	l.mu.Lock()
	l.indices[repo] = entrada[[]Doc]{valor: docs, buscou: l.agora()}
	l.mu.Unlock()
	return docs, nil
}

// Pagina busca um documento e o converte em HTML. Só aceita caminhos que
// estão no índice do repo: assim a URL não vira uma porta para ler
// qualquer arquivo do repositório.
func (l *Leitor) Pagina(ctx context.Context, repo, caminho string, link LinkDoc) (Pagina, bool, error) {
	docs, err := l.Indice(ctx, repo)
	if err != nil {
		return Pagina{}, false, err
	}
	if !contem(docs, caminho) {
		return Pagina{}, false, nil
	}

	chave := repo + "\x00" + caminho
	l.mu.Lock()
	e, ok := l.conteudos[chave]
	l.mu.Unlock()
	if !ok || l.agora().Sub(e.buscou) >= Validade {
		dados, err := l.fonte.ReadDoc(ctx, repo, caminho)
		if err != nil {
			return Pagina{}, false, err
		}
		e = entrada[[]byte]{valor: dados, buscou: l.agora()}
		l.mu.Lock()
		l.conteudos[chave] = e
		l.mu.Unlock()
	}

	html, err := renderizar(e.valor, func(destino string) string {
		alvo, ok := resolverLink(caminho, destino)
		if !ok || !contem(docs, alvo) {
			return destino
		}
		return link(repo, alvo)
	})
	if err != nil {
		return Pagina{}, false, fmt.Errorf("convertendo %s de %s: %w", caminho, repo, err)
	}
	titulo := tituloDoMarkdown(e.valor)
	if titulo == "" {
		titulo = novoDoc(caminho).Nome
	}
	return Pagina{Caminho: caminho, Titulo: titulo, HTML: html}, true, nil
}

func novoDoc(caminho string) Doc {
	dentro := strings.TrimPrefix(caminho, github.PastaDocs+"/")
	pasta, arquivo := path.Split(dentro)
	return Doc{
		Caminho: caminho,
		Pasta:   strings.TrimSuffix(pasta, "/"),
		Nome:    strings.TrimSuffix(arquivo, path.Ext(arquivo)),
	}
}

func contem(docs []Doc, caminho string) bool {
	for _, d := range docs {
		if d.Caminho == caminho {
			return true
		}
	}
	return false
}

// resolverLink transforma um link relativo escrito dentro de um documento
// (ex.: "../visao.md#metas") no caminho do arquivo no repo
// ("docs/visao.md"). Links absolutos (http://, /algo, #ancora) ficam como estão.
func resolverLink(docAtual, destino string) (string, bool) {
	if destino == "" || strings.HasPrefix(destino, "#") || strings.HasPrefix(destino, "/") || strings.Contains(destino, "://") || strings.HasPrefix(destino, "mailto:") {
		return "", false
	}
	if i := strings.IndexAny(destino, "#?"); i >= 0 {
		destino = destino[:i]
	}
	return path.Join(path.Dir(docAtual), destino), true
}

// tituloDoMarkdown devolve o texto do primeiro título "# " (fora de blocos de código).
func tituloDoMarkdown(md []byte) string {
	emCodigo := false
	for _, linha := range strings.Split(string(md), "\n") {
		if strings.HasPrefix(strings.TrimSpace(linha), "```") {
			emCodigo = !emCodigo
		}
		if !emCodigo && strings.HasPrefix(linha, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(linha, "# "))
		}
	}
	return ""
}

// renderizar converte Markdown (GFM) em HTML, trocando o destino de cada
// link por reescrever(destino). HTML cru dentro do Markdown não é
// renderizado (padrão do goldmark), o que evita injetar scripts na página.
func renderizar(md []byte, reescrever func(string) string) (template.HTML, error) {
	conversor := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithASTTransformers(
			util.Prioritized(trocaLinks(reescrever), 100),
		)),
	)
	var buf bytes.Buffer
	if err := conversor.Convert(md, &buf); err != nil {
		return "", err
	}
	// Seguro marcar como HTML confiável pelo mesmo motivo do pacote product.
	return template.HTML(buf.String()), nil
}

// trocaLinks é um passo extra do goldmark: depois de ler o Markdown, ele
// percorre a árvore (AST) e reescreve o destino de cada link.
type trocaLinks func(string) string

func (f trocaLinks) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	ast.Walk(doc, func(n ast.Node, entrando bool) (ast.WalkStatus, error) {
		if link, ok := n.(*ast.Link); ok && entrando {
			link.Destination = []byte(f(string(link.Destination)))
		}
		return ast.WalkContinue, nil
	})
}
