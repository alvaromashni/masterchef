// Package collector busca dados nas APIs de tempos em tempos e grava no store.
package collector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/linear"
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

// Gravador é o que o coletor precisa do store.
type Gravador interface {
	UltimoSyncOK(ctx context.Context) (*time.Time, error)
	SalvarSync(ctx context.Context, r store.SyncResultado) error
}

// Collector roda os ciclos de coleta.
type Collector struct {
	produtos  []config.Produto
	intervalo time.Duration
	linear    FonteIssues
	store     Gravador
	logger    *slog.Logger
	// agora existe para os testes controlarem o relógio.
	agora func() time.Time
}

// New monta um coletor a partir da config.
func New(cfg *config.Config, fonte FonteIssues, st Gravador, logger *slog.Logger) *Collector {
	return &Collector{
		produtos:  cfg.Produtos,
		intervalo: cfg.PollInterval,
		linear:    fonte,
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

// Ciclo faz uma coleta completa (seção 10 do CONTEXT.md).
//
// Um produto com erro não derruba os outros: o erro é anotado, os demais
// seguem e, no fim, tudo que deu certo é gravado. Mas last_sync_ok só avança
// se TODOS deram certo; senão o próximo ciclo pediria "desde agora" e
// perderia as mudanças do produto que falhou.
func (c *Collector) Ciclo(ctx context.Context) error {
	// Guardamos o INÍCIO do ciclo como marca do sync. Se usássemos o fim,
	// uma issue alterada durante a coleta poderia ficar de fora para sempre.
	inicio := c.agora()

	desde, err := c.store.UltimoSyncOK(ctx)
	if err != nil {
		return err
	}

	resultado := store.SyncResultado{Iniciado: inicio}
	for _, p := range c.produtos {
		issues, err := c.linear.ListIssues(ctx, p.LinearProjectID, desde)
		if err != nil {
			c.logger.Error("falha ao coletar produto", "produto", p.Slug, "erro", err)
			resultado.Erros = append(resultado.Erros, fmt.Sprintf("%s: %v", p.Slug, err))
			continue
		}
		for _, i := range issues {
			resultado.Issues = append(resultado.Issues, paraStore(p.Slug, i))
		}
	}

	return c.store.SalvarSync(ctx, resultado)
}

// paraStore converte a issue do Linear para a linha do banco,
// aplicando a regra de escopo.
func paraStore(produto string, i linear.Issue) store.Issue {
	return store.Issue{
		ID:          i.ID,
		Identifier:  i.Identifier,
		ProductSlug: produto,
		Title:       i.Title,
		StateName:   i.StateName,
		StateType:   i.StateType,
		ScopeLabel:  EscopoDaLabel(i.Labels),
		URL:         i.URL,
		Description: i.Description,
		UpdatedAt:   i.UpdatedAt,
	}
}
