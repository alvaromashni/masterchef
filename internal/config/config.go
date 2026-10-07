// Package config carrega e valida o config.yaml do painel.
//
// A validação acontece toda aqui, na inicialização, para que um erro de
// digitação no YAML apareça logo ao subir o binário e não horas depois,
// no meio de um ciclo de coleta.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

// Níveis de risco aceitos nas regras do config.yaml.
// "baixo" não entra aqui porque é o resultado padrão, não algo que uma regra atribui.
const (
	NivelAlto  = "alto"
	NivelMedio = "medio"
)

// Nomes que não podem ser usados como escopo porque colidem com as rotas
// /p/{produto}/decisoes e /p/{produto}/a-classificar (seção 11 do CONTEXT.md).
const (
	EscopoReservado    = "decisoes"
	EscopoAClassificar = "a-classificar"
)

// Config é o conteúdo do config.yaml já validado e com os segredos lidos do ambiente.
type Config struct {
	// Campos como string no YAML e convertidos em Parse, para dar mensagens de erro claras.
	PollIntervalRaw string `yaml:"poll_interval"`
	StaleAfterRaw   string `yaml:"stale_after"`

	Listen          string    `yaml:"listen"`
	DBPath          string    `yaml:"db_path"`
	CentralRepoPath string    `yaml:"central_repo_path"`
	Produtos        []Produto `yaml:"produtos"`
	Risco           Risco     `yaml:"risco"`

	// Preenchidos por Parse; não vêm do YAML.
	PollInterval time.Duration `yaml:"-"`
	StaleAfter   time.Duration `yaml:"-"`
	LinearAPIKey string        `yaml:"-"`
	GitHubToken  string        `yaml:"-"`
}

// Produto é uma linha da matriz.
type Produto struct {
	Nome            string  `yaml:"nome"`
	Slug            string  `yaml:"slug"`
	LinearProjectID string  `yaml:"linear_project_id"`
	Escopos         Escopos `yaml:"escopos"`
}

// Escopo liga um nome de escopo (api, front...) a um repositório GitHub.
type Escopo struct {
	Nome string // ex.: "api"
	Repo string // ex.: "alvaromashni/produto-x-api"
}

// Escopos é uma lista (e não um map) para preservar a ordem em que o dono
// escreveu os escopos no YAML. Essa ordem vira a ordem das colunas da matriz.
type Escopos []Escopo

// UnmarshalYAML lê o mapa "nome: repo" mantendo a ordem do arquivo.
// Um map[string]string do Go perderia essa ordem.
func (e *Escopos) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("linha %d: escopos deve ser um mapa no formato nome: owner/repo", node.Line)
	}
	// Num MappingNode, Content alterna chave, valor, chave, valor...
	for i := 0; i+1 < len(node.Content); i += 2 {
		chave, valor := node.Content[i], node.Content[i+1]
		if valor.Kind != yaml.ScalarNode {
			return fmt.Errorf("linha %d: o repo do escopo %q deve ser um texto owner/repo", valor.Line, chave.Value)
		}
		*e = append(*e, Escopo{Nome: chave.Value, Repo: valor.Value})
	}
	return nil
}

// Risco são as regras do motor de risco (seção 8 do CONTEXT.md).
type Risco struct {
	LimiteLinhasDiff int          `yaml:"limite_linhas_diff"`
	Regras           []RegraRisco `yaml:"regras"`
	PadroesDeTeste   []string     `yaml:"padroes_de_teste"`
}

// RegraRisco atribui um nível a PRs que alteram arquivos que casam com Padrao.
type RegraRisco struct {
	Padrao string `yaml:"padrao"`
	Nivel  string `yaml:"nivel"`
	Motivo string `yaml:"motivo"`
}

// Load lê o arquivo em path, os segredos do ambiente e valida tudo.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lendo config %s: %w", path, err)
	}
	cfg, err := Parse(data, os.Getenv)
	if err != nil {
		return nil, fmt.Errorf("config %s inválida:\n%w", path, err)
	}
	return cfg, nil
}

// Parse faz o trabalho de Load a partir de bytes. Recebe getenv como
// parâmetro para que os testes não dependam das variáveis de ambiente reais.
func Parse(data []byte, getenv func(string) string) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	// KnownFields faz uma chave desconhecida (ex.: "pool_interval") virar erro
	// em vez de ser ignorada em silêncio.
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("lendo YAML: %w", err)
	}

	cfg.LinearAPIKey = getenv("LINEAR_API_KEY")
	cfg.GitHubToken = getenv("GITHUB_TOKEN")

	// Coletamos todos os erros de uma vez: é mais útil ver a lista inteira
	// do que corrigir um, rodar de novo, descobrir o próximo...
	errs := cfg.validate()
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	var err error
	if cfg.DBPath, err = expandHome(cfg.DBPath); err != nil {
		return nil, err
	}
	if cfg.CentralRepoPath, err = expandHome(cfg.CentralRepoPath); err != nil {
		return nil, err
	}
	return &cfg, nil
}

var (
	slugRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	// owner/nome no formato aceito pelo GitHub.
	repoRegex = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)
)

