package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MetaLastVisit guarda até quando o dono já viu as mudanças.
const MetaLastVisit = "last_visit"

// Tipos de evento (coluna kind), conforme a seção 9 do CONTEXT.md.
const (
	EventoIssueCriada      = "issue_created"
	EventoIssueMudouEstado = "issue_state_changed"
	EventoPRAberto         = "pr_opened"
	EventoPRMergeado       = "pr_merged"
	EventoPRFechado        = "pr_closed"
	EventoPRAtualizado     = "pr_updated"
)

// Evento é uma linha da tabela events: uma mudança detectada pelo coletor.
type Evento struct {
	ID          int64
	OccurredAt  time.Time
	ProductSlug string
	Scope       string // vazio = "a classificar"
	Kind        string
	Ref         string // ABC-123 ou owner/repo#42
	Summary     string
	URL         string
}

func inserirEvento(ctx context.Context, tx *sql.Tx, e Evento) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO events (occurred_at, product_slug, scope, kind, ref, summary, url)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		formatar(e.OccurredAt), e.ProductSlug, nuloSeVazio(e.Scope), e.Kind, e.Ref, e.Summary, e.URL)
	if err != nil {
		return fmt.Errorf("gravando evento %s de %s: %w", e.Kind, e.Ref, err)
	}
	return nil
}

// EventosDesde devolve os eventos que aconteceram depois de desde (zero =
// todos), do mais antigo para o mais novo.
//
// Comparar datas como texto funciona porque todas estão em ISO 8601 UTC
// com o mesmo formato: a ordem alfabética é a ordem cronológica.
func (s *Store) EventosDesde(ctx context.Context, desde time.Time) ([]Evento, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, occurred_at, product_slug, COALESCE(scope, ''), kind, ref, summary, url
		FROM events
		WHERE occurred_at > ?
		ORDER BY occurred_at, id`, formatar(desde))
	if err != nil {
		return nil, fmt.Errorf("listando eventos: %w", err)
	}
	defer rows.Close()

	var eventos []Evento
	for rows.Next() {
		var e Evento
		var quando string
		if err := rows.Scan(&e.ID, &quando, &e.ProductSlug, &e.Scope, &e.Kind, &e.Ref, &e.Summary, &e.URL); err != nil {
			return nil, fmt.Errorf("lendo evento: %w", err)
		}
		e.OccurredAt = parsear(quando)
		eventos = append(eventos, e)
	}
	return eventos, rows.Err()
}

// UltimaVisita devolve até quando as mudanças já foram vistas (zero = nunca).
func (s *Store) UltimaVisita(ctx context.Context) (time.Time, error) {
	v, err := s.getMeta(ctx, MetaLastVisit)
	if err != nil {
		return time.Time{}, err
	}
	return parsear(v), nil
}

// MarcarVistoAte grava last_visit. Só o botão "marcar como visto" chama
// isto: abrir a página de mudanças NÃO marca nada como visto.
func (s *Store) MarcarVistoAte(ctx context.Context, quando time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("iniciando transação: %w", err)
	}
	defer tx.Rollback()
	if err := setMeta(ctx, tx, MetaLastVisit, formatar(quando)); err != nil {
		return err
	}
	return tx.Commit()
}
