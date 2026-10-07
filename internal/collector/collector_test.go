package collector

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/escopo"
	"github.com/alvaromashni/masterchef/internal/github"
	"github.com/alvaromashni/masterchef/internal/linear"
	"github.com/alvaromashni/masterchef/internal/store"
)

// linearFalso responde com fixtures por projeto e anota o "desde" recebido.
type linearFalso struct {
	issues     map[string][]linear.Issue // projectID -> issues
	falhas     map[string]error          // projectID -> erro a devolver
	desdeVisto []*time.Time
}

func (f *linearFalso) ListIssues(_ context.Context, projectID string, desde *time.Time) ([]linear.Issue, error) {
	f.desdeVisto = append(f.desdeVisto, desde)
	if err := f.falhas[projectID]; err != nil {
		return nil, err
	}
	return f.issues[projectID], nil
}

// githubFalso devolve PRs e arquivos fixos por repo e conta as chamadas.
type githubFalso struct {
	prs              map[string][]github.PR   // repo -> PRs
	arquivos         map[int][]github.Arquivo // número do PR -> arquivos
	falhas           map[string]error         // repo -> erro
	chamadasArquivos int
}

func (f *githubFalso) ListPRs(_ context.Context, repo string, _ *time.Time) ([]github.PR, error) {
	if err := f.falhas[repo]; err != nil {
		return nil, err
	}
	return f.prs[repo], nil
}

func (f *githubFalso) ListArquivos(_ context.Context, _ string, number int) ([]github.Arquivo, error) {
	f.chamadasArquivos++
	return f.arquivos[number], nil
}

// carregarFixture lê um JSON de testdata/ para dentro de destino.
func carregarFixture[T any](t *testing.T, nome string) T {
	t.Helper()
	dados, err := os.ReadFile(filepath.Join("testdata", nome))
	if err != nil {
		t.Fatal(err)
	}
	var destino T
	if err := json.Unmarshal(dados, &destino); err != nil {
		t.Fatalf("fixture %s: %v", nome, err)
	}
	return destino
}

// montar cria um coletor com dois produtos, store SQLite num arquivo
// temporário (sem rede) e um relógio controlado pelo teste.
func montar(t *testing.T) (*Collector, *linearFalso, *githubFalso, *store.Store, *time.Time) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	st, err := store.Open(filepath.Join(t.TempDir(), "teste.db"), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	fonte := &linearFalso{
		issues: map[string][]linear.Issue{
			"proj-x": carregarFixture[[]linear.Issue](t, "issues_produto_x.json"),
			"proj-y": carregarFixture[[]linear.Issue](t, "issues_produto_y.json"),
		},
		falhas: map[string]error{},
	}
	gh := &githubFalso{
		prs: map[string][]github.PR{
			"o/x-api": carregarFixture[[]github.PR](t, "prs_produto_x_api.json"),
		},
		arquivos: map[int][]github.Arquivo{
			7: {{Nome: "db/migrations/0003.sql", Additions: 20, Deletions: 2}},
		},
		falhas: map[string]error{},
	}
	cfg := &config.Config{
		PollInterval: time.Minute,
		Produtos: []config.Produto{
			{Slug: "produto-x", LinearProjectID: "proj-x", Escopos: config.Escopos{
				{Nome: "api", Repo: "o/x-api"}, {Nome: "front", Repo: "o/x-front"}, {Nome: "infra", Repo: "o/x-infra"},
			}},
			{Slug: "produto-y", LinearProjectID: "proj-y", Escopos: config.Escopos{{Nome: "app", Repo: "o/y-app"}}},
		},
		Risco: config.Risco{
			LimiteLinhasDiff: 400,
			Regras:           []config.RegraRisco{{Padrao: "**/migrations/**", Nivel: "alto", Motivo: "Altera migração"}},
		},
	}
	c := New(cfg, fonte, gh, st, logger)

	relogio := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	c.agora = func() time.Time { return relogio }
	return c, fonte, gh, st, &relogio
}

// contagens aplica a regra de escopo ao que está gravado e devolve
// "produto/escopo" -> issues em andamento (escopo "" = a classificar).
func contagens(t *testing.T, st *store.Store) map[string]int {
	t.Helper()
	ctx := context.Background()
	issues, err := st.ListarIssues(ctx)
	if err != nil {
		t.Fatal(err)
	}
	prs, _ := st.ListarPRs(ctx)
	vinculos, _ := st.ListarVinculos(ctx)

	escopoDoPR := map[int64]string{}
	for _, p := range prs {
		escopoDoPR[p.ID] = p.Scope
	}
	escoposPRsDaIssue := map[string][]string{}
	for _, v := range vinculos {
		escoposPRsDaIssue[v.IssueID] = append(escoposPRsDaIssue[v.IssueID], escopoDoPR[v.PRID])
	}

	m := map[string]int{}
	for _, i := range issues {
		if i.StateType != "started" {
			continue
		}
		for _, e := range escopo.DaIssue(escoposPRsDaIssue[i.ID], i.ScopeLabel) {
			m[i.ProductSlug+"/"+e]++
		}
	}
	return m
}

