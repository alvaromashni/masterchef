package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// PR é uma linha da tabela pull_requests.
type PR struct {
	ID          int64
	Repo        string
	Number      int
	ProductSlug string
	Scope       string
	Title       string
	Branch      string
	State       string // open, closed, merged
	Draft       bool
	Additions   int
	Deletions   int
	RiskLevel   string
	RiskReasons []string
	URL         string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Vinculo liga uma issue a um PR (tabela issue_prs).
type Vinculo struct {
	IssueID string
	PRID    int64
}

func upsertPR(ctx context.Context, tx *sql.Tx, p PR) error {
	motivos, err := json.Marshal(p.RiskReasons)
	if err != nil {
		return fmt.Errorf("codificando motivos do PR %s#%d: %w", p.Repo, p.Number, err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO pull_requests (id, repo, number, product_slug, scope, title, branch, state, draft,
			additions, deletions, risk_level, risk_reasons, url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			repo = excluded.repo, number = excluded.number,
			product_slug = excluded.product_slug, scope = excluded.scope,
			title = excluded.title, branch = excluded.branch,
			state = excluded.state, draft = excluded.draft,
			additions = excluded.additions, deletions = excluded.deletions,
			risk_level = excluded.risk_level, risk_reasons = excluded.risk_reasons,
			url = excluded.url, created_at = excluded.created_at, updated_at = excluded.updated_at`,
		p.ID, p.Repo, p.Number, p.ProductSlug, p.Scope, p.Title, p.Branch, p.State, p.Draft,
		p.Additions, p.Deletions, p.RiskLevel, string(motivos), p.URL,
		formatar(p.CreatedAt), formatar(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("gravando PR %s#%d: %w", p.Repo, p.Number, err)
	}
	return nil
}

func inserirVinculo(ctx context.Context, tx *sql.Tx, v Vinculo) error {
	// OR IGNORE: se o vínculo já existe, não faz nada (a chave primária é o par).
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO issue_prs (issue_id, pr_id) VALUES (?, ?)`, v.IssueID, v.PRID)
	if err != nil {
		return fmt.Errorf("gravando vínculo %s ↔ %d: %w", v.IssueID, v.PRID, err)
	}
	return nil
}

// ListarPRs devolve todos os PRs gravados.
func (s *Store) ListarPRs(ctx context.Context) ([]PR, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, repo, number, product_slug, scope, title, branch, state, draft,
			additions, deletions, risk_level, risk_reasons, url, created_at, updated_at
		FROM pull_requests`)
	if err != nil {
		return nil, fmt.Errorf("listando PRs: %w", err)
	}
	defer rows.Close()

	var prs []PR
	for rows.Next() {
		var p PR
		var motivos, criado, atualizado string
		err := rows.Scan(&p.ID, &p.Repo, &p.Number, &p.ProductSlug, &p.Scope, &p.Title, &p.Branch,
			&p.State, &p.Draft, &p.Additions, &p.Deletions, &p.RiskLevel, &motivos, &p.URL, &criado, &atualizado)
		if err != nil {
			return nil, fmt.Errorf("lendo PR: %w", err)
		}
		if err := json.Unmarshal([]byte(motivos), &p.RiskReasons); err != nil {
			return nil, fmt.Errorf("lendo motivos do PR %s#%d: %w", p.Repo, p.Number, err)
		}
		p.CreatedAt, p.UpdatedAt = parsear(criado), parsear(atualizado)
		prs = append(prs, p)
	}
	return prs, rows.Err()
}

// ListarVinculos devolve todos os pares issue ↔ PR.
func (s *Store) ListarVinculos(ctx context.Context) ([]Vinculo, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT issue_id, pr_id FROM issue_prs`)
	if err != nil {
		return nil, fmt.Errorf("listando vínculos: %w", err)
	}
	defer rows.Close()

	var vs []Vinculo
	for rows.Next() {
		var v Vinculo
		if err := rows.Scan(&v.IssueID, &v.PRID); err != nil {
			return nil, fmt.Errorf("lendo vínculo: %w", err)
		}
		vs = append(vs, v)
	}
	return vs, rows.Err()
}

// formatar e parsear convertem entre time.Time e o TEXT ISO 8601 do banco.
func formatar(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// parsear devolve a data zero para texto vazio ou inválido; o painel mostra
// "nunca" nesses casos em vez de quebrar a página.
func parsear(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
