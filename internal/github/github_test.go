package github

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var loggerSilencioso = slog.New(slog.NewTextHandler(io.Discard, nil))

// servirFixture escreve o conteúdo de testdata/nome na resposta.
func servirFixture(t *testing.T, w http.ResponseWriter, nome string) {
	t.Helper()
	dados, err := os.ReadFile(filepath.Join("testdata", nome))
	if err != nil {
		t.Fatal(err)
	}
	w.Write(dados)
}

// servidorPRs simula a paginação do GitHub: a página 1 aponta para a 2
// pelo header Link. Conta quantas páginas foram pedidas.
func servidorPRs(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	paginas := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		paginas++
		if r.URL.Query().Get("page") == "2" {
			servirFixture(t, w, "prs_pagina2.json")
			return
		}
		if q := r.URL.Query(); q.Get("state") != "all" || q.Get("sort") != "updated" || q.Get("direction") != "desc" {
			t.Errorf("query inesperada: %s", r.URL.RawQuery)
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=2>; rel="next", <%s%s?page=2>; rel="last"`, srv.URL, r.URL.Path, srv.URL, r.URL.Path))
		servirFixture(t, w, "prs_pagina1.json")
	}))
	t.Cleanup(srv.Close)
	return srv, &paginas
}

func TestListPRs_TodasAsPaginas(t *testing.T) {
	srv, paginas := servidorPRs(t)
	prs, err := New("tok", srv.URL, loggerSilencioso).ListPRs(context.Background(), "o/api", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 3 || *paginas != 2 {
		t.Fatalf("veio %d PRs em %d páginas, esperava 3 em 2", len(prs), *paginas)
	}

	aberto := prs[0]
	if aberto.State != "open" || aberto.Branch != "alvaro/abc-12-login" || aberto.Repo != "o/api" || aberto.Body != "Fecha ABC-12" {
		t.Errorf("PR aberto decodificado errado: %+v", aberto)
	}
	if prs[1].State != "merged" {
		t.Errorf("PR com merged_at deveria ser merged, veio %s", prs[1].State)
	}
	if prs[2].State != "closed" || !prs[2].Draft {
		t.Errorf("PR fechado sem merge: %+v", prs[2])
	}
}

func TestListPRs_ParaNoUltimoSync(t *testing.T) {
	srv, paginas := servidorPRs(t)
	// Só o PR #3 (atualizado em 06/10) é mais novo que 05/10.
	desde := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	prs, err := New("tok", srv.URL, loggerSilencioso).ListPRs(context.Background(), "o/api", &desde)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].Number != 3 {
		t.Fatalf("esperava só o PR #3, veio %+v", prs)
	}
	if *paginas != 1 {
		t.Errorf("pediu %d páginas; deveria parar na 1ª ao achar um PR antigo", *paginas)
	}
}

func TestListArquivos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/api/pulls/3/files" {
			t.Errorf("caminho = %s", r.URL.Path)
		}
		servirFixture(t, w, "arquivos.json")
	}))
	defer srv.Close()

	arqs, err := New("tok", srv.URL, loggerSilencioso).ListArquivos(context.Background(), "o/api", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(arqs) != 2 || arqs[0].Nome != "db/migrations/0002.sql" || arqs[1].Additions != 30 {
		t.Errorf("arquivos = %+v", arqs)
	}
}

func TestRateLimit_EsperaETentaDeNovo(t *testing.T) {
	chamadas := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chamadas++
		if chamadas == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := New("tok", srv.URL, loggerSilencioso)
	var dormiu time.Duration
	c.dormir = func(_ context.Context, d time.Duration) error { dormiu = d; return nil }

	if _, err := c.ListPRs(context.Background(), "o/api", nil); err != nil {
		t.Fatalf("depois de esperar deveria funcionar: %v", err)
	}
	if dormiu != 7*time.Second || chamadas != 2 {
		t.Errorf("dormiu %v em %d chamadas; esperava 7s e 2 chamadas", dormiu, chamadas)
	}
}

func TestRateLimit_EsperaLongaDemaisDesiste(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(time.Hour).Unix()))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := New("tok", srv.URL, loggerSilencioso)
	c.dormir = func(context.Context, time.Duration) error { t.Error("não deveria esperar 1h"); return nil }

	_, err := c.ListPRs(context.Background(), "o/api", nil)
	if err == nil || !strings.Contains(err.Error(), "rate limit") || !strings.Contains(err.Error(), "o/api") {
		t.Fatalf("esperava erro de rate limit com o repo, veio: %v", err)
	}
}

func TestErroDePermissao(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Resource not accessible"}`, http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := New("tok", srv.URL, loggerSilencioso).ListPRs(context.Background(), "o/api", nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("esperava HTTP 403, veio: %v", err)
	}
}

func TestListPRsAbertos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("state"); got != "open" {
			t.Errorf("state = %q, esperava open", got)
		}
		servirFixture(t, w, "prs_pagina2.json")
	}))
	defer srv.Close()

	if _, err := New("tok", srv.URL, loggerSilencioso).ListPRsAbertos(context.Background(), "o/api"); err != nil {
		t.Fatal(err)
	}
}
