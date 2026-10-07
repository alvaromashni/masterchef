package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/store"
)

func novoServidor(t *testing.T) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(filepath.Join(t.TempDir(), "teste.db"), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	cfg := &config.Config{Produtos: []config.Produto{
		{Nome: "Produto X", Slug: "produto-x", Escopos: config.Escopos{{Nome: "api"}, {Nome: "front"}}},
	}}
	s, err := New(cfg, st, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s.Handler()
}

func TestIndex(t *testing.T) {
	rec := httptest.NewRecorder()
	novoServidor(t).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperava 200", rec.Code)
	}
	corpo := rec.Body.String()
	for _, trecho := range []string{"<h1>Matriz</h1>", "htmx.org", "/static/painel.css", "Produto X", "a classificar", "Último sync: nunca"} {
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

func TestCSS(t *testing.T) {
	rec := httptest.NewRecorder()
	novoServidor(t).ServeHTTP(rec, httptest.NewRequest("GET", "/static/painel.css", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, esperava 200", rec.Code)
	}
}

func TestFilaDeReviewVazia(t *testing.T) {
	rec := httptest.NewRecorder()
	novoServidor(t).ServeHTTP(rec, httptest.NewRequest("GET", "/prs", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Nenhum PR aberto") {
		t.Errorf("status = %d; corpo sem a mensagem de fila vazia", rec.Code)
	}
}
