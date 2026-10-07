package product

import (
	"strings"
	"testing"
)

func TestExtrairSecao(t *testing.T) {
	md := "# Título\n\nintro\n\n## api\n\nlinha api\n\n### sub\nsub api\n\n## front\nlinha front\n"

	casos := []struct {
		titulo string
		quer   string
		achou  bool
	}{
		{"api", "linha api\n\n### sub\nsub api", true},
		{" API ", "linha api\n\n### sub\nsub api", true}, // maiúsculas e espaços
		{"front", "linha front", true},
		{"infra", "", false},
	}
	for _, c := range casos {
		got, achou := ExtrairSecao(md, c.titulo)
		if got != c.quer || achou != c.achou {
			t.Errorf("ExtrairSecao(%q) = (%q, %v), esperava (%q, %v)", c.titulo, got, achou, c.quer, c.achou)
		}
	}
}

func TestSecaoDoEscopo(t *testing.T) {
	l := NewLeitor("testdata/central")

	t.Run("seção existente vira HTML", func(t *testing.T) {
		doc := l.SecaoDoEscopo("produto-x", "api")
		html := string(doc.HTML)
		if doc.Aviso != "" {
			t.Fatalf("aviso inesperado: %s", doc.Aviso)
		}
		for _, trecho := range []string{"<strong>já faz</strong>", "<li>Login com token</li>", "<h3>Pendências</h3>", `type="checkbox"`} {
			if !strings.Contains(html, trecho) {
				t.Errorf("HTML não contém %q:\n%s", trecho, html)
			}
		}
		if strings.Contains(html, "front") {
			t.Errorf("HTML da api não deveria trazer a seção front:\n%s", html)
		}
	})

	t.Run("bloco de código com ## não corta a seção", func(t *testing.T) {
		doc := l.SecaoDoEscopo("produto-x", "front")
		if !strings.Contains(string(doc.HTML), "Fim do front.") {
			t.Errorf("seção front foi cortada:\n%s", doc.HTML)
		}
	})

	t.Run("seção vazia vira aviso", func(t *testing.T) {
		doc := l.SecaoDoEscopo("produto-x", "infra")
		if doc.HTML != "" || !strings.Contains(doc.Aviso, "Nada escrito") {
			t.Errorf("esperava aviso de seção vazia, veio %+v", doc)
		}
	})

	t.Run("seção ausente vira aviso", func(t *testing.T) {
		doc := l.SecaoDoEscopo("produto-x", "mobile")
		if doc.HTML != "" || !strings.Contains(doc.Aviso, `"## mobile"`) {
			t.Errorf("esperava aviso de seção ausente, veio %+v", doc)
		}
	})

	t.Run("arquivo ausente vira aviso, não erro", func(t *testing.T) {
		doc := l.SecaoDoEscopo("produto-y", "api")
		if doc.HTML != "" || !strings.Contains(doc.Aviso, "não encontrado") || !strings.Contains(doc.Aviso, "PRODUCT.md") {
			t.Errorf("esperava aviso de arquivo ausente, veio %+v", doc)
		}
	})
}

func TestDecisoes(t *testing.T) {
	l := NewLeitor("testdata/central")

	doc := l.Decisoes("produto-x")
	html := string(doc.HTML)
	if !strings.Contains(html, "SQLite em vez de Postgres") {
		t.Errorf("DECISIONS.md não renderizado:\n%s", html)
	}
	if strings.Contains(html, "<script>") {
		t.Errorf("HTML cru do Markdown não pode chegar à página:\n%s", html)
	}

	if doc := l.Decisoes("produto-y"); !strings.Contains(doc.Aviso, "DECISIONS.md") {
		t.Errorf("esperava aviso de arquivo ausente, veio %+v", doc)
	}
}
