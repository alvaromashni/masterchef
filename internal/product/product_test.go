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
		if doc.HTML != "" || !strings.Contains(doc.Aviso, "não foi encontrado") || !strings.Contains(doc.Aviso, "PRODUCT.md") {
			t.Errorf("esperava aviso de arquivo ausente, veio %+v", doc)
		}
	})
}

func TestDecisoes(t *testing.T) {
	l := NewLeitor("testdata/central")

	d := l.Decisoes("produto-x")
	if d.Aviso != "" {
		t.Fatalf("aviso inesperado: %s", d.Aviso)
	}
	if d.Caminho != "produtos/produto-x/DECISIONS.md" {
		t.Errorf("Caminho = %q", d.Caminho)
	}
	if !strings.Contains(string(d.Intro), "Registro das decisões") || strings.Contains(string(d.Intro), "<h1>") {
		t.Errorf("Intro deveria ter o texto e não o título # :\n%s", d.Intro)
	}
	if len(d.Itens) != 2 {
		t.Fatalf("esperava 2 decisões (o ## dentro do bloco de código não conta), veio %d: %+v", len(d.Itens), d.Itens)
	}
	if got := d.Itens[0]; got.Data != "2026-10-01" || got.Titulo != "SQLite em vez de Postgres" || got.Ancora != "decisao-1" {
		t.Errorf("primeira decisão = %+v", got)
	}
	if got := d.Itens[1]; got.Data != "" || got.Titulo != "Sem login social no MVP" || !strings.Contains(string(got.HTML), "<li>E-mail e senha</li>") {
		t.Errorf("decisão sem data = %+v", got)
	}
	if strings.Contains(string(d.Itens[0].HTML), "<script>") {
		t.Errorf("HTML cru do Markdown não pode chegar à página:\n%s", d.Itens[0].HTML)
	}

	if d := l.Decisoes("produto-y"); !strings.Contains(d.Aviso, "produtos/produto-y/DECISIONS.md não foi encontrado") {
		t.Errorf("esperava aviso de arquivo ausente, veio %+v", d)
	}
}
