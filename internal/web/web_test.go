package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func novoServidor(t *testing.T) http.Handler {
	t.Helper()
	// O store ainda não é usado pelas páginas da Fase 0, então nil basta.
	s, err := New(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
	for _, trecho := range []string{"<h1>Matriz</h1>", "htmx.org", "/static/painel.css"} {
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
