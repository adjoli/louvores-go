// Package app é o composition root da aplicação.
//
// Ele inicializa e conecta todas as camadas (config, logging, banco de
// dados, repository e services), expondo a estrutura App como container de
// dependências e ponto de lifecycle (Close).
//
// Motivação de design: concentrar a montagem em um único lugar evita que o
// main.go e os handlers HTTP saibam como construir cada dependência. O
// main.go apenas chama app.New e libera com defer app.Close.
package app

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/adjoli/louvores-go/internal/config"
	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/logging"
	"github.com/adjoli/louvores-go/internal/repository"
	"github.com/adjoli/louvores-go/internal/services"
)

// App é o container de dependências e lifecycle da aplicação.
//
// Ele mantém as referências para a configuração, a conexão com o banco, o
// logger e os serviços. A instância deve ser criada com New e liberada com
// Close quando não for mais necessária.
type App struct {
	cfg      *config.Config
	db       *sql.DB
	logger   *slog.Logger
	hinoSvc  *services.HinoService
	statsSvc *services.StatsService
}

// Config retorna a configuração carregada da aplicação.
func (a *App) Config() *config.Config {
	return a.cfg
}

// DB retorna a conexão com o banco de dados.
func (a *App) DB() *sql.DB {
	return a.db
}

// Logger retorna o logger estruturado da aplicação.
func (a *App) Logger() *slog.Logger {
	return a.logger
}

// HinoService retorna o serviço de leitura de coletâneas e hinos.
func (a *App) HinoService() *services.HinoService {
	return a.hinoSvc
}

// StatsService retorna o serviço de estatísticas.
func (a *App) StatsService() *services.StatsService {
	return a.statsSvc
}

// Close fecha a conexão com o banco de dados.
// É nil-safe: retorna nil imediatamente se o banco não foi inicializado.
func (a *App) Close() error {
	if a.db == nil {
		return nil
	}
	return a.db.Close()
}

// New inicializa a aplicação criando e conectando todas as camadas na
// ordem: configuração, logging, banco de dados (com schema aplicado),
// repository e services. A função também cria o schema do banco de dados
// automaticamente.
//
// O caller deve chamar Close quando a instância não for mais necessária.
//
// Annotation de erros: todos os erros são empacotados com %w preservando a
// origem, e uma conexão aberta é fechada caso um passo posterior falhe
// (evitar vazamento).
func New() (*App, error) {
	cfg, err := config.New()
	if err != nil {
		return nil, fmt.Errorf("inicializar config: %w", err)
	}

	logger, err := logging.Setup(cfg.LogPath)
	if err != nil {
		return nil, fmt.Errorf("inicializar logging: %w", err)
	}

	// Se usando Turso (Embedded Replica), sincroniza a réplica local primeiro
	if cfg.UseTurso() {
		replicaPath := cfg.TursoReplicaPath()
		logger.Info("sincronizando réplica Turso", "replica", replicaPath, "primary", cfg.TursoDatabaseURL)
		if err := database.SyncTursoReplica(replicaPath, cfg.TursoDatabaseURL, cfg.TursoAuthToken); err != nil {
			return nil, fmt.Errorf("sync réplica Turso: %w", err)
		}
		logger.Info("réplica Turso sincronizada com sucesso")
	}

	db, err := database.Open(cfg.DatabaseDSN())
	if err != nil {
		return nil, fmt.Errorf("inicializar banco: %w", err)
	}

	if err := database.Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("aplicar schema: %w", err)
	}

	hinoRepo := repository.NewSQLiteHinoRepository(db)
	coletaneaRepo := repository.NewSQLiteColetaneaRepository(db)

	return &App{
		cfg:      cfg,
		db:       db,
		logger:   logger,
		hinoSvc:  services.NewHinoService(hinoRepo, coletaneaRepo),
		statsSvc: services.NewStatsService(hinoRepo),
	}, nil
}
