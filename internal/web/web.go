// Package web tem os handlers HTTP e os templates do painel.
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/alvaromashni/masterchef/internal/store"
)

// Templates e CSS ficam embutidos no binário (mesmo motivo das migrações:
// um único executável, nada para copiar junto).
//
//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

// Server agrupa o que os handlers precisam para responder.
type Server struct {
	store  *store.Store
	logger *slog.Logger
	// Um template por página. Cada página é o layout + o arquivo da página,
	// parseados juntos, porque todas definem um bloco "conteudo" e um único
	// conjunto de templates não aceitaria vários blocos com o mesmo nome.
	paginas map[string]*template.Template
}

// New prepara os templates e devolve o servidor pronto para Handler().
func New(st *store.Store, logger *slog.Logger) (*Server, error) {
	s := &Server{store: st, logger: logger, paginas: map[string]*template.Template{}}

	for _, pagina := range []string{"index.html"} {
		tmpl, err := template.ParseFS(templatesFS, "templates/layout.html", "templates/"+pagina)
		if err != nil {
			return nil, fmt.Errorf("carregando template %s: %w", pagina, err)
		}
		s.paginas[pagina] = tmpl
	}
	return s, nil
}

// Handler devolve o roteador com todas as rotas do painel.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Desde o Go 1.22 o ServeMux aceita método e caminho no padrão.
	// "{$}" faz "/" casar só com a raiz, e não com qualquer caminho.
	mux.HandleFunc("GET /{$}", s.handleIndex)

	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.render(w, "index.html", map[string]any{
		"Titulo": "Matriz",
	})
}

// render executa o template num buffer antes de escrever a resposta.
// Assim, se o template falhar no meio, devolvemos um 500 limpo em vez
// de uma página cortada pela metade.
func (s *Server) render(w http.ResponseWriter, pagina string, dados any) {
	tmpl, ok := s.paginas[pagina]
	if !ok {
		s.logger.Error("template inexistente", "pagina", pagina)
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "layout", dados); err != nil {
		s.logger.Error("renderizando página", "pagina", pagina, "erro", err)
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}