func TestCiclo_PrimeiroSync(t *testing.T) {
	c, fonte, _, st, relogio := montar(t)
	ctx := context.Background()

	if err := c.Ciclo(ctx); err != nil {
		t.Fatalf("Ciclo: %v", err)
	}

	// Primeiro sync não tem "desde": o Linear deve devolver tudo.
	for i, d := range fonte.desdeVisto {
		if d != nil {
			t.Errorf("chamada %d passou desde=%v no primeiro sync, esperava nil", i, d)
		}
	}

	got := contagens(t, st)
	quer := map[string]int{
		// PX-1 (label api) + PX-3 (sem label, mas o PR #7 do repo api cita
		// "PX-3" no título: o PR tem prioridade). PX-4 está no backlog.
		"produto-x/api":   2,
		"produto-x/infra": 1,
		"produto-y/app":   1,
	}
	if len(got) != len(quer) {
		t.Errorf("contagens = %v, esperava %v", got, quer)
	}
	for k, v := range quer {
		if got[k] != v {
			t.Errorf("%s = %d, esperava %d", k, got[k], v)
		}
	}

	estado, _ := st.LerEstadoSync(ctx)
	if !estado.UltimoOK.Equal(*relogio) {
		t.Errorf("last_sync_ok = %v, esperava %v", estado.UltimoOK, *relogio)
	}
	if estado.UltimoErro != "" {
		t.Errorf("não deveria haver erro: %q", estado.UltimoErro)
	}
}

func TestCiclo_SegundoSyncPedeSoMudancas(t *testing.T) {
	c, fonte, _, _, relogio := montar(t)
	ctx := context.Background()

	primeiro := *relogio
	if err := c.Ciclo(ctx); err != nil {
		t.Fatal(err)
	}
	*relogio = relogio.Add(5 * time.Minute)
	fonte.desdeVisto = nil
	if err := c.Ciclo(ctx); err != nil {
		t.Fatal(err)
	}

	for i, d := range fonte.desdeVisto {
		if d == nil || !d.Equal(primeiro) {
			t.Errorf("chamada %d: desde = %v, esperava %v", i, d, primeiro)
		}
	}
}

func TestCiclo_ErroEmUmProdutoNaoDerrubaOsOutros(t *testing.T) {
	c, fonte, _, st, relogio := montar(t)
	ctx := context.Background()

	// Um sync ok antes, para ver que last_sync_ok NÃO avança no sync com erro.
	if err := c.Ciclo(ctx); err != nil {
		t.Fatal(err)
	}
	syncOK := *relogio

	*relogio = relogio.Add(5 * time.Minute)
	fonte.falhas["proj-x"] = errors.New("HTTP 500")
	// Produto Y mudou: a issue PY-1 foi concluída.
	fonte.issues["proj-y"][0].StateType = "completed"

	if err := c.Ciclo(ctx); err != nil {
		t.Fatalf("Ciclo não deveria falhar por causa de um produto: %v", err)
	}

	// O produto Y foi gravado mesmo com o X falhando.
	if n := contagens(t, st)["produto-y/app"]; n != 0 {
		t.Errorf("PY-1 deveria ter saído de 'em andamento', contagem = %d", n)
	}

	estado, _ := st.LerEstadoSync(ctx)
	if !estado.UltimoOK.Equal(syncOK) {
		t.Errorf("last_sync_ok avançou para %v, deveria continuar %v", estado.UltimoOK, syncOK)
	}
	if !strings.Contains(estado.UltimoErro, "produto-x") || !strings.Contains(estado.UltimoErro, "HTTP 500") {
		t.Errorf("last_sync_error = %q, esperava mencionar produto-x e o erro", estado.UltimoErro)
	}

	// Quando o produto volta a funcionar, o aviso de erro some.
	*relogio = relogio.Add(5 * time.Minute)
	delete(fonte.falhas, "proj-x")
	if err := c.Ciclo(ctx); err != nil {
		t.Fatal(err)
	}
	estado, _ = st.LerEstadoSync(ctx)
	if estado.UltimoErro != "" || !estado.UltimoOK.Equal(*relogio) {
		t.Errorf("depois de um sync ok: erro=%q, ok=%v", estado.UltimoErro, estado.UltimoOK)
	}
}

