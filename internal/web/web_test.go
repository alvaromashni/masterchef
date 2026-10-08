package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/store"
)

func novoServidor(t *testing.T) http.Handler {
	t.Helper()
	h, _ := novoServidorComStore(t)
	return h
}

func novoServidorComStore(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(filepath.Join(t.TempDir(), "teste.db"), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	cfg := &config.Config{
		CentralRepoPath: filepath.Join("..", "product", "testdata", "central"),
		Produtos: []config.Produto{
			{Nome: "Produto X", Slug: "produto-x", Escopos: config.Escopos{{Nome: "api"}, {Nome: "front"}}},
			{Nome: "Produto Y", Slug: "produto-y", Escopos: config.Escopos{{Nome: "api"}}},
		},
	}
	s, err := New(cfg, st, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s.Handler(), st
}

func TestIndex(t *testing.T) {
	rec := httptest.NewRecorder()
	novoServidor(t).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperava 200", rec.Code)
	}
	corpo := rec.Body.String()
	for _, trecho := range []string{"<h1>Matriz</h1>", "htmx.org", "/static/painel.css", "Produto X", "a classificar", "nenhum sync ainda"} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("página não contém %q", trecho)
		}
	}
}

func TestCaminhoInexistenteDa404(t *testing.T) {
	rec := httptest.NewRecorder()
	novoServidor(t).ServeHTTP(rec, httptest.NewRequest("GET", "/nao-existe", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, esperava 404", rec.Code)
	}
}

func TestArquivosEstaticos(t *testing.T) {
	h := novoServidor(t)
	// O CSS e as fontes que ele referencia precisam estar embutidos no binário.
	for _, caminho := range []string{"/static/painel.css", "/static/fontes/schibsted-grotesk-latin-wght-normal.woff2", "/static/fontes/ibm-plex-mono-latin-400-normal.woff2", "/static/fontes/ibm-plex-mono-latin-500-normal.woff2"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", caminho, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, esperava 200", caminho, rec.Code)
		}
	}
}

