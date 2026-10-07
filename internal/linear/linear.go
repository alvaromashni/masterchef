// Package linear é um cliente mínimo da API GraphQL do Linear.
//
// Usamos net/http puro em vez de um SDK: só precisamos de uma consulta, e
// assim fica fácil ver exatamente o que vai e volta pela rede.
// O cliente só LÊ dados; o painel nunca escreve no Linear.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultEndpoint é o endereço da API GraphQL do Linear.
const DefaultEndpoint = "https://api.linear.app/graphql"

// tamanhoPagina é quantas issues pedimos por requisição (o máximo do Linear é 250).
const tamanhoPagina = 100

// Issue é uma issue do Linear com os campos que o painel usa.
type Issue struct {
	ID          string
	Identifier  string // ex.: ABC-123
	Title       string
	Description string
	StateName   string // ex.: "In Progress"
	StateType   string // backlog, unstarted, started, completed, canceled
	Labels      []string
	URL         string
	UpdatedAt   time.Time
	// URLs dos anexos (o Linear anexa PRs do GitHub como links). Usado na Fase 2.
	AttachmentURLs []string
}

// Client fala com a API do Linear.
type Client struct {
	apiKey   string
	endpoint string
	http     *http.Client
}

// New cria um cliente. endpoint vazio usa o da API real; os testes passam
// o endereço de um servidor local falso.
func New(apiKey, endpoint string) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{
		apiKey:   apiKey,
		endpoint: endpoint,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

// A consulta pede só os campos necessários. $filter é montado em Go
// (ver filtroIssues) porque muda entre o primeiro sync e os seguintes.
const consultaIssues = `
query Issues($filter: IssueFilter, $after: String, $first: Int) {
  issues(first: $first, after: $after, filter: $filter) {
    nodes {
      id
      identifier
      title
      description
      url
      updatedAt
      state { name type }
      labels { nodes { name } }
      attachments { nodes { url } }
    }
    pageInfo { hasNextPage endCursor }
  }
}`

// ListIssues busca todas as issues do projeto, página por página.
//
// Se updatedSince for nil (primeiro sync), traz todas as issues não canceladas.
// Senão, traz só as atualizadas depois dessa data, INCLUINDO as canceladas,
// para que o painel perceba quando uma issue é cancelada.
func (c *Client) ListIssues(ctx context.Context, projectID string, updatedSince *time.Time) ([]Issue, error) {
	var todas []Issue
	var cursor *string // nil = primeira página

	for {
		variaveis := map[string]any{
			"filter": filtroIssues(projectID, updatedSince),
			"after":  cursor,
			"first":  tamanhoPagina,
		}
		var resp respostaIssues
		if err := c.consultar(ctx, consultaIssues, variaveis, &resp); err != nil {
			return nil, fmt.Errorf("buscando issues do projeto %s: %w", projectID, err)
		}

		for _, n := range resp.Issues.Nodes {
			todas = append(todas, n.paraIssue())
		}

		if !resp.Issues.PageInfo.HasNextPage {
			return todas, nil
		}
		proximo := resp.Issues.PageInfo.EndCursor
		cursor = &proximo
	}
}

// filtroIssues monta o IssueFilter do GraphQL do Linear.
func filtroIssues(projectID string, updatedSince *time.Time) map[string]any {
	filtro := map[string]any{
		"project": map[string]any{"id": map[string]any{"eq": projectID}},
	}
	if updatedSince == nil {
		filtro["state"] = map[string]any{"type": map[string]any{"neq": "canceled"}}
	} else {
		filtro["updatedAt"] = map[string]any{"gt": updatedSince.UTC().Format(time.RFC3339)}
	}
	return filtro
}

// consultar faz um POST GraphQL e decodifica "data" em destino.
func (c *Client) consultar(ctx context.Context, query string, variaveis map[string]any, destino any) error {
	corpo, err := json.Marshal(map[string]any{"query": query, "variables": variaveis})
	if err != nil {
		return fmt.Errorf("montando requisição: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(corpo))
	if err != nil {
		return fmt.Errorf("montando requisição: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Chave pessoal do Linear vai pura, sem "Bearer " (seção 7 do CONTEXT.md).
	req.Header.Set("Authorization", c.apiKey)

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("chamando o Linear: %w", err)
	}
	defer res.Body.Close()

	dados, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("lendo resposta do Linear: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Linear respondeu HTTP %d: %s", res.StatusCode, resumir(dados))
	}

	// GraphQL costuma responder 200 mesmo com erro; o erro vem no campo "errors".
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(dados, &envelope); err != nil {
		return fmt.Errorf("resposta do Linear não é JSON válido: %w", err)
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("Linear retornou erro: %s", envelope.Errors[0].Message)
	}
	if err := json.Unmarshal(envelope.Data, destino); err != nil {
		return fmt.Errorf("decodificando dados do Linear: %w", err)
	}
	return nil
}

// resumir corta respostas longas para não poluir o log e a tela.
func resumir(b []byte) string {
	const max = 300
	if len(b) > max {
		return string(b[:max]) + "..."
	}
	return string(b)
}

// Os tipos abaixo espelham o JSON da API. Ficam separados de Issue para que
// o resto do painel não dependa do formato aninhado do GraphQL.
type respostaIssues struct {
	Issues struct {
		Nodes    []noIssue `json:"nodes"`
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"issues"`
}

type noIssue struct {
	ID          string    `json:"id"`
	Identifier  string    `json:"identifier"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	UpdatedAt   time.Time `json:"updatedAt"`
	State       struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"state"`
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	Attachments struct {
		Nodes []struct {
			URL string `json:"url"`
		} `json:"nodes"`
	} `json:"attachments"`
}

func (n noIssue) paraIssue() Issue {
	issue := Issue{
		ID:          n.ID,
		Identifier:  n.Identifier,
		Title:       n.Title,
		Description: n.Description,
		StateName:   n.State.Name,
		StateType:   n.State.Type,
		URL:         n.URL,
		UpdatedAt:   n.UpdatedAt,
	}
	for _, l := range n.Labels.Nodes {
		issue.Labels = append(issue.Labels, l.Name)
	}
	for _, a := range n.Attachments.Nodes {
		issue.AttachmentURLs = append(issue.AttachmentURLs, a.URL)
	}
	return issue
}