func TestCiclo_IssueMudaDeEscopoPelaLabel(t *testing.T) {
	c, fonte, _, st, relogio := montar(t)
	ctx := context.Background()
	if err := c.Ciclo(ctx); err != nil {
		t.Fatal(err)
	}

	// PX-2 trocou a label de infra para front.
	*relogio = relogio.Add(time.Minute)
	fonte.issues["proj-x"][1].Labels = []string{"scope:front"}
	if err := c.Ciclo(ctx); err != nil {
		t.Fatal(err)
	}

	got := contagens(t, st)
	if got["produto-x/infra"] != 0 || got["produto-x/front"] != 1 {
		t.Errorf("contagens = %v; esperava infra=0 e front=1", got)
	}
}

func TestCiclo_PRGanhaRiscoEVinculo(t *testing.T) {
	c, _, _, st, _ := montar(t)
	ctx := context.Background()
	if err := c.Ciclo(ctx); err != nil {
		t.Fatal(err)
	}

	prs, err := st.ListarPRs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 {
		t.Fatalf("esperava 1 PR gravado, veio %d", len(prs))
	}
	pr := prs[0]
	if pr.ProductSlug != "produto-x" || pr.Scope != "api" {
		t.Errorf("PR no lugar errado: %s/%s", pr.ProductSlug, pr.Scope)
	}
	if pr.RiskLevel != "alto" || len(pr.RiskReasons) == 0 || !strings.Contains(pr.RiskReasons[0], "Altera migração") {
		t.Errorf("risco = %s %v, esperava alto por migração", pr.RiskLevel, pr.RiskReasons)
	}
	if pr.Additions != 20 || pr.Deletions != 2 {
		t.Errorf("linhas = +%d -%d, esperava +20 -2 (soma dos arquivos)", pr.Additions, pr.Deletions)
	}

	vinculos, _ := st.ListarVinculos(ctx)
	if len(vinculos) != 1 || vinculos[0].IssueID != "x-3" || vinculos[0].PRID != 501 {
		t.Errorf("vínculos = %+v, esperava x-3 ↔ 501", vinculos)
	}
}

func TestCiclo_PRSemMudancaNaoBuscaArquivosDeNovo(t *testing.T) {
	c, _, gh, _, relogio := montar(t)
	ctx := context.Background()
	if err := c.Ciclo(ctx); err != nil {
		t.Fatal(err)
	}
	if gh.chamadasArquivos != 1 {
		t.Fatalf("1º ciclo buscou arquivos %d vezes, esperava 1", gh.chamadasArquivos)
	}

	*relogio = relogio.Add(time.Minute)
	if err := c.Ciclo(ctx); err != nil {
		t.Fatal(err)
	}
	if gh.chamadasArquivos != 1 {
		t.Errorf("PR sem mudança não deveria buscar arquivos de novo (chamadas = %d)", gh.chamadasArquivos)
	}

	// Um push muda o updated_at: agora sim busca de novo.
	*relogio = relogio.Add(time.Minute)
	gh.prs["o/x-api"][0].UpdatedAt = gh.prs["o/x-api"][0].UpdatedAt.Add(time.Hour)
	if err := c.Ciclo(ctx); err != nil {
		t.Fatal(err)
	}
	if gh.chamadasArquivos != 2 {
		t.Errorf("PR alterado deveria buscar arquivos de novo (chamadas = %d)", gh.chamadasArquivos)
	}
}

func TestCiclo_ErroEmUmRepoNaoDerrubaOCiclo(t *testing.T) {
	c, _, gh, st, _ := montar(t)
	ctx := context.Background()
	gh.falhas["o/x-front"] = errors.New("HTTP 404")

	if err := c.Ciclo(ctx); err != nil {
		t.Fatalf("Ciclo não deveria falhar por causa de um repo: %v", err)
	}
	prs, _ := st.ListarPRs(ctx)
	if len(prs) != 1 {
		t.Errorf("o PR do repo que funcionou deveria ter sido gravado")
	}
	estado, _ := st.LerEstadoSync(ctx)
	if !strings.Contains(estado.UltimoErro, "o/x-front") || !estado.UltimoOK.IsZero() {
		t.Errorf("erro = %q, ok = %v; esperava erro do repo e last_sync_ok vazio", estado.UltimoErro, estado.UltimoOK)
	}
}
