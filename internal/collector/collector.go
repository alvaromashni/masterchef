// Package collector busca dados nas APIs de tempos em tempos e grava no store.
package collector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/escopo"
	"github.com/alvaromashni/masterchef/internal/github"
	"github.com/alvaromashni/masterchef/internal/linear"
	"github.com/alvaromashni/masterchef/internal/risk"
	"github.com/alvaromashni/masterchef/internal/store"
)

// FonteIssues é o que o coletor precisa do Linear.
//
// Em Go, a interface costuma ser declarada por quem USA, não por quem
// implementa (diferente do Java, onde a classe declara "implements").
// O *linear.Client satisfaz esta interface só por ter o método certo, e
// os testes passam uma implementação falsa sem rede.
type FonteIssues interface {
	ListIssues(ctx context.Context, projectID string, updatedSince *time.Time) ([]linear.Issue, error)
}

// FontePRs é o que o coletor precisa do GitHub.
type FontePRs interface {
	ListPRs(ctx context.Context, repo string, updatedSince *time.Time) ([]github.PR, error)
	ListPRsAbertos(ctx context.Context, repo string) ([]github.PR, error)
	ListArquivos(ctx context.Context, repo string, number int) ([]github.Arquivo, error)
}

// Gravador é o que o coletor precisa do store.
type Gravador interface {
	UltimoSyncOK(ctx context.Context) (*time.Time, error)
	ListarIssues(ctx context.Context) ([]store.Issue, error)
	ListarPRs(ctx context.Context) ([]store.PR, error)
	ListarVinculos(ctx context.Context) ([]store.Vinculo, error)
	SalvarSync(ctx context.Context, r store.SyncResultado) error
}

// JanelaPrimeiroSync limita quanto do passado o primeiro sync busca no
// GitHub: PRs fechados ou mergeados há mais tempo que isso ficam de fora
// (abertos entram sempre). Sem limite, o primeiro sync buscaria a história
// inteira de cada repo, com uma chamada de arquivos por PR.
const JanelaPrimeiroSync = 45 * 24 * time.Hour

// Collector roda os ciclos de coleta.
type Collector struct {
	produtos  []config.Produto
	risco     config.Risco
	intervalo time.Duration
	linear    FonteIssues
	github    FontePRs
	store     Gravador
	logger    *slog.Logger
	// agora existe para os testes controlarem o relógio.
	agora func() time.Time
}

// New monta um coletor a partir da config.
func New(cfg *config.Config, issues FonteIssues, prs FontePRs, st Gravador, logger *slog.Logger) *Collector {
	return &Collector{
		produtos:  cfg.Produtos,
		risco:     cfg.Risco,
		intervalo: cfg.PollInterval,
		linear:    issues,
		github:    prs,
		store:     st,
		logger:    logger,
		agora:     time.Now,
	}
}

// Run roda um ciclo agora e depois um a cada poll_interval, até ctx ser
// cancelado. Deve ser chamado numa goroutine: "go c.Run(ctx)".
func (c *Collector) Run(ctx context.Context) {
	c.cicloComLog(ctx)

	ticker := time.NewTicker(c.intervalo)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.cicloComLog(ctx)
		}
	}
}

func (c *Collector) cicloComLog(ctx context.Context) {
	inicio := c.agora()
	if err := c.Ciclo(ctx); err != nil {
		c.logger.Error("ciclo de coleta falhou", "erro", err)
		return
	}
	c.logger.Info("ciclo de coleta concluído", "duracao", time.Since(inicio).Round(time.Millisecond))
}

