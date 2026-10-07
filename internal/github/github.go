// Package github é um cliente mínimo da API REST do GitHub.
//
// Só usamos dois endpoints (listar PRs e listar arquivos de um PR), então
// net/http puro resolve sem SDK. O cliente só LÊ: o token deve ser
// fine-grained e somente leitura.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

// DefaultBaseURL é o endereço da API real.
const DefaultBaseURL = "https://api.github.com"

// esperaMaxima limita quanto tempo esperamos por um rate limit. Se o GitHub
// pedir mais que isso, desistimos deste ciclo e tentamos no próximo.
const esperaMaxima = 2 * time.Minute

// PR é um pull request com os campos que o painel usa.
type PR struct {
	ID        int64
	Repo      string // owner/nome
	Number    int
	Title     string
	Body      string
	Branch    string
	State     string // open, closed ou merged
	Draft     bool
	URL       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Arquivo é um arquivo alterado num PR.
type Arquivo struct {
	Nome      string
	Additions int
	Deletions int
}

// Client fala com a API do GitHub.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
	logger  *slog.Logger
	// dormir existe para os testes não esperarem de verdade no rate limit.
	dormir func(ctx context.Context, d time.Duration) error
}

// New cria um cliente. baseURL vazio usa a API real.
func New(token, baseURL string, logger *slog.Logger) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		token:   token,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
		logger:  logger,
		dormir:  dormirComContexto,
	}
}

// ListPRs devolve os PRs do repo (abertos, fechados e mergeados).
//
// Pedimos ordenado por "atualizado por último" primeiro. Assim, quando
// updatedSince é informado, podemos parar de paginar assim que achamos um
// PR mais antigo que ele: todos os seguintes também serão.
func (c *Client) ListPRs(ctx context.Context, repo string, updatedSince *time.Time) ([]PR, error) {
	proxima := fmt.Sprintf("%s/repos/%s/pulls?state=all&sort=updated&direction=desc&per_page=100", c.baseURL, repo)
	var todos []PR

	for proxima != "" {
		var pagina []prJSON
		link, err := c.get(ctx, proxima, &pagina)
		if err != nil {
			return nil, fmt.Errorf("buscando PRs de %s: %w", repo, err)
		}
		for _, p := range pagina {
			if updatedSince != nil && !p.UpdatedAt.After(*updatedSince) {
				return todos, nil
			}
			todos = append(todos, p.paraPR(repo))
		}
		proxima = proximaPagina(link)
	}
	return todos, nil
}

// ListArquivos devolve todos os arquivos alterados num PR (paginado).
func (c *Client) ListArquivos(ctx context.Context, repo string, number int) ([]Arquivo, error) {
	proxima := fmt.Sprintf("%s/repos/%s/pulls/%d/files?per_page=100", c.baseURL, repo, number)
	var todos []Arquivo

	for proxima != "" {
		var pagina []struct {
			Filename  string `json:"filename"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		}
		link, err := c.get(ctx, proxima, &pagina)
		if err != nil {
			return nil, fmt.Errorf("buscando arquivos de %s#%d: %w", repo, number, err)
		}
		for _, a := range pagina {
			todos = append(todos, Arquivo{Nome: a.Filename, Additions: a.Additions, Deletions: a.Deletions})
		}
		proxima = proximaPagina(link)
	}
	return todos, nil
}

// get faz um GET, decodifica o JSON em destino e devolve o header Link
// (que diz onde está a próxima página).
//
// Rate limit, do jeito simples: se o GitHub responde 403/429 avisando que
// o limite acabou, esperamos até o horário que ele indica (no máximo
// esperaMaxima) e tentamos UMA vez de novo.
func (c *Client) get(ctx context.Context, endereco string, destino any) (string, error) {
	for tentativa := 1; ; tentativa++ {
		res, err := c.fazerGet(ctx, endereco)
		if err != nil {
			return "", err
		}

		if espera, limitado := esperaDoRateLimit(res); limitado && tentativa == 1 {
			res.Body.Close()
			if espera > esperaMaxima {
				return "", fmt.Errorf("rate limit do GitHub esgotado; libera em %s", espera.Round(time.Second))
			}
			c.logger.Warn("rate limit do GitHub, aguardando", "espera", espera.Round(time.Second))
			if err := c.dormir(ctx, espera); err != nil {
				return "", err
			}
			continue
		}

		defer res.Body.Close()
		dados, err := io.ReadAll(res.Body)
		if err != nil {
			return "", fmt.Errorf("lendo resposta: %w", err)
		}
		if res.StatusCode != http.StatusOK {
			return "", fmt.Errorf("GitHub respondeu HTTP %d: %s", res.StatusCode, resumir(dados))
		}
		if err := json.Unmarshal(dados, destino); err != nil {
			return "", fmt.Errorf("decodificando resposta: %w", err)
		}
		return res.Header.Get("Link"), nil
	}
}

func (c *Client) fazerGet(ctx context.Context, endereco string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endereco, nil)
	if err != nil {
		return nil, fmt.Errorf("montando requisição: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chamando o GitHub: %w", err)
	}
	return res, nil
}

// esperaDoRateLimit diz se a resposta é um "limite esgotado" e quanto esperar.
// O GitHub usa dois formatos: Retry-After (segundos) ou
// X-RateLimit-Remaining: 0 com X-RateLimit-Reset (horário Unix).
func esperaDoRateLimit(res *http.Response) (time.Duration, bool) {
	if res.StatusCode != http.StatusForbidden && res.StatusCode != http.StatusTooManyRequests {
		return 0, false
	}
	if s := res.Header.Get("Retry-After"); s != "" {
		if seg, err := strconv.Atoi(s); err == nil {
			return time.Duration(seg) * time.Second, true
		}
	}
	if res.Header.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(res.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			espera := time.Until(time.Unix(reset, 0))
			if espera < time.Second {
				espera = time.Second
			}
			return espera, true
		}
	}
	// 403 sem esses headers é falta de permissão, não rate limit.
	return 0, false
}

func dormirComContexto(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// linkNext acha a URL com rel="next" no header Link, ex.:
// <https://api.github.com/...&page=2>; rel="next", <...>; rel="last"
var linkNext = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func proximaPagina(link string) string {
	m := linkNext.FindStringSubmatch(link)
	if m == nil {
		return ""
	}
	if _, err := url.Parse(m[1]); err != nil {
		return ""
	}
	return m[1]
}

func resumir(b []byte) string {
	const max = 300
	if len(b) > max {
		return string(b[:max]) + "..."
	}
	return string(b)
}

// prJSON espelha o JSON da API; fica separado de PR para isolar o formato.
type prJSON struct {
	ID        int64      `json:"id"`
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"` // open ou closed
	Draft     bool       `json:"draft"`
	HTMLURL   string     `json:"html_url"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	MergedAt  *time.Time `json:"merged_at"`
	Head      struct {
		Ref string `json:"ref"`
	} `json:"head"`
}

func (p prJSON) paraPR(repo string) PR {
	estado := p.State
	// A API diz "closed" tanto para fechado quanto para mergeado;
	// quem diferencia é o merged_at.
	if p.MergedAt != nil {
		estado = "merged"
	}
	return PR{
		ID:        p.ID,
		Repo:      repo,
		Number:    p.Number,
		Title:     p.Title,
		Body:      p.Body,
		Branch:    p.Head.Ref,
		State:     estado,
		Draft:     p.Draft,
		URL:       p.HTMLURL,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}
