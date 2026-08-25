// Package database implementa o acesso ao banco de dados (SQLite local ou Turso).
//
// Modos suportados:
//   - SQLite local: modernc.org/sqlite (100% Go, sem CGO) para arquivo local ou :memory:
//   - Turso (Embedded Replica): usa go-libsql para sincronizar réplica local, depois
//     abre com modernc.org/sqlite. Requer CGO_ENABLED=1.
//
// A abertura da conexão (Open) é separada da aplicação do schema (Migrate),
// e o DDL é idempotente (IF NOT EXISTS) para permitir execuções repetidas
// com segurança — é o mesmo caminho usado por testes com banco em memória.
//
// Este pacote fica restrito à conexão e ao schema: os tipos de dados vivem
// em internal/models e as consultas SQL em internal/repository — a única
// camada que fala SQL.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/tursodatabase/go-libsql"
)

// schema contém o DDL das tabelas e índices.
//
// Motivação de design: é propositalmente idempotente (IF NOT EXISTS), para
// que Migrate possa rodar a cada execução sem risco de erro — não existe
// versão ou histórico de migração, o schema é simples e estável.
const schema = `
CREATE TABLE IF NOT EXISTS coletanea (
	id     INTEGER PRIMARY KEY AUTOINCREMENT,
	codigo TEXT NOT NULL,
	titulo TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS hino (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	coletanea_id INTEGER NOT NULL REFERENCES coletanea(id),
	numeracao    INTEGER,
	titulo       TEXT NOT NULL,
	letra        TEXT,
	creditos     TEXT,
	revisado     INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_hino_coletanea_num ON hino (coletanea_id, numeracao);
`

// SyncTursoReplica sincroniza a réplica local embedded com o banco Turso remoto.
// Usa go-libsql EmbeddedReplicaConnector para fazer o sync inicial.
// Deve ser chamado ANTES de Open() quando usar Turso.
//
// replicaPath: caminho do arquivo SQLite local (ex.: data/hinos.db)
// primaryURL: URL do Turso (ex.: libsql://db-org.turso.io)
// authToken: token de autenticação do Turso
func SyncTursoReplica(replicaPath, primaryURL, authToken string) error {
	// Limpar arquivos locais existentes (evita erro "db file exists but metadata file does not")
	// O EmbeddedReplicaConnector cria seus próprios arquivos de metadados
	if err := cleanupLocalDBFiles(replicaPath); err != nil {
		return fmt.Errorf("limpar arquivos locais: %w", err)
	}

	connector, err := libsql.NewEmbeddedReplicaConnector(
		replicaPath,
		primaryURL,
		libsql.WithAuthToken(authToken),
	)
	if err != nil {
		return fmt.Errorf("criar connector embedded replica: %w", err)
	}
	defer connector.Close()

	// Sync inicial (bloqueia até completar)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db := sql.OpenDB(connector)
	defer db.Close()

	// Força sync fazendo uma query simples
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("sync inicial falhou: %w", err)
	}

	return nil
}

// cleanupLocalDBFiles remove arquivos SQLite locais que possam ter sido criados
// por outro driver (ex.: modernc.org/sqlite) para evitar conflito com o
// EmbeddedReplicaConnector do libSQL.
func cleanupLocalDBFiles(replicaPath string) error {
	files := []string{
		replicaPath,
		replicaPath + "-info",
		replicaPath + "-shm",
		replicaPath + "-wal",
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remover %s: %w", f, err)
		}
	}
	return nil
}

// indexOfQuery retorna o índice do primeiro '?' na string, ou -1 se não houver.
func indexOfQuery(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '?' {
			return i
		}
	}
	return -1
}

// Open abre uma conexão com o banco de dados SQLite e devolve um *sql.DB pronto para uso.
// dsn é a connection string já formatada pelo config.DatabaseDSN().
//
// Para testes, use dsn=":memory:" (banco em memória, sem arquivo).
//
// Motivações de design:
//   - foreign_keys(1) via pragma no DSN garante integridade referencial
//     (os "REFERENCES" do schema passam a ser aplicados de fato);
//   - SetMaxOpenConns(1) limita a uma conexão: SQLite serializa escritas.
func Open(dsn string) (*sql.DB, error) {
	// Para arquivo, garantir que o diretório exista
	if dsn != ":memory:" && len(dsn) > 5 && dsn[:5] == "file:" {
		// Extrair path do dsn "file:path?params"
		path := dsn[5:]
		if idx := indexOfQuery(path); idx >= 0 {
			path = path[:idx]
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("criar diretório do banco: %w", err)
		}
	}

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir banco: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	conn.SetMaxOpenConns(1)
	return conn, nil
}

// Migrate aplica o schema (DDL idempotente) na conexão informada. Pode ser
// chamado em qualquer ordem e quantas vezes for necessário.
func Migrate(conn *sql.DB) error {
	if _, err := conn.Exec(schema); err != nil {
		return fmt.Errorf("aplicar schema: %w", err)
	}
	return nil
}