// validate devolve todos os problemas encontrados (lista vazia = config ok).
// Também preenche PollInterval e StaleAfter quando conseguem ser convertidos.
func (c *Config) validate() []error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	if c.LinearAPIKey == "" {
		add("variável de ambiente LINEAR_API_KEY não definida")
	}
	if c.GitHubToken == "" {
		add("variável de ambiente GITHUB_TOKEN não definida")
	}

	if d, err := parsePositiveDuration("poll_interval", c.PollIntervalRaw); err != nil {
		errs = append(errs, err)
	} else {
		c.PollInterval = d
	}
	if d, err := parsePositiveDuration("stale_after", c.StaleAfterRaw); err != nil {
		errs = append(errs, err)
	} else {
		c.StaleAfter = d
	}

	if strings.TrimSpace(c.Listen) == "" {
		add("listen é obrigatório (ex.: 127.0.0.1:7777)")
	}
	if strings.TrimSpace(c.DBPath) == "" {
		add("db_path é obrigatório (ex.: ./painel.db)")
	}
	if strings.TrimSpace(c.CentralRepoPath) == "" {
		add("central_repo_path é obrigatório")
	}

	errs = append(errs, c.validateProdutos()...)
	errs = append(errs, c.Risco.validate()...)
	return errs
}

func (c *Config) validateProdutos() []error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	if len(c.Produtos) == 0 {
		add("produtos: configure pelo menos um produto")
		return errs
	}

	slugsVistos := map[string]bool{}
	// repo -> "slug/escopo" de onde ele apareceu primeiro, para a mensagem de erro.
	reposVistos := map[string]string{}

	for i, p := range c.Produtos {
		// Identifica o produto pelo slug quando há, senão pela posição na lista.
		ref := fmt.Sprintf("produtos[%d]", i)
		if p.Slug != "" {
			ref = fmt.Sprintf("produto %q", p.Slug)
		}

		if strings.TrimSpace(p.Nome) == "" {
			add("%s: nome é obrigatório", ref)
		}
		switch {
		case p.Slug == "":
			add("%s: slug é obrigatório", ref)
		case !slugRegex.MatchString(p.Slug):
			add("%s: slug deve ter só letras minúsculas, números e hífens (ex.: produto-x)", ref)
		case slugsVistos[p.Slug]:
			add("%s: slug duplicado", ref)
		}
		slugsVistos[p.Slug] = true

		if strings.TrimSpace(p.LinearProjectID) == "" {
			add("%s: linear_project_id é obrigatório", ref)
		}

		if len(p.Escopos) == 0 {
			add("%s: configure pelo menos um escopo", ref)
		}
		escoposVistos := map[string]bool{}
		for _, e := range p.Escopos {
			switch {
			case e.Nome == "":
				add("%s: escopo com nome vazio", ref)
			case !slugRegex.MatchString(e.Nome):
				add("%s: escopo %q deve ter só letras minúsculas, números e hífens", ref, e.Nome)
			case e.Nome == EscopoReservado || e.Nome == EscopoAClassificar:
				add("%s: %q é um nome reservado (usado pela página de decisões)", ref, e.Nome)
			case escoposVistos[e.Nome]:
				add("%s: escopo %q duplicado", ref, e.Nome)
			}
			escoposVistos[e.Nome] = true

			onde := p.Slug + "/" + e.Nome
			switch {
			case !repoRegex.MatchString(e.Repo):
				add("%s: repo %q do escopo %q deve estar no formato owner/repo", ref, e.Repo, e.Nome)
			case reposVistos[strings.ToLower(e.Repo)] != "":
				// Um repo em dois escopos tornaria ambígua a regra "escopo da issue = escopo do repo do PR".
				add("%s: repo %q do escopo %q já foi usado em %s", ref, e.Repo, e.Nome, reposVistos[strings.ToLower(e.Repo)])
			default:
				// GitHub não diferencia maiúsculas no nome do repo, então comparamos em minúsculas.
				reposVistos[strings.ToLower(e.Repo)] = onde
			}
		}
	}
	return errs
}

func (r *Risco) validate() []error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	if r.LimiteLinhasDiff <= 0 {
		add("risco.limite_linhas_diff deve ser maior que zero")
	}
	for i, regra := range r.Regras {
		ref := fmt.Sprintf("risco.regras[%d]", i)
		if regra.Padrao == "" {
			add("%s: padrao é obrigatório", ref)
		} else if !doublestar.ValidatePattern(regra.Padrao) {
			add("%s: padrao %q não é um glob válido", ref, regra.Padrao)
		}
		if regra.Nivel != NivelAlto && regra.Nivel != NivelMedio {
			add("%s: nivel %q inválido (use %q ou %q)", ref, regra.Nivel, NivelAlto, NivelMedio)
		}
		if strings.TrimSpace(regra.Motivo) == "" {
			add("%s: motivo é obrigatório (ele aparece na tela explicando o risco)", ref)
		}
	}
	for i, padrao := range r.PadroesDeTeste {
		if !doublestar.ValidatePattern(padrao) {
			add("risco.padroes_de_teste[%d]: %q não é um glob válido", i, padrao)
		}
	}
	return errs
}

func parsePositiveDuration(campo, valor string) (time.Duration, error) {
	if valor == "" {
		return 0, fmt.Errorf("%s é obrigatório (ex.: 5m, 72h)", campo)
	}
	d, err := time.ParseDuration(valor)
	if err != nil {
		return 0, fmt.Errorf("%s: %q não é uma duração válida (ex.: 5m, 72h)", campo, valor)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s deve ser maior que zero", campo)
	}
	return d, nil
}

// expandHome troca um "~" no início do caminho pela pasta home do usuário.
// O Go não faz isso sozinho: quem expande "~" normalmente é o shell.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expandindo ~ em %q: %w", path, err)
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}
