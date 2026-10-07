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

func carregarFixture(t *testing.T, nome string) []linear.Issue {
	t.Helper()
	dados, err := os.ReadFile(filepath.Join("testdata", nome))
	if err != nil {
		t.Fatal(err)
	}
	var issues []linear.Issue
	if err := json.Unmarshal(dados, &issues); err != nil {
		t.Fatalf("fixture %s: %v", nome, err)
	}
	return issues
}

// montar cria um coletor com dois produtos, store SQLite num arquivo
// temporário (sem rede) e um relógio controlado pelo teste.
func montar(t *testing.T) (*Collector, *linearFalso, *store.Store, *time.Time) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	st, err := store.Open(filepath.Join(t.TempDir(), "teste.db"), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	fonte := &linearFalso{
		issues: map[string][]linear.Issue{
			"proj-x": carregarFixture(t, "issues_produto_x.json"),
			"proj-y": carregarFixture(t, "issues_produto_y.json"),
		},
		falhas: map[string]error{},
	}
	cfg := &config.Config{
		PollInterval: time.Minute,
		Produtos: []config.Produto{
			{Slug: "produto-x", LinearProjectID: "proj-x"},
			{Slug: "produto-y", LinearProjectID: "proj-y"},
		},
	}
	c := New(cfg, fonte, st, logger)

	relogio := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	c.agora = func() time.Time { return relogio }
	return c, fonte, st, &relogio
}

// contagens transforma o resultado do store num mapa "produto/escopo" -> n.
func contagens(t *testing.T, st *store.Store) map[string]int {
	t.Helper()
	cs, err := st.ContarEmAndamento(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]int{}
	for _, c := range cs {
		m[c.ProductSlug+"/"+c.Escopo] = c.EmAndamento
	}
	return m
}

func TestCiclo_PrimeiroSync(t *testing.T) {
	c, fonte, st, relogio := montar(t)
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
		"produto-x/api":   1, // PX-4 está no backlog, não conta
		"produto-x/infra": 1,
		"produto-x/":      1, // sem label → a classificar
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
	c, fonte, _, relogio := montar(t)
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
	c, fonte, st, relogio := montar(t)
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

func TestCiclo_IssueMudaDeEscopo(t *testing.T) {
	c, fonte, st, relogio := montar(t)
	ctx := context.Background()
	if err := c.Ciclo(ctx); err != nil {
		t.Fatal(err)
	}

	// PX-3 ganhou a label scope:api: sai de "a classificar" e entra em api.
	*relogio = relogio.Add(time.Minute)
	fonte.issues["proj-x"][2].Labels = []string{"scope:api"}
	if err := c.Ciclo(ctx); err != nil {
		t.Fatal(err)
	}

	got := contagens(t, st)
	if got["produto-x/"] != 0 || got["produto-x/api"] != 2 {
		t.Errorf("contagens = %v; esperava a classificar=0 e api=2", got)
	}
}