func TestFilaDeReviewVazia(t *testing.T) {
	rec := httptest.NewRecorder()
	novoServidor(t).ServeHTTP(rec, httptest.NewRequest("GET", "/prs", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Nenhum PR aguardando review") {
		t.Errorf("status = %d; corpo sem a mensagem de fila vazia", rec.Code)
	}
}

func TestMudancas_AbrirNaoMarcaComoVisto(t *testing.T) {
	h, st := novoServidorComStore(t)
	ctx := context.Background()
	quando := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	err := st.SalvarSync(ctx, store.SyncResultado{Iniciado: quando, Eventos: []store.Evento{
		{OccurredAt: quando, ProductSlug: "produto-x", Scope: "api", Kind: store.EventoPRMergeado, Ref: "o/r#1", Summary: "Mergeado: login", URL: "u"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ { // abrir duas vezes: continua lá
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/mudancas", nil))
		corpo := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(corpo, "PR mergeado") || !strings.Contains(corpo, ">login<") || !strings.Contains(corpo, "2026-10-07T10:00:00Z") {
			t.Fatalf("abertura %d: status %d, evento ou data ausente no corpo", i+1, rec.Code)
		}
	}
	if v, _ := st.UltimaVisita(ctx); !v.IsZero() {
		t.Errorf("abrir a página não deveria gravar last_visit (gravou %v)", v)
	}
}

func TestMudancas_MarcarComoVisto(t *testing.T) {
	h, st := novoServidorComStore(t)
	ctx := context.Background()
	ate := "2026-10-07T10:00:00Z"

	marcar := func(valor string, htmx bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/mudancas/visto", strings.NewReader("ate="+valor))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := marcar(ate, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Tudo marcado como visto") {
		t.Fatalf("htmx: status %d, corpo %q", rec.Code, rec.Body.String())
	}
	if v, _ := st.UltimaVisita(ctx); v.Format(time.RFC3339) != ate {
		t.Errorf("last_visit = %v, esperava %s", v, ate)
	}

	// Um formulário antigo não volta last_visit no tempo.
	if rec := marcar("2026-10-01T00:00:00Z", false); rec.Code != http.StatusSeeOther {
		t.Errorf("sem htmx deveria redirecionar (303), veio %d", rec.Code)
	}
	if v, _ := st.UltimaVisita(ctx); v.Format(time.RFC3339) != ate {
		t.Errorf("last_visit voltou no tempo para %v", v)
	}

	if rec := marcar("ontem", true); rec.Code != http.StatusBadRequest {
		t.Errorf("data inválida deveria dar 400, veio %d", rec.Code)
	}
}

func TestPaginasDeEscopoEDecisoes(t *testing.T) {
	h, st := novoServidorComStore(t)
	quando := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	err := st.SalvarSync(context.Background(), store.SyncResultado{
		Iniciado: quando,
		Issues: []store.Issue{
			{ID: "1", Identifier: "X-1", ProductSlug: "produto-x", Title: "Login", StateName: "In Progress", StateType: "started", ScopeLabel: "api", UpdatedAt: quando},
			{ID: "2", Identifier: "X-2", ProductSlug: "produto-x", Title: "Sem escopo", StateName: "Todo", StateType: "unstarted", UpdatedAt: quando},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	casos := []struct {
		caminho string
		status  int
		contem  []string
		naoTem  []string
	}{
		{"/p/produto-x/api", 200, []string{"Produto X / api", "X-1", "In Progress", "Login com token", "Nenhum PR aberto"}, []string{"X-2"}},
		{"/p/produto-x/front", 200, []string{"## isto não é um título", "Nenhuma issue"}, nil},
		{"/p/produto-x/a-classificar", 200, []string{"X-2", "Issues sem escopo não têm seção"}, []string{"X-1"}},
		{"/p/produto-x/decisoes", 200, []string{"SQLite em vez de Postgres"}, []string{"<script>alert"}},
		{"/p/produto-y/api", 200, []string{"PRODUCT.md"}, nil}, // arquivo ausente: aviso, não erro
		{"/p/produto-y/decisoes", 200, []string{"DECISIONS.md"}, nil},
		{"/p/produto-x/infra-que-nao-existe", 404, nil, nil},
		{"/p/produto-z/api", 404, nil, nil},
		{"/p/produto-z/decisoes", 404, nil, nil},
	}
	for _, c := range casos {
		t.Run(c.caminho, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", c.caminho, nil))
			if rec.Code != c.status {
				t.Fatalf("status = %d, esperava %d", rec.Code, c.status)
			}
			corpo := rec.Body.String()
			for _, trecho := range c.contem {
				if !strings.Contains(corpo, trecho) {
					t.Errorf("página não contém %q", trecho)
				}
			}
			for _, trecho := range c.naoTem {
				if strings.Contains(corpo, trecho) {
					t.Errorf("página não deveria conter %q", trecho)
				}
			}
		})
	}
}

func TestMatrizLigaParaAsPaginas(t *testing.T) {
	rec := httptest.NewRecorder()
	novoServidor(t).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	for _, link := range []string{`href="/p/produto-x/api"`, `href="/p/produto-x/a-classificar"`, `href="/p/produto-x/decisoes"`} {
		if !strings.Contains(rec.Body.String(), link) {
			t.Errorf("matriz sem o link %s", link)
		}
	}
}

func TestGuiaETopo(t *testing.T) {
	h, st := novoServidorComStore(t)
	quando := time.Now().UTC().Add(-time.Hour)
	err := st.SalvarSync(context.Background(), store.SyncResultado{
		Iniciado: quando,
		PRs: []store.PR{
			{ID: 1, Repo: "o/api", Number: 1, ProductSlug: "produto-x", Scope: "api", Title: "PR um", State: "open", RiskLevel: "baixo", CreatedAt: quando, UpdatedAt: quando},
			{ID: 2, Repo: "o/api", Number: 2, ProductSlug: "produto-x", Scope: "api", Title: "PR dois", State: "open", RiskLevel: "alto", CreatedAt: quando, UpdatedAt: quando},
			{ID: 3, Repo: "o/api", Number: 3, ProductSlug: "produto-x", Scope: "api", Title: "Rascunho", State: "open", Draft: true, RiskLevel: "baixo", CreatedAt: quando, UpdatedAt: quando},
		},
		Eventos: []store.Evento{{OccurredAt: quando, ProductSlug: "produto-x", Scope: "api", Kind: store.EventoPRAberto, Ref: "o/api#1", Summary: "Aberto: PR um", URL: "u"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	get := func(caminho string) string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", caminho, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", caminho, rec.Code)
		}
		return rec.Body.String()
	}

	guia := get("/guia")
	for _, trecho := range []string{"Guia de UI", "Cor só quando significa algo", `href="/guia" aria-current="page"`} {
		if !strings.Contains(guia, trecho) {
			t.Errorf("guia sem %q", trecho)
		}
	}
	// Contadores das abas: 1 evento não visto e 2 PRs na fila (o rascunho não conta).
	if !strings.Contains(guia, `<span class="contador novo" title="Mudanças não vistas">1</span>`) || !strings.Contains(guia, `<span class="contador" title="PRs aguardando review">2</span>`) {
		t.Errorf("contadores do topo errados")
	}

	// A fila abre o primeiro PR (risco alto) por padrão, ou o de ?abrir=.
	if fila := get("/prs"); !strings.Contains(fila, `<details id="pr-2" open>`) || strings.Contains(fila, `<details id="pr-1" open>`) {
		t.Errorf("sem ?abrir, só o primeiro PR da fila deveria vir aberto")
	}
	if fila := get("/prs?abrir=1"); !strings.Contains(fila, `<details id="pr-1" open>`) || strings.Contains(fila, `<details id="pr-2" open>`) {
		t.Errorf("?abrir=1 deveria abrir só o PR 1")
	}
}
