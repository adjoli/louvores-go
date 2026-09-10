// Package database implementa o acesso ao banco de dados.
//
// Há dois modos de conexão:
//   - SQLite local, via modernc.org/sqlite (100% Go, sem CGO), aberto por
//     Open a partir de um caminho/arquivo;
//   - Turso na nuvem, via libsql-client-go (puro Go, protocolo libSQL sobre
//     HTTP/WebSocket), aberto por OpenRemote a partir da URL e do token.
//
// A abertura da conexão (Open/OpenRemote) é separada da aplicação do schema
// (Migrate), e o DDL é idempotente (IF NOT EXISTS) para permitir execuções
// repetidas com segurança — é o mesmo caminho usado por testes com banco em
// memória.
//
// Este pacote fica restrito à conexão e ao schema: os tipos de dados vivem
// em internal/models e as consultas SQL em internal/repository — a única
// camada que fala SQL.
package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tursodatabase/libsql-client-go/libsql"
	_ "modernc.org/sqlite"
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

// Open abre uma conexão SQLite e devolve um *sql.DB pronto para uso.
// Para testes, use o caminho especial ":memory:" (banco em memória, sem
// arquivo).
//
// Motivações de design:
//   - foreign_keys(1) via pragma no DSN garante integridade referencial
//     (os "REFERENCES" do schema passam a ser aplicados de fato);
//   - SetMaxOpenConns(1) limita a uma conexão: o SQLite serializa escritas,
//     e manter uma única conexão evita erros de "database is locked".
func Open(path string) (*sql.DB, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("criar diretório do banco: %w", err)
		}
	}

	dsn := path
	if path != ":memory:" {
		dsn = fmt.Sprintf("file:%s?_pragma=foreign_keys(1)", path)
	}

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir banco %q: %w", path, err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	conn.SetMaxOpenConns(1)
	return conn, nil
}

// OpenRemote abre uma conexão com um banco Turso na nuvem e devolve um
// *sql.DB pronto para uso.
//
// A URL deve usar o esquema libsql:// (ex.: libsql://meu-banco.turso.io) e o
// token é passado via libsql.WithAuthToken — a versão atual do driver proíbe
// o parâmetro ?authToken= na URL. O pool de conexões é deixado no padrão do
// database/sql: a limitação de uma única conexão de Open existe apenas por
// causa do lock de arquivo do SQLite local e não se aplica ao Turso.
func OpenRemote(url, authToken string) (*sql.DB, error) {
	connector, err := libsql.NewConnector(url, libsql.WithAuthToken(authToken))
	if err != nil {
		return nil, fmt.Errorf("criar conector Turso %q: %w", url, err)
	}

	conn := sql.OpenDB(connector)

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ping Turso %q: %w", url, err)
	}

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