// Ciclo faz uma coleta completa (seção 10 do CONTEXT.md):
//
//  1. issues de cada produto no Linear;
//  2. PRs de cada repo no GitHub, com risco calculado para os novos/alterados;
//  3. vínculos issue ↔ PR;
//  4. eventos (comparando com o banco ANTES de gravar);
//  5. grava tudo numa transação e atualiza last_sync_ok / last_sync_error.
//
// Um produto ou repo com erro não derruba os outros: o erro é anotado, os
// demais seguem e, no fim, tudo que deu certo é gravado. Mas last_sync_ok só
// avança se TUDO deu certo; senão o próximo ciclo pediria "desde agora" e
// perderia as mudanças de quem falhou.
func (c *Collector) Ciclo(ctx context.Context) error {
	// Guardamos o INÍCIO do ciclo como marca do sync. Se usássemos o fim,
	// algo alterado durante a coleta poderia ficar de fora para sempre.
	inicio := c.agora()

	desde, err := c.store.UltimoSyncOK(ctx)
	if err != nil {
		return err
	}

	// Foto do banco antes de qualquer gravação: base para saber o que mudou.
	antes, vinculosAntes, err := c.lerEstadoAnterior(ctx)
	if err != nil {
		return err
	}

	resultado := store.SyncResultado{Iniciado: inicio}
	anotarErro := func(onde string, err error) {
		c.logger.Error("falha na coleta", "onde", onde, "erro", err)
		resultado.Erros = append(resultado.Erros, fmt.Sprintf("%s: %v", onde, err))
	}

	issuesNovas := c.coletarIssues(ctx, desde, &resultado, anotarErro)
	prsNovos := c.coletarPRs(ctx, desde, antes.PRs, &resultado, anotarErro)
	resultado.Vinculos = calcularVinculos(antes, issuesNovas, prsNovos)

	// Primeiro sync (nunca houve um bem-sucedido): só popular o banco, sem
	// eventos. Senão a tela "o que mudou" viria com o projeto inteiro.
	if desde != nil {
		escopos := escoposDasIssues(resultado.Issues, antes.PRs, resultado.PRs, append(vinculosAntes, resultado.Vinculos...))
		resultado.Eventos = GerarEventos(antes, resultado.Issues, resultado.PRs, escopos, inicio)
	}

	return c.store.SalvarSync(ctx, resultado)
}

// lerEstadoAnterior lê issues, PRs e vínculos que já estão no banco.
func (c *Collector) lerEstadoAnterior(ctx context.Context) (EstadoAnterior, []store.Vinculo, error) {
	antes := EstadoAnterior{Issues: map[string]store.Issue{}, PRs: map[int64]store.PR{}}

	issues, err := c.store.ListarIssues(ctx)
	if err != nil {
		return antes, nil, err
	}
	for _, i := range issues {
		antes.Issues[i.ID] = i
	}
	prs, err := c.store.ListarPRs(ctx)
	if err != nil {
		return antes, nil, err
	}
	for _, p := range prs {
		antes.PRs[p.ID] = p
	}
	vinculos, err := c.store.ListarVinculos(ctx)
	return antes, vinculos, err
}

// escoposDasIssues aplica a regra de escopo às issues que chegaram neste
// ciclo, usando os PRs e vínculos mais recentes (banco + ciclo atual).
func escoposDasIssues(issues []store.Issue, prsAntes map[int64]store.PR, prsNovos []store.PR, vinculos []store.Vinculo) map[string][]string {
	escopoDoPR := map[int64]string{}
	for id, p := range prsAntes {
		escopoDoPR[id] = p.Scope
	}
	for _, p := range prsNovos {
		escopoDoPR[p.ID] = p.Scope
	}
	escoposDosPRs := map[string][]string{}
	for _, v := range vinculos {
		if e, ok := escopoDoPR[v.PRID]; ok {
			escoposDosPRs[v.IssueID] = append(escoposDosPRs[v.IssueID], e)
		}
	}

	resultado := map[string][]string{}
	for _, i := range issues {
		resultado[i.ID] = escopo.DaIssue(escoposDosPRs[i.ID], i.ScopeLabel)
	}
	return resultado
}

// coletarIssues busca as issues de cada produto e as coloca em resultado.
// Devolve as issues como vieram do Linear (com anexos), para o vínculo.
func (c *Collector) coletarIssues(ctx context.Context, desde *time.Time, resultado *store.SyncResultado, anotarErro func(string, error)) []IssueRef {
	var refs []IssueRef
	for _, p := range c.produtos {
		issues, err := c.linear.ListIssues(ctx, p.LinearProjectID, desde)
		if err != nil {
			anotarErro(p.Slug, err)
			continue
		}
		for _, i := range issues {
			resultado.Issues = append(resultado.Issues, issueParaStore(p.Slug, i))
			refs = append(refs, IssueRef{ID: i.ID, Identifier: i.Identifier, Produto: p.Slug, AttachmentURLs: i.AttachmentURLs})
		}
	}
	return refs
}

// coletarPRs busca os PRs de cada repo configurado. Só para PRs novos ou
// alterados (updated_at diferente do gravado) busca os arquivos e calcula
// o risco: isso economiza chamadas à API, que tem limite por hora.
func (c *Collector) coletarPRs(ctx context.Context, desde *time.Time, gravados map[int64]store.PR, resultado *store.SyncResultado, anotarErro func(string, error)) []PRRef {
	var refs []PRRef
	for _, p := range c.produtos {
		for _, e := range p.Escopos {
			prs, err := c.buscarPRs(ctx, e.Repo, desde)
			if err != nil {
				anotarErro(e.Repo, err)
				continue
			}
			for _, pr := range prs {
				if antigo, ok := gravados[pr.ID]; ok && antigo.UpdatedAt.Equal(pr.UpdatedAt) {
					continue // nada mudou desde a última vez
				}
				arquivos, err := c.github.ListArquivos(ctx, e.Repo, pr.Number)
				if err != nil {
					anotarErro(fmt.Sprintf("%s#%d", e.Repo, pr.Number), err)
					continue
				}
				resultado.PRs = append(resultado.PRs, prParaStore(p.Slug, e.Nome, pr, arquivos, c.risco))
				refs = append(refs, PRRef{ID: pr.ID, Produto: p.Slug, URL: pr.URL, Branch: pr.Branch, Title: pr.Title, Body: pr.Body})
			}
		}
	}
	return refs
}

