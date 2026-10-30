package github

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListDocs_SoMarkdownDaPastaDocs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/api":
			fmt.Fprint(w, `{"default_branch": "develop"}`)
		case "/repos/o/api/git/trees/develop":
			if r.URL.Query().Get("recursive") != "1" {
				t.Errorf("faltou recursive=1: %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"tree": [
				{"path": "README.md", "type": "blob", "size": 10},
				{"path": "docs", "type": "tree"},
				{"path": "docs/visao.md", "type": "blob", "size": 120},
				{"path": "docs/arquitetura/banco.MD", "type": "blob", "size": 80},
				{"path": "docs/diagrama.png", "type": "blob", "size": 999},
				{"path": "src/docs/outro.md", "type": "blob", "size": 5}
			], "truncated": false}`)
		default:
			t.Errorf("caminho inesperado %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	docs, err := New("tok", srv.URL, loggerSilencioso).ListDocs(context.Background(), "o/api")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 || docs[0].Caminho != "docs/arquitetura/banco.MD" || docs[1].Caminho != "docs/visao.md" {
		t.Fatalf("docs = %+v", docs)
	}
}

func TestReadDoc_DecodificaBase64ComQuebras(t *testing.T) {
	texto := "# Visão\n\nO app de pagamentos.\n"
	b64 := base64.StdEncoding.EncodeToString([]byte(texto))
	// Simula as quebras de linha que o GitHub coloca no base64.
	b64 = b64[:10] + `\n` + b64[10:]

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/repos/o/api/contents/docs/vis%C3%A3o%20geral.md" {
			t.Errorf("caminho = %s", r.URL.EscapedPath())
		}
		fmt.Fprintf(w, `{"encoding": "base64", "content": "%s"}`, b64)
	}))
	t.Cleanup(srv.Close)

	dados, err := New("tok", srv.URL, loggerSilencioso).ReadDoc(context.Background(), "o/api", "docs/visão geral.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(dados) != texto {
		t.Fatalf("conteúdo = %q", dados)
	}
}
