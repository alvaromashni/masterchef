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
	"time"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/product"
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
	cfg    *config.Config
	store  *store.Store
	logger *slog.Logger
	// markdown lê PRODUCT.md e DECISIONS.md do repo central a cada pedido,
	// então editar o arquivo e recarregar a página já mostra a mudança.
	markdown *product.Leitor
	// Um template por página. Cada página é o layout + o arquivo da página,
	// parseados juntos, porque todas definem um bloco "conteudo" e um único
	// conjunto de templates não aceitaria vários blocos com o mesmo nome.
	paginas map[string]*template.Template
}

// New prepara os templates e devolve o servidor pronto para Handler().
func New(cfg *config.Config, st *store.Store, logger *slog.Logger) (*Server, error) {
	s := &Server{
		cfg:      cfg,
		store:    st,
		logger:   logger,
		markdown: product.NewLeitor(cfg.CentralRepoPath),
		paginas:  map[string]*template.Template{},
	}

	// Funções que os templates podem chamar, ex.: {{haQuanto .Sync.UltimoOK}}.
	funcoes := template.FuncMap{
		"dataHora": dataHora,
		"haQuanto": func(t time.Time) string { return haQuanto(time.Now(), t) },
		"risco":    textoRisco,
		"rfc3339":  func(t time.Time) string { return t.UTC().Format(time.RFC3339) },
		"escopo": func(nome string) string {
			if nome == "" {
				return "a classificar"
			}
			return nome
		},
		// linkEscopo monta o endereço da página de um escopo ("" = a classificar).
		"linkEscopo": linkEscopo,
	}

	for _, pagina := range []string{"index.html", "prs.html", "mudancas.html", "escopo.html", "decisoes.html"} {
		tmpl, err := template.New(pagina).Funcs(funcoes).ParseFS(templatesFS, "templates/layout.html", "templates/"+pagina)
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
	mux.HandleFunc("GET /prs", s.handlePRs)
	mux.HandleFunc("GET /mudancas", s.handleMudancas)
	mux.HandleFunc("POST /mudancas/visto", s.handleMarcarVisto)
	// O padrão mais específico ("decisoes" fixo) vence o genérico ({escopo}).
	mux.HandleFunc("GET /p/{produto}/decisoes", s.handleDecisoes)
	mux.HandleFunc("GET /p/{produto}/{escopo}", s.handleEscopo)

	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	return mux
}

// dadosPagina é o que todo template recebe. Sync alimenta o aviso do topo,
// que aparece em todas as páginas (seção 11 do CONTEXT.md).
type dadosPagina struct {
	Titulo   string
	Sync     store.EstadoSync
	Conteudo any
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	d, err := s.carregarDados(r)
	if err != nil {
		s.erroInterno(w, err)
		return
	}
	s.render(w, r, "index.html", "Matriz", montarMatriz(s.cfg.Produtos, d, s.cfg.StaleAfter, time.Now()))
}

func (s *Server) handlePRs(w http.ResponseWriter, r *http.Request) {
	d, err := s.carregarDados(r)
	if err != nil {
		s.erroInterno(w, err)
		return
	}
	s.render(w, r, "prs.html", "Fila de review", montarFila(d))
}

// carregarDados lê issues, PRs e vínculos do banco. Com poucos produtos,
// ler tudo e montar as telas em Go é mais simples que uma SQL para cada
// tela, e deixa as regras de domínio num lugar só (testáveis sem banco).
func (s *Server) carregarDados(r *http.Request) (dadosPainel, error) {
	ctx := r.Context()
	var d dadosPainel
	var err error
	if d.Issues, err = s.store.ListarIssues(ctx); err != nil {
		return d, err
	}
	if d.PRs, err = s.store.ListarPRs(ctx); err != nil {
		return d, err
	}
	if d.Vinculos, err = s.store.ListarVinculos(ctx); err != nil {
		return d, err
	}
	d.NaoVistos, err = s.eventosNaoVistos(r)
	return d, err
}

// eventosNaoVistos devolve os eventos depois de last_visit.
func (s *Server) eventosNaoVistos(r *http.Request) ([]store.Evento, error) {
	visita, err := s.store.UltimaVisita(r.Context())
	if err != nil {
		return nil, err
	}
	return s.store.EventosDesde(r.Context(), visita)
}

// handleMudancas mostra os eventos não vistos. Abrir a página NÃO marca
// nada como visto (seção 11): só o botão faz isso.
func (s *Server) handleMudancas(w http.ResponseWriter, r *http.Request) {
	eventos, err := s.eventosNaoVistos(r)
	if err != nil {
		s.erroInterno(w, err)
		return
	}
	s.render(w, r, "mudancas.html", "O que mudou", montarMudancas(s.cfg.Produtos, eventos))
}

// handleMarcarVisto grava last_visit com a data enviada pelo formulário
// (a do evento mais recente que estava na tela).
func (s *Server) handleMarcarVisto(w http.ResponseWriter, r *http.Request) {
	ate, err := time.Parse(time.RFC3339, r.FormValue("ate"))
	if err != nil {
		http.Error(w, "parâmetro 'ate' inválido", http.StatusBadRequest)
		return
	}

	// Nunca voltar no tempo: um formulário antigo (outra aba) não pode
	// fazer eventos já vistos reaparecerem.
	visita, err := s.store.UltimaVisita(r.Context())
	if err != nil {
		s.erroInterno(w, err)
		return
	}
	if ate.After(visita) {
		if err := s.store.MarcarVistoAte(r.Context(), ate); err != nil {
			s.erroInterno(w, err)
			return
		}
	}

	// Com htmx, devolvemos só o pedaço da página que muda. Sem JavaScript
	// (formulário comum), redirecionamos de volta para a página.
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<div id="mudancas"><p class="vazio">Tudo marcado como visto.</p></div>`)
		return
	}
	http.Redirect(w, r, "/mudancas", http.StatusSeeOther)
}

// handleEscopo mostra a página de uma célula da matriz.
func (s *Server) handleEscopo(w http.ResponseWriter, r *http.Request) {
	p, ok := s.produto(r.PathValue("produto"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	alvo := r.PathValue("escopo")
	if alvo == config.EscopoAClassificar {
		alvo = aClassificar
	} else if celulaDe(p, alvo) != alvo {
		http.NotFound(w, r) // escopo que este produto não tem
		return
	}

	d, err := s.carregarDados(r)
	if err != nil {
		s.erroInterno(w, err)
		return
	}
	eventos, err := s.store.EventosDoProduto(r.Context(), p.Slug, 500)
	if err != nil {
		s.erroInterno(w, err)
		return
	}

	doc := product.Documento{Aviso: "Issues sem escopo não têm seção no PRODUCT.md."}
	if alvo != aClassificar {
		doc = s.markdown.SecaoDoEscopo(p.Slug, alvo)
	}
	titulo := p.Nome + " · " + alvo
	if alvo == aClassificar {
		titulo = p.Nome + " · a classificar"
	}
	s.render(w, r, "escopo.html", titulo, montarPaginaEscopo(p, alvo, d, eventos, doc))
}

// handleDecisoes mostra o DECISIONS.md do produto.
func (s *Server) handleDecisoes(w http.ResponseWriter, r *http.Request) {
	p, ok := s.produto(r.PathValue("produto"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.render(w, r, "decisoes.html", p.Nome+" · decisões", map[string]any{
		"Produto": p,
		"Doc":     s.markdown.Decisoes(p.Slug),
	})
}

// produto acha o produto pelo slug da URL.
func (s *Server) produto(slug string) (config.Produto, bool) {
	for _, p := range s.cfg.Produtos {
		if p.Slug == slug {
			return p, true
		}
	}
	return config.Produto{}, false
}

// erroInterno loga o erro com detalhes e mostra ao usuário só uma mensagem genérica.
func (s *Server) erroInterno(w http.ResponseWriter, err error) {
	s.logger.Error("erro ao montar página", "erro", err)
	http.Error(w, "erro interno", http.StatusInternalServerError)
}

// render executa o template num buffer antes de escrever a resposta.
// Assim, se o template falhar no meio, devolvemos um 500 limpo em vez
// de uma página cortada pela metade.
func (s *Server) render(w http.ResponseWriter, r *http.Request, pagina, titulo string, conteudo any) {
	sync, err := s.store.LerEstadoSync(r.Context())
	if err != nil {
		s.erroInterno(w, err)
		return
	}
	dados := dadosPagina{Titulo: titulo, Sync: sync, Conteudo: conteudo}

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
