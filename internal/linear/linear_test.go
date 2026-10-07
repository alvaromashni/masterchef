package linear

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// requisicaoGraphQL é o que o cliente manda; o servidor falso decodifica para conferir.
type requisicaoGraphQL struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// servidorFalso sobe um HTTP local (httptest, sem internet) que responde
// com as fixtures de testdata/ e guarda as requisições recebidas.
func servidorFalso(t *testing.T, fixtures ...string) (*httptest.Server, *[]requisicaoGraphQL, *[]http.Header) {
	t.Helper()
	var reqs []requisicaoGraphQL
	var headers []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corpo, _ := io.ReadAll(r.Body)
		var req requisicaoGraphQL
		if err := json.Unmarshal(corpo, &req); err != nil {
			t.Errorf("corpo não é JSON: %v", err)
		}
		reqs = append(reqs, req)
		headers = append(headers, r.Header.Clone())

		i := len(reqs) - 1
		if i >= len(fixtures) {
			t.Errorf("requisição extra inesperada (%d)", i+1)
			return
		}
		dados, err := os.ReadFile(filepath.Join("testdata", fixtures[i]))
		if err != nil {
			t.Fatal(err)
		}
		w.Write(dados)
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs, &headers
}

func TestListIssues_Paginacao(t *testing.T) {
	srv, reqs, headers := servidorFalso(t, "issues_pagina1.json", "issues_pagina2.json")
	c := New("lin_api_chave", srv.URL)

	issues, err := c.ListIssues(context.Background(), "proj-1", nil)
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	if len(issues) != 2 {
		t.Fatalf("veio %d issues, esperava 2 (uma por página)", len(issues))
	}
	if len(*reqs) != 2 {
		t.Fatalf("fez %d requisições, esperava 2", len(*reqs))
	}
	// A segunda página tem que pedir a partir do cursor da primeira.
	if got := (*reqs)[1].Variables["after"]; got != "cursor-1" {
		t.Errorf("after da 2ª página = %v, esperava cursor-1", got)
	}
	if got := (*headers)[0].Get("Authorization"); got != "lin_api_chave" {
		t.Errorf("Authorization = %q, esperava a chave pura, sem Bearer", got)
	}

	i := issues[0]
	if i.Identifier != "ABC-1" || i.StateName != "In Progress" || i.StateType != "started" {
		t.Errorf("issue decodificada errada: %+v", i)
	}
	if strings.Join(i.Labels, ",") != "scope:api,bug" {
		t.Errorf("labels = %v", i.Labels)
	}
	if len(i.AttachmentURLs) != 1 || !strings.Contains(i.AttachmentURLs[0], "/pull/7") {
		t.Errorf("anexos = %v", i.AttachmentURLs)
	}
	if !i.UpdatedAt.Equal(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("UpdatedAt = %v", i.UpdatedAt)
	}
	// description null no JSON vira string vazia.
	if issues[1].Description != "" {
		t.Errorf("Description de null = %q", issues[1].Description)
	}
}

func TestListIssues_Filtros(t *testing.T) {
	t.Run("primeiro sync exclui canceladas e não filtra data", func(t *testing.T) {
		srv, reqs, _ := servidorFalso(t, "issues_pagina2.json")
		if _, err := New("k", srv.URL).ListIssues(context.Background(), "proj-1", nil); err != nil {
			t.Fatal(err)
		}
		filtro, _ := json.Marshal((*reqs)[0].Variables["filter"])
		if !strings.Contains(string(filtro), `"neq":"canceled"`) {
			t.Errorf("filtro deveria excluir canceladas: %s", filtro)
		}
		if strings.Contains(string(filtro), "updatedAt") {
			t.Errorf("primeiro sync não deveria filtrar por data: %s", filtro)
		}
		if !strings.Contains(string(filtro), `"eq":"proj-1"`) {
			t.Errorf("filtro deveria ter o projeto: %s", filtro)
		}
	})

	t.Run("sync incremental filtra por data e inclui canceladas", func(t *testing.T) {
		srv, reqs, _ := servidorFalso(t, "issues_pagina2.json")
		desde := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
		if _, err := New("k", srv.URL).ListIssues(context.Background(), "proj-1", &desde); err != nil {
			t.Fatal(err)
		}
		filtro, _ := json.Marshal((*reqs)[0].Variables["filter"])
		if !strings.Contains(string(filtro), `"gt":"2026-10-05T10:00:00Z"`) {
			t.Errorf("filtro deveria ter updatedAt > desde: %s", filtro)
		}
		if strings.Contains(string(filtro), "canceled") {
			t.Errorf("sync incremental deveria trazer canceladas: %s", filtro)
		}
	})
}

func TestListIssues_ErroGraphQL(t *testing.T) {
	srv, _, _ := servidorFalso(t, "erro_graphql.json")
	_, err := New("k", srv.URL).ListIssues(context.Background(), "proj-x", nil)
	if err == nil || !strings.Contains(err.Error(), "Entity not found") || !strings.Contains(err.Error(), "proj-x") {
		t.Fatalf("esperava erro do GraphQL com contexto, veio: %v", err)
	}
}

func TestListIssues_ErroHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "chave inválida", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := New("k", srv.URL).ListIssues(context.Background(), "proj-1", nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("esperava erro HTTP 401, veio: %v", err)
	}
}
