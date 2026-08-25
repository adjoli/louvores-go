// Package config carrega e valida as configurações da aplicação.
// As configurações são definidas por variáveis de ambiente, com valores
// padrão sensatos quando a variável não está presente.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

const (
	// EnvDatabasePath é o nome da variável de ambiente que define o caminho
	// do banco SQLite (modo local).
	EnvDatabasePath = "DB_PATH"

	// EnvTemplatePath é o nome da variável de ambiente que define o caminho
	// do template PPTX usado na geração.
	EnvTemplatePath = "TEMPLATE_PATH"

	// EnvLogPath é o nome da variável de ambiente que define o arquivo de log.
	EnvLogPath = "LOG_PATH"

	// EnvHost é o nome da variável de ambiente que define o endereço de
	// escuta do servidor HTTP (vazio = todas as interfaces).
	EnvHost = "HOST"

	// EnvPort é o nome da variável de ambiente que define a porta do
	// servidor HTTP.
	EnvPort = "PORT"

	// EnvTursoDatabaseURL é o nome da variável de ambiente que define a URL
	// do banco Turso (ex.: libsql://seu-db-sua-conta.turso.io).
	// Se definida, usa Embedded Replica (réplica local sincronizada com Turso).
	EnvTursoDatabaseURL = "TURSO_DATABASE_URL"

	// EnvTursoAuthToken é o nome da variável de ambiente que define o token
	// de autenticação do Turso.
	EnvTursoAuthToken = "TURSO_AUTH_TOKEN"

	// DefaultDBPath é o banco padrão quando DB_PATH não está definida.
	DefaultDBPath = "data/hinos.db"

	// DefaultTemplatePath é o template padrão quando TEMPLATE_PATH não está
	// definida.
	DefaultTemplatePath = "data/templates/default.pptx"

	// DefaultLogPath é o arquivo de log padrão quando LOG_PATH não está
	// definida.
	DefaultLogPath = "logs/app.log"

	// DefaultPort é a porta padrão quando PORT não está definida.
	DefaultPort = "8080"
)

// Config armazena as configurações carregadas da aplicação.
// Todos os campos são sempre preenchidos em New (default ou variável).
type Config struct {
	DBPath           string
	TemplatePath     string
	LogPath          string
	Host             string
	Port             string
	TursoDatabaseURL string
	TursoAuthToken   string
}

// TursoReplicaPath retorna o caminho do arquivo de réplica local para Turso.
// Se DB_PATH já termina com .db, usa ele; senão, adiciona _turso_replica.db
func (c *Config) TursoReplicaPath() string {
	if strings.HasSuffix(c.DBPath, ".db") {
		return c.DBPath
	}
	return strings.TrimSuffix(c.DBPath, ".db") + "_turso_replica.db"
}

// Addr retorna o endereço de escuta do servidor HTTP no formato
// host:port. Com Host vazio, escuta em todas as interfaces (":8080").
func (c *Config) Addr() string {
	return c.Host + ":" + c.Port
}

// UseTurso retorna true se a configuração deve usar Turso (libsql)
// em vez do SQLite local.
func (c *Config) UseTurso() bool {
	return c.TursoDatabaseURL != ""
}

// DatabaseDSN retorna o DSN (Data Source Name) a ser usado para abrir
// a conexão com o banco SQLite.
// Para SQLite local, inclui pragmas de foreign_keys e busy_timeout.
// Para Turso (Embedded Replica), retorna o DSN do arquivo de réplica local.
func (c *Config) DatabaseDSN() string {
	if c.UseTurso() {
		// Embedded Replica: usa driver sqlite moderno no arquivo de réplica local
		replicaPath := c.TursoReplicaPath()
		return fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", replicaPath)
	}

	// SQLite local (arquivo ou :memory:)
	if c.DBPath == ":memory:" {
		return ":memory:"
	}
	return fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", c.DBPath)
}

// containsQuery verifica se a string já contém query parameters.
func containsQuery(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '?' {
			return true
		}
	}
	return false
}

// New cria uma nova Config aplicando os valores padrão, carregando
// sobrescritas de variáveis de ambiente e preparando o diretório do banco
// de dados.
//
// Se um arquivo .env existir no diretório de trabalho, as variáveis
// definidas nele são carregadas antes da leitura do ambiente. Variáveis de
// ambiente reais SEMPRE SOBREPÕEM valores do arquivo .env (o godotenv não
// sobrescreve variáveis já definidas no processo).
func New() (*Config, error) {
	cfg := defaultConfig()

	_ = godotenv.Load()

	if err := loadEnvironment(cfg); err != nil {
		return nil, err
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	if err := prepareDatabasePath(cfg); err != nil {
		return nil, fmt.Errorf("prepare database path: %w", err)
	}

	return cfg, nil
}

// defaultConfig retorna uma Config com os valores padrão.
func defaultConfig() *Config {
	return &Config{
		DBPath:       DefaultDBPath,
		TemplatePath: DefaultTemplatePath,
		LogPath:      DefaultLogPath,
		Port:         DefaultPort,
	}
}

// loadEnvironment sobrescreve os defaults com variáveis de ambiente.
// Variáveis definidas mas vazias são tratadas como "não configuradas",
// mantendo o default — comportamento de fallback seguro.
func loadEnvironment(cfg *Config) error {
	if v, ok := os.LookupEnv(EnvDatabasePath); ok && v != "" {
		cfg.DBPath = v
	}
	if v, ok := os.LookupEnv(EnvTemplatePath); ok && v != "" {
		cfg.TemplatePath = v
	}
	if v, ok := os.LookupEnv(EnvLogPath); ok && v != "" {
		cfg.LogPath = v
	}
	if v, ok := os.LookupEnv(EnvHost); ok && v != "" {
		cfg.Host = v
	}
	if v, ok := os.LookupEnv(EnvPort); ok && v != "" {
		cfg.Port = v
	}
	if v, ok := os.LookupEnv(EnvTursoDatabaseURL); ok && v != "" {
		cfg.TursoDatabaseURL = v
	}
	if v, ok := os.LookupEnv(EnvTursoAuthToken); ok && v != "" {
		cfg.TursoAuthToken = v
	}
	return nil
}

// validate verifica a consistência da configuração carregada.
// Caminhos vazios indicam configuração inválida, já que os defaults
// sempre preenchem os campos.
func validate(cfg *Config) error {
	if cfg.DBPath == "" {
		return fmt.Errorf("DB_PATH não pode ser vazio")
	}
	if cfg.TemplatePath == "" {
		return fmt.Errorf("TEMPLATE_PATH não pode ser vazio")
	}
	if cfg.LogPath == "" {
		return fmt.Errorf("LOG_PATH não pode ser vazio")
	}
	if cfg.Port == "" {
		return fmt.Errorf("PORT não pode ser vazio")
	}
	return nil
}

// prepareDatabasePath garante que o diretório do banco exista antes do
// primeiro uso. Lida com o caminho especial ":memory:" (testes).
func prepareDatabasePath(cfg *Config) error {
	if cfg.DBPath == ":memory:" {
		return nil
	}
	return os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755)
}
