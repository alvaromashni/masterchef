package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// envCompleto simula um ambiente com os dois segredos definidos.
func envCompleto(key string) string {
	return map[string]string{
		"LINEAR_API_KEY": "lin_api_teste",
		"GITHUB_TOKEN":   "ghp_teste",
	}[key]
}

func lerFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "valida.yaml"))
	if err != nil {
		t.Fatalf("lendo fixture: %v", err)
	}
	return string(data)
}

func TestParse_ConfigValida(t *testing.T) {
	cfg, err := Parse([]byte(lerFixture(t)), envCompleto)
	if err != nil {
		t.Fatalf("esperava config válida, veio erro: %v", err)
	}

	if cfg.PollInterval != 5*time.Minute {
		t.Errorf("PollInterval = %v, esperava 5m", cfg.PollInterval)
	}
	if cfg.StaleAfter != 72*time.Hour {
		t.Errorf("StaleAfter = %v, esperava 72h", cfg.StaleAfter)
	}
	if cfg.LinearAPIKey != "lin_api_teste" || cfg.GitHubToken != "ghp_teste" {
		t.Errorf("segredos não foram lidos do ambiente: %q / %q", cfg.LinearAPIKey, cfg.GitHubToken)
	}

	home, _ := os.UserHomeDir()
	if want := filepath.Join(home, "painel.db"); cfg.DBPath != want {
		t.Errorf("DBPath = %q, esperava %q (com ~ expandido)", cfg.DBPath, want)
	}
	if want := filepath.Join(home, "dev/painel-central"); cfg.CentralRepoPath != want {
		t.Errorf("CentralRepoPath = %q, esperava %q", cfg.CentralRepoPath, want)
	}

	// A ordem dos escopos tem que ser a mesma do arquivo (vira a ordem das colunas).
	var nomes []string
	for _, e := range cfg.Produtos[0].Escopos {
		nomes = append(nomes, e.Nome)
	}
	if got := strings.Join(nomes, ","); got != "api,front,infra" {
		t.Errorf("ordem dos escopos = %s, esperava api,front,infra", got)
	}
	if cfg.Produtos[0].Escopos[0].Repo != "alvaromashni/produto-x-api" {
		t.Errorf("repo do escopo api = %q", cfg.Produtos[0].Escopos[0].Repo)
	}
}

