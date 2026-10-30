package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/github"
	"github.com/alvaromashni/masterchef/internal/store"
)

// docsFalsos simula a pasta docs/ no GitHub: o repo o/x-api tem dois
// documentos e o o/x-front responde com erro.
type docsFalsos struct{}

var arquivosFalsos = map[string]string{
	"docs/visao.md":             "# Visão do Produto X\n\nVeja o [banco](arquitetura/banco.md).\n",
	"docs/arquitetura/banco.md": "# Banco\n\nPostgres.\n",
}

func (docsFalsos) ListDocs(_ context.Context, repo string) ([]github.Doc, error) {
	if repo != "o/x-api" {
		return nil, errors.New("HTTP 404")
	}
	return []github.Doc{{Caminho: "docs/arquitetura/banco.md"}, {Caminho: "docs/visao.md"}}, nil
}

func (docsFalsos) ReadDoc(_ context.Context, _, caminho string) ([]byte, error) {
	return []byte(arquivosFalsos[caminho]), nil
}

func TestMontarRoadmap_SeparaPorTipoDeEstado(t *testing.T) {
	base := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	p := config.Produto{Slug: "px", Escopos: config.Escopos{{Nome: "api"}}}
	d := dadosPainel{Issues: []store.Issue{
		{ID: "1", ProductSlug: "px", StateType: "completed", UpdatedAt: base.Add(-time.Hour)},
		{ID: "2", ProductSlug: "px", StateType: "completed", UpdatedAt: base},
		{ID: "3", ProductSlug: "px", StateType: "started", ScopeLabel: "api"},
		{ID: "4", ProductSlug: "px", StateType: "backlog", UpdatedAt: base.Add(-48 * time.Hour)},
		{ID: "5", ProductSlug: "px", StateType: "unstarted", UpdatedAt: base},
		{ID: "6", ProductSlug: "px", StateType: "canceled"},
		{ID: "7", ProductSlug: "outro", StateType: "started"},
	}}

	r := montarRoadmap(p, d)
	if r.Total != 5 || r.Percentual != 40 {
		t.Errorf("Total=%d Percentual=%d, esperava 5 e 40", r.Total, r.Percentual)
	}
	if len(r.Feito) != 2 || r.Feito[0].ID != "2" {
		t.Errorf("Feito = %+v (mais recente primeiro)", r.Feito)
	}
	if len(r.EmAndamento) != 1 || len(r.EmAndamento[0].Escopos) != 1 || r.EmAndamento[0].Escopos[0] != "api" {
		t.Errorf("EmAndamento = %+v", r.EmAndamento)
	}
	// Todo (unstarted) antes do backlog, mesmo sendo mais novo.
	if len(r.AFazer) != 2 || r.AFazer[0].ID != "5" || r.AFazer[1].ID != "4" {
		t.Errorf("AFazer = %+v", r.AFazer)
	}
}

func TestRoadmap_FeitoVisivelLimita(t *testing.T) {
	r := Roadmap{Feito: make([]ItemRoadmap, feitasVisiveis+3)}
	if len(r.FeitoVisivel()) != feitasVisiveis || len(r.FeitoResto()) != 3 {
		t.Fatalf("visível=%d resto=%d", len(r.FeitoVisivel()), len(r.FeitoResto()))
	}
	if len((Roadmap{}).FeitoVisivel()) != 0 {
		t.Fatal("roadmap vazio deveria ter 0 feitas visíveis")
	}
}

func get(t *testing.T, h http.Handler, caminho string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", caminho, nil))
	return rec
}

func TestPaginaProjetos(t *testing.T) {
	rec := get(t, novoServidor(t), "/projetos")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	for _, trecho := range []string{"<h1>Projetos</h1>", `href="/p/produto-x"`, "Produto Y", `aria-current="page">Projetos`} {
		if !strings.Contains(rec.Body.String(), trecho) {
			t.Errorf("página não contém %q", trecho)
		}
	}
}

func TestPaginaProjeto_RoadmapEContexto(t *testing.T) {
	rec := get(t, novoServidor(t), "/p/produto-x")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	corpo := rec.Body.String()
	for _, trecho := range []string{
		"Roadmap", "Contexto",
		`href="/p/produto-x/docs/api/visao.md"`,
		`href="/p/produto-x/docs/api/arquitetura/banco.md"`,
		"arquitetura/",
		"Não foi possível ler a pasta docs/ no GitHub: HTTP 404", // o repo do front falhou, a página não
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("página não contém %q", trecho)
		}
	}
}

func TestPaginaDoc(t *testing.T) {
	h := novoServidor(t)
	rec := get(t, h, "/p/produto-x/docs/api/visao.md")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	corpo := rec.Body.String()
	for _, trecho := range []string{
		"<title>Visão do Produto X / Produto X | masterchef</title>",
		`<h1>Visão do Produto X</h1>`,
		`href="/p/produto-x/docs/api/arquitetura/banco.md"`, // link relativo reescrito
		`aria-current="page">visao</a>`,
		"https://github.com/o/x-api/blob/HEAD/docs/visao.md",
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("página não contém %q", trecho)
		}
	}

	for _, caminho := range []string{
		"/p/produto-x/docs/api/nao-existe.md", // fora do índice
		"/p/produto-x/docs/infra/visao.md",    // escopo que o produto não tem
		"/p/nada/docs/api/visao.md",           // produto que não existe
	} {
		if rec := get(t, h, caminho); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, esperava 404", caminho, rec.Code)
		}
	}
}
