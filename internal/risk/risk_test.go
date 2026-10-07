package risk

import (
	"strings"
	"testing"

	"github.com/alvaromashni/masterchef/internal/config"
)

// cfgPadrao é a mesma config de risco do exemplo do CONTEXT.md.
var cfgPadrao = config.Risco{
	LimiteLinhasDiff: 400,
	Regras: []config.RegraRisco{
		{Padrao: "**/migrations/**", Nivel: "alto", Motivo: "Altera migração de banco"},
		{Padrao: "**/auth/**", Nivel: "alto", Motivo: "Mexe em autenticação/permissões"},
		{Padrao: "**/Dockerfile", Nivel: "medio", Motivo: "Altera imagem de container"},
		{Padrao: ".github/workflows/**", Nivel: "medio", Motivo: "Altera CI/CD"},
	},
	PadroesDeTeste: []string{"**/*_test.go", "**/*.test.*", "**/*.spec.*", "**/src/test/**"},
}

func TestAvaliar(t *testing.T) {
	casos := []struct {
		nome     string
		arquivos []string
		linhas   int
		cfg      config.Risco
		nivel    string
		motivos  []string // trechos que devem aparecer, na ordem
	}{
		{
			nome:     "baixo: código com teste e diff pequeno",
			arquivos: []string{"internal/api/handler.go", "internal/api/handler_test.go"},
			linhas:   50,
			nivel:    Baixo,
		},
		{
			nome:     "alto: migração",
			arquivos: []string{"db/migrations/0002_add.sql", "db/repo_test.go"},
			linhas:   10,
			nivel:    Alto,
			motivos:  []string{"Altera migração de banco (db/migrations/0002_add.sql)"},
		},
		{
			nome:     "alto: autenticação em pasta funda",
			arquivos: []string{"src/main/java/com/x/auth/Login.java", "src/test/java/LoginTest.java"},
			linhas:   10,
			nivel:    Alto,
			motivos:  []string{"Mexe em autenticação"},
		},
		{
			nome:     "medio: Dockerfile na raiz (** casa zero pastas)",
			arquivos: []string{"Dockerfile", "app.test.js"},
			linhas:   5,
			nivel:    Medio,
			motivos:  []string{"Altera imagem de container (Dockerfile)"},
		},
		{
			nome:     "medio: workflow de CI",
			arquivos: []string{".github/workflows/ci.yml", "x.spec.ts"},
			linhas:   5,
			nivel:    Medio,
			motivos:  []string{"Altera CI/CD"},
		},
		{
			nome:     "medio: diff acima do limite",
			arquivos: []string{"a.go", "a_test.go"},
			linhas:   401,
			nivel:    Medio,
			motivos:  []string{"Diff grande: 401 linhas alteradas (limite 400)"},
		},
		{
			nome:     "baixo: diff exatamente no limite não conta",
			arquivos: []string{"a.go", "a_test.go"},
			linhas:   400,
			nivel:    Baixo,
		},
		{
			nome:     "medio: sem testes",
			arquivos: []string{"internal/api/handler.go"},
			linhas:   10,
			nivel:    Medio,
			motivos:  []string{"Nenhum arquivo de teste alterado"},
		},
		{
			nome:     "alto mantém os motivos de medio também",
			arquivos: []string{"db/migrations/1.sql", "Dockerfile"},
			linhas:   900,
			nivel:    Alto,
			motivos: []string{
				"Altera migração de banco",
				"Altera imagem de container",
				"Diff grande",
				"Nenhum arquivo de teste alterado",
			},
		},
		{
			nome:     "sem padrões de teste configurados, a regra de testes não se aplica",
			arquivos: []string{"main.go"},
			linhas:   10,
			cfg:      config.Risco{LimiteLinhasDiff: 400},
			nivel:    Baixo,
		},
		{
			nome:     "parecido mas não casa: 'migration' sem s",
			arquivos: []string{"db/migration/1.sql", "x_test.go"},
			linhas:   10,
			nivel:    Baixo,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			cfg := c.cfg
			if cfg.LimiteLinhasDiff == 0 {
				cfg = cfgPadrao
			}
			got := Avaliar(c.arquivos, c.linhas, cfg)

			if got.Nivel != c.nivel {
				t.Errorf("nível = %s, esperava %s (motivos: %v)", got.Nivel, c.nivel, got.Motivos)
			}
			if len(got.Motivos) != len(c.motivos) {
				t.Fatalf("motivos = %v, esperava %d motivos", got.Motivos, len(c.motivos))
			}
			for i, trecho := range c.motivos {
				if !strings.Contains(got.Motivos[i], trecho) {
					t.Errorf("motivo %d = %q, esperava conter %q", i, got.Motivos[i], trecho)
				}
			}
		})
	}
}
