package contexto

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alvaromashni/masterchef/internal/github"
)

// fonteFalsa guarda arquivos em memória e conta as chamadas.
type fonteFalsa struct {
	arquivos      map[string]string // caminho -> conteúdo
	listas, leits int
	erro          error
}

func (f *fonteFalsa) ListDocs(_ context.Context, _ string) ([]github.Doc, error) {
	f.listas++
	if f.erro != nil {
		return nil, f.erro
	}
	var docs []github.Doc
	for c := range f.arquivos {
		docs = append(docs, github.Doc{Caminho: c})
	}
	return docs, nil
}

func (f *fonteFalsa) ReadDoc(_ context.Context, _, caminho string) ([]byte, error) {
	f.leits++
	return []byte(f.arquivos[caminho]), nil
}

func link(repo, caminho string) string { return "/doc/" + repo + "/" + caminho }

func novoLeitorFalso() (*Leitor, *fonteFalsa, *time.Time) {
	f := &fonteFalsa{arquivos: map[string]string{
		"docs/visao.md":             "# Visão do Tinker\n\nVeja a [arquitetura](arquitetura/banco.md#tabelas), o [site](https://x.dev) e o [código](../src/main.go).\n",
		"docs/arquitetura/banco.md": "Sem título aqui.\n\n<script>alert(1)</script>\n",
	}}
	agora := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	l := NewLeitor(f)
	l.agora = func() time.Time { return agora }
	return l, f, &agora
}

func TestPagina_TituloELinks(t *testing.T) {
	l, _, _ := novoLeitorFalso()
	p, ok, err := l.Pagina(context.Background(), "o/tinker", "docs/visao.md", link)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if p.Titulo != "Visão do Tinker" {
		t.Errorf("Titulo = %q", p.Titulo)
	}
	html := string(p.HTML)
	for _, trecho := range []string{
		`href="/doc/o/tinker/docs/arquitetura/banco.md"`, // link relativo para outro doc vira link do painel
		`href="https://x.dev"`,                           // link externo fica igual
		`href="../src/main.go"`,                          // arquivo fora do índice fica igual
	} {
		if !strings.Contains(html, trecho) {
			t.Errorf("HTML não contém %s:\n%s", trecho, html)
		}
	}
}

func TestPagina_SemTituloUsaNomeESemHTMLCru(t *testing.T) {
	l, _, _ := novoLeitorFalso()
	p, _, err := l.Pagina(context.Background(), "o/tinker", "docs/arquitetura/banco.md", link)
	if err != nil {
		t.Fatal(err)
	}
	if p.Titulo != "banco" {
		t.Errorf("Titulo = %q", p.Titulo)
	}
	if strings.Contains(string(p.HTML), "<script>") {
		t.Errorf("HTML cru passou: %s", p.HTML)
	}
}

func TestPagina_ForaDoIndiceNaoBusca(t *testing.T) {
	l, f, _ := novoLeitorFalso()
	_, ok, err := l.Pagina(context.Background(), "o/tinker", ".env", link)
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, esperava não encontrado", ok, err)
	}
	if f.leits != 0 {
		t.Errorf("leu o arquivo %d vezes, esperava 0", f.leits)
	}
}

func TestCache_ExpiraDepoisDaValidade(t *testing.T) {
	l, f, agora := novoLeitorFalso()
	ctx := context.Background()
	for range 3 {
		if _, _, err := l.Pagina(ctx, "o/tinker", "docs/visao.md", link); err != nil {
			t.Fatal(err)
		}
	}
	if f.listas != 1 || f.leits != 1 {
		t.Fatalf("listas=%d leituras=%d, esperava 1 e 1 (cache)", f.listas, f.leits)
	}

	*agora = agora.Add(Validade)
	if _, _, err := l.Pagina(ctx, "o/tinker", "docs/visao.md", link); err != nil {
		t.Fatal(err)
	}
	if f.listas != 2 || f.leits != 2 {
		t.Fatalf("listas=%d leituras=%d, esperava 2 e 2 depois de expirar", f.listas, f.leits)
	}
}

func TestIndice_PastasENomes(t *testing.T) {
	l, _, _ := novoLeitorFalso()
	docs, err := l.Indice(context.Background(), "o/tinker")
	if err != nil {
		t.Fatal(err)
	}
	achou := false
	for _, d := range docs {
		if d.Caminho == "docs/arquitetura/banco.md" {
			achou = d.Pasta == "arquitetura" && d.Nome == "banco"
		}
	}
	if !achou {
		t.Errorf("docs = %+v", docs)
	}
}

func TestIndice_ErroNaoEntraNoCache(t *testing.T) {
	l, f, _ := novoLeitorFalso()
	f.erro = errors.New("HTTP 404")
	if _, err := l.Indice(context.Background(), "o/tinker"); err == nil {
		t.Fatal("esperava erro")
	}
	f.erro = nil
	if _, err := l.Indice(context.Background(), "o/tinker"); err != nil {
		t.Fatalf("depois do erro: %v", err)
	}
}