// Cada caso pega a fixture válida, troca um trecho por algo inválido e
// confere que o erro menciona o problema certo.
func TestParse_ConfigInvalida(t *testing.T) {
	casos := []struct {
		nome       string
		trocar     string // trecho da fixture válida
		por        string
		erroContem string
	}{
		{
			nome:       "slug duplicado",
			trocar:     "slug: produto-y",
			por:        "slug: produto-x",
			erroContem: "slug duplicado",
		},
		{
			nome:       "slug com maiúscula",
			trocar:     "slug: produto-y",
			por:        "slug: Produto-Y",
			erroContem: "slug deve ter só letras minúsculas",
		},
		{
			nome:       "repo repetido em outro produto",
			trocar:     "app: alvaromashni/produto-y-app",
			por:        "app: alvaromashni/produto-x-api",
			erroContem: "já foi usado em produto-x/api",
		},
		{
			nome:       "repo repetido no mesmo produto",
			trocar:     "infra: alvaromashni/produto-x-infra",
			por:        "infra: alvaromashni/produto-x-front",
			erroContem: "já foi usado em produto-x/front",
		},
		{
			nome:       "repo fora do formato owner/repo",
			trocar:     "app: alvaromashni/produto-y-app",
			por:        "app: produto-y-app",
			erroContem: "formato owner/repo",
		},
		{
			nome:       "escopo com nome reservado",
			trocar:     "app: alvaromashni/produto-y-app",
			por:        "decisoes: alvaromashni/produto-y-app",
			erroContem: "nome reservado",
		},
		{
			nome:       "escopo com o nome da coluna a classificar",
			trocar:     "app: alvaromashni/produto-y-app",
			por:        "a-classificar: alvaromashni/produto-y-app",
			erroContem: "nome reservado",
		},
		{
			nome:       "nível de risco inválido",
			trocar:     "nivel: medio",
			por:        "nivel: critico",
			erroContem: `nivel "critico" inválido`,
		},
		{
			nome:       "regra sem motivo",
			trocar:     `motivo: "Altera CI/CD"`,
			por:        `motivo: ""`,
			erroContem: "motivo é obrigatório",
		},
		{
			nome:       "glob inválido",
			trocar:     `padrao: "**/migrations/**"`,
			por:        `padrao: "**/[migrations"`,
			erroContem: "não é um glob válido",
		},
		{
			nome:       "limite de linhas zero",
			trocar:     "limite_linhas_diff: 400",
			por:        "limite_linhas_diff: 0",
			erroContem: "limite_linhas_diff deve ser maior que zero",
		},
		{
			nome:       "poll_interval inválido",
			trocar:     "poll_interval: 5m",
			por:        "poll_interval: cinco minutos",
			erroContem: "poll_interval",
		},
		{
			nome:       "stale_after ausente",
			trocar:     "stale_after: 72h",
			por:        "",
			erroContem: "stale_after é obrigatório",
		},
		{
			nome:       "listen ausente",
			trocar:     "listen: 127.0.0.1:7777",
			por:        "",
			erroContem: "listen é obrigatório",
		},
		{
			nome:       "linear_project_id ausente",
			trocar:     `linear_project_id: "uuid-y"`,
			por:        "",
			erroContem: "linear_project_id é obrigatório",
		},
		{
			nome:       "chave desconhecida (erro de digitação)",
			trocar:     "poll_interval: 5m",
			por:        "pool_interval: 5m",
			erroContem: "pool_interval",
		},
	}

	base := lerFixture(t)
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if !strings.Contains(base, c.trocar) {
				t.Fatalf("trecho %q não existe na fixture; o teste está desatualizado", c.trocar)
			}
			yaml := strings.Replace(base, c.trocar, c.por, 1)

			_, err := Parse([]byte(yaml), envCompleto)
			if err == nil {
				t.Fatalf("esperava erro contendo %q, mas a config foi aceita", c.erroContem)
			}
			if !strings.Contains(err.Error(), c.erroContem) {
				t.Errorf("erro = %q\nesperava conter %q", err, c.erroContem)
			}
		})
	}
}

func TestParse_SemSegredos(t *testing.T) {
	envVazio := func(string) string { return "" }

	_, err := Parse([]byte(lerFixture(t)), envVazio)
	if err == nil {
		t.Fatal("esperava erro sem LINEAR_API_KEY e GITHUB_TOKEN")
	}
	// Os dois problemas devem aparecer juntos, não um de cada vez.
	for _, trecho := range []string{"LINEAR_API_KEY", "GITHUB_TOKEN"} {
		if !strings.Contains(err.Error(), trecho) {
			t.Errorf("erro deveria mencionar %s: %v", trecho, err)
		}
	}
}

func TestParse_SemProdutos(t *testing.T) {
	yaml := `
poll_interval: 5m
listen: 127.0.0.1:7777
db_path: ./painel.db
central_repo_path: ./central
stale_after: 72h
risco:
  limite_linhas_diff: 400
`
	_, err := Parse([]byte(yaml), envCompleto)
	if err == nil || !strings.Contains(err.Error(), "pelo menos um produto") {
		t.Fatalf("esperava erro de produtos vazios, veio: %v", err)
	}
}

func TestLoad_ArquivoInexistente(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nao-existe.yaml"))
	if err == nil || !strings.Contains(err.Error(), "lendo config") {
		t.Fatalf("esperava erro de leitura com contexto, veio: %v", err)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("sem home dir neste ambiente")
	}
	casos := map[string]string{
		"~":           home,
		"~/x/y.db":    filepath.Join(home, "x/y.db"),
		"./painel.db": "./painel.db",
		"/abs/p.db":   "/abs/p.db",
		"~outro/x":    "~outro/x", // "~usuario" não é suportado; fica como está
	}
	for entrada, esperado := range casos {
		got, err := expandHome(entrada)
		if err != nil {
			t.Fatalf("expandHome(%q): %v", entrada, err)
		}
		if got != esperado {
			t.Errorf("expandHome(%q) = %q, esperava %q", entrada, got, esperado)
		}
	}
}
