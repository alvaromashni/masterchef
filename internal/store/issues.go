package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Chaves da tabela meta.
const (
	MetaLastSyncOK    = "last_sync_ok"
	MetaLastSyncError = "last_sync_error"
)

// Issue é uma linha da tabela issues.
type Issue struct {
	ID          string
	Identifier  string
	ProductSlug string
	Title       string
	StateName   string
	StateType   string
	ScopeLabel  string // vazio = sem label scope:<nome>
	URL         string
	Description string
	UpdatedAt   time.Time
}

// SyncResultado é tudo que um ciclo de coleta quer gravar.
type SyncResultado struct {
	Issues   []Issue
	PRs      []PR
	Vinculos []Vinculo
	Eventos  []Evento
	// Iniciado é quando o ciclo começou. Vira last_sync_ok se não houve erro.
	Iniciado time.Time
	// Erros de produtos que falharam. Se houver algum, last_sync_ok NÃO avança.
	Erros []string
}

// SalvarSync grava o resultado de um ciclo numa única transação: ou entra
// tudo (issues + meta), ou nada. Assim a tela nunca vê um sync pela metade.
func (s *Store) SalvarSync(ctx context.Context, r SyncResultado) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("iniciando transação do sync: %w", err)
	}
	defer tx.Rollback()

	for _, i := range r.Issues {
		if err := upsertIssue(ctx, tx, i); err != nil {
			return err
		}
	}
	for _, p := range r.PRs {
		if err := upsertPR(ctx, tx, p); err != nil {
			return err
		}
	}
	for _, v := range r.Vinculos {
		if err := inserirVinculo(ctx, tx, v); err != nil {
			return err
		}
	}
	for _, e := range r.Eventos {
		if err := inserirEvento(ctx, tx, e); err != nil {
			return err
		}
	}

	if len(r.Erros) > 0 {
		// Um erro por linha: a tela mostra cada um separado.
		msg := "Ciclo de " + r.Iniciado.UTC().Format(time.RFC3339) + ":\n" + strings.Join(r.Erros, "\n")
		if err := setMeta(ctx, tx, MetaLastSyncError, msg); err != nil {
			return err
		}
	} else {
		if err := setMeta(ctx, tx, MetaLastSyncOK, r.Iniciado.UTC().Format(time.RFC3339)); err != nil {
			return err
		}
		// Sync bem-sucedido apaga o aviso de erro antigo.
		if _, err := tx.ExecContext(ctx, `DELETE FROM meta WHERE key = ?`, MetaLastSyncError); err != nil {
			return fmt.Errorf("limpando %s: %w", MetaLastSyncError, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("confirmando sync: %w", err)
	}
	return nil
}

// upsertIssue insere a issue ou, se o id já existe, atualiza os campos.
// "ON CONFLICT ... DO UPDATE" é o "upsert" do SQLite (como um MERGE).
func upsertIssue(ctx context.Context, tx *sql.Tx, i Issue) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO issues (id, identifier, product_slug, title, state_name, state_type, scope_label, url, description, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			identifier   = excluded.identifier,
			product_slug = excluded.product_slug,
			title        = excluded.title,
			state_name   = excluded.state_name,
			state_type   = excluded.state_type,
			scope_label  = excluded.scope_label,
			url          = excluded.url,
			description  = excluded.description,
			updated_at   = excluded.updated_at`,
		i.ID, i.Identifier, i.ProductSlug, i.Title, i.StateName, i.StateType,
		nuloSeVazio(i.ScopeLabel), i.URL, i.Description, formatar(i.UpdatedAt))
	if err != nil {
		return fmt.Errorf("gravando issue %s: %w", i.Identifier, err)
	}
	return nil
}

// ListarIssues devolve todas as issues gravadas.
func (s *Store) ListarIssues(ctx context.Context) ([]Issue, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, identifier, product_slug, title, state_name, state_type,
			COALESCE(scope_label, ''), url, COALESCE(description, ''), updated_at
		FROM issues`)
	if err != nil {
		return nil, fmt.Errorf("listando issues: %w", err)
	}
	defer rows.Close()

	var issues []Issue
	for rows.Next() {
		var i Issue
		var atualizado string
		err := rows.Scan(&i.ID, &i.Identifier, &i.ProductSlug, &i.Title, &i.StateName, &i.StateType,
			&i.ScopeLabel, &i.URL, &i.Description, &atualizado)
		if err != nil {
			return nil, fmt.Errorf("lendo issue: %w", err)
		}
		i.UpdatedAt = parsear(atualizado)
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

// EstadoSync é o que o topo de toda página mostra.
type EstadoSync struct {
	UltimoOK   time.Time // zero = nunca sincronizou com sucesso
	UltimoErro string    // vazio = último sync deu certo
}

// LerEstadoSync lê last_sync_ok e last_sync_error da tabela meta.
func (s *Store) LerEstadoSync(ctx context.Context) (EstadoSync, error) {
	var e EstadoSync

	ok, err := s.getMeta(ctx, MetaLastSyncOK)
	if err != nil {
		return e, err
	}
	if ok != "" {
		if e.UltimoOK, err = time.Parse(time.RFC3339, ok); err != nil {
			return e, fmt.Errorf("lendo %s: %w", MetaLastSyncOK, err)
		}
	}

	e.UltimoErro, err = s.getMeta(ctx, MetaLastSyncError)
	return e, err
}

// UltimoSyncOK devolve quando foi o último sync bem-sucedido, ou nil se nunca houve.
// O coletor usa isso para pedir ao Linear só o que mudou desde então.
func (s *Store) UltimoSyncOK(ctx context.Context) (*time.Time, error) {
	e, err := s.LerEstadoSync(ctx)
	if err != nil || e.UltimoOK.IsZero() {
		return nil, err
	}
	return &e.UltimoOK, nil
}

// getMeta devolve "" quando a chave não existe.
func (s *Store) getMeta(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lendo meta %s: %w", key, err)
	}
	return v, nil
}

func setMeta(ctx context.Context, tx *sql.Tx, key, value string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("gravando meta %s: %w", key, err)
	}
	return nil
}

// nuloSeVazio grava NULL no banco em vez de "" (scope_label nulo = sem label).
func nuloSeVazio(s string) any {
	if s == "" {
		return nil
	}
	return s
}
