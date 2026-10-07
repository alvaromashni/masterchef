// Package store é a camada fina sobre o SQLite. Toda SQL do painel fica aqui.
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	// Driver SQLite escrito em Go puro: não precisa de CGO nem de gcc para compilar.
	// O "_" importa o pacote só pelo efeito de registrar o driver "sqlite".
	_ "modernc.org/sqlite"
)

// migrationsFS embute os arquivos .sql dentro do binário, assim o painel
// continua sendo um único executável, sem pasta de migrações para distribuir.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store guarda a conexão com o banco.
type Store struct {
	db *sql.DB
}

// Open abre (ou cria) o arquivo SQLite em path e aplica as migrações pendentes.
func Open(path string, logger *slog.Logger) (*Store, error) {
	// busy_timeout: se o banco estiver ocupado, espera até 5s em vez de falhar na hora.
	// WAL: permite ler enquanto o coletor escreve, então a tela não trava durante um sync.
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrindo banco %s: %w", path, err)
	}
	// O SQLite aceita um escritor por vez. Uma única conexão evita erros de
	// "database is locked" e é mais que suficiente para um painel local.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("conectando ao banco %s: %w", path, err)
	}

	s := &Store{db: db}
	if err := s.migrate(logger); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close fecha a conexão com o banco.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate aplica, em ordem, cada arquivo de migrations/ que ainda não está
// registrado na tabela schema_migrations. Cada migração roda na sua própria
// transação: ou entra inteira, ou não entra.
func (s *Store) migrate(logger *slog.Logger) error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("criando schema_migrations: %w", err)
	}

	aplicadas, err := s.migracoesAplicadas()
	if err != nil {
		return err
	}

	arquivos, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("listando migrações: %w", err)
	}
	// Os arquivos começam com número (0001_, 0002_...), então a ordem
	// alfabética é a ordem de aplicação.
	sort.Strings(arquivos)

	for _, arquivo := range arquivos {
		versao := strings.TrimSuffix(strings.TrimPrefix(arquivo, "migrations/"), ".sql")
		if aplicadas[versao] {
			continue
		}
		if err := s.aplicarMigracao(arquivo, versao); err != nil {
			return err
		}
		logger.Info("migração aplicada", "versao", versao)
	}
	return nil
}

func (s *Store) migracoesAplicadas() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("lendo schema_migrations: %w", err)
	}
	defer rows.Close()

	aplicadas := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("lendo schema_migrations: %w", err)
		}
		aplicadas[v] = true
	}
	return aplicadas, rows.Err()
}

func (s *Store) aplicarMigracao(arquivo, versao string) error {
	conteudo, err := migrationsFS.ReadFile(arquivo)
	if err != nil {
		return fmt.Errorf("lendo migração %s: %w", arquivo, err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("iniciando transação da migração %s: %w", versao, err)
	}
	// Se algo der errado antes do Commit, o Rollback desfaz tudo.
	// Depois de um Commit bem-sucedido, o Rollback não faz nada.
	defer tx.Rollback()

	if _, err := tx.Exec(string(conteudo)); err != nil {
		return fmt.Errorf("aplicando migração %s: %w", versao, err)
	}
	_, err = tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		versao, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("registrando migração %s: %w", versao, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("confirmando migração %s: %w", versao, err)
	}
	return nil
}