// buscarPRs traz os PRs alterados desde o último sync. No primeiro sync
// (desde == nil) traz os PRs abertos mais os atualizados dentro de
// JanelaPrimeiroSync, sem repetir os que aparecem nas duas listas.
func (c *Collector) buscarPRs(ctx context.Context, repo string, desde *time.Time) ([]github.PR, error) {
	if desde != nil {
		return c.github.ListPRs(ctx, repo, desde)
	}

	corte := c.agora().Add(-JanelaPrimeiroSync)
	recentes, err := c.github.ListPRs(ctx, repo, &corte)
	if err != nil {
		return nil, err
	}
	abertos, err := c.github.ListPRsAbertos(ctx, repo)
	if err != nil {
		return nil, err
	}

	vistos := map[int64]bool{}
	var todos []github.PR
	for _, pr := range append(recentes, abertos...) {
		if !vistos[pr.ID] {
			vistos[pr.ID] = true
			todos = append(todos, pr)
		}
	}
	return todos, nil
}

// calcularVinculos junta o que já está no banco com o que acabou de chegar e
// recalcula os vínculos. Os dados novos substituem os antigos porque trazem
// mais informação (anexos das issues e corpo dos PRs não ficam no banco).
// Vínculos só são adicionados, nunca removidos.
func calcularVinculos(antes EstadoAnterior, issuesNovas []IssueRef, prsNovos []PRRef) []store.Vinculo {
	issues := map[string]IssueRef{}
	for _, i := range antes.Issues {
		issues[i.ID] = IssueRef{ID: i.ID, Identifier: i.Identifier, Produto: i.ProductSlug}
	}
	for _, i := range issuesNovas {
		issues[i.ID] = i
	}
	prs := map[int64]PRRef{}
	for _, p := range antes.PRs {
		prs[p.ID] = PRRef{ID: p.ID, Produto: p.ProductSlug, URL: p.URL, Branch: p.Branch, Title: p.Title}
	}
	for _, p := range prsNovos {
		prs[p.ID] = p
	}

	var listaIssues []IssueRef
	for _, i := range issues {
		listaIssues = append(listaIssues, i)
	}
	var listaPRs []PRRef
	for _, p := range prs {
		listaPRs = append(listaPRs, p)
	}

	var vinculos []store.Vinculo
	for _, v := range Vincular(listaIssues, listaPRs) {
		vinculos = append(vinculos, store.Vinculo{IssueID: v.IssueID, PRID: v.PRID})
	}
	return vinculos
}

// issueParaStore converte a issue do Linear para a linha do banco.
func issueParaStore(produto string, i linear.Issue) store.Issue {
	return store.Issue{
		ID:          i.ID,
		Identifier:  i.Identifier,
		ProductSlug: produto,
		Title:       i.Title,
		StateName:   i.StateName,
		StateType:   i.StateType,
		ScopeLabel:  escopo.DaLabel(i.Labels),
		URL:         i.URL,
		Description: i.Description,
		UpdatedAt:   i.UpdatedAt,
	}
}

// prParaStore converte o PR do GitHub para a linha do banco, já com o risco.
func prParaStore(produto, nomeEscopo string, pr github.PR, arquivos []github.Arquivo, regras config.Risco) store.PR {
	var nomes []string
	additions, deletions := 0, 0
	for _, a := range arquivos {
		nomes = append(nomes, a.Nome)
		additions += a.Additions
		deletions += a.Deletions
	}
	r := risk.Avaliar(nomes, additions+deletions, regras)

	return store.PR{
		ID:          pr.ID,
		Repo:        pr.Repo,
		Number:      pr.Number,
		ProductSlug: produto,
		Scope:       nomeEscopo,
		Title:       pr.Title,
		Branch:      pr.Branch,
		State:       pr.State,
		Draft:       pr.Draft,
		Additions:   additions,
		Deletions:   deletions,
		RiskLevel:   r.Nivel,
		RiskReasons: r.Motivos,
		URL:         pr.URL,
		CreatedAt:   pr.CreatedAt,
		UpdatedAt:   pr.UpdatedAt,
	}
}
