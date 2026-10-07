package store

import (
	"io"
	"io/fs"
	"log/slog"
	"path/filepath"
	"testing"
)

var loggerSilencioso = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestOpen_CriaTabelas(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "teste.db"), loggerSilencioso)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	for _, tabela := range []string{"issues", "pull_requests", "issue_prs", "events", "meta", "schema_migrations"} {
		var nome string
		err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, tabela).Scan(&nome)
		if err != nil {
			t.Errorf("tabela %s não foi criada: %v", tabela, err)
		}
	}
}

// Abrir o mesmo banco duas vezes não pode reaplicar migrações
// (o CREATE TABLE falharia porque a tabela já existe).
func TestOpen_MigracoesSaoAplicadasUmaVez(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teste.db")

	s, err := Open(path, loggerSilencioso)
	if err != nil {
		t.Fatalf("primeiro Open: %v", err)
	}
	s.Close()

	s, err = Open(path, loggerSilencioso)
	if err != nil {
		t.Fatalf("segundo Open: %v", err)
	}
	defer s.Close()

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	arquivos, _ := fs.Glob(migrationsFS, "migrations/*.sql")
	if total != len(arquivos) {
		t.Errorf("schema_migrations tem %d linhas, esperava %d (uma por arquivo)", total, len(arquivos))
	}
}
