package services

import (
	"context"

	"github.com/adjoli/louvores-go/internal/models"
	"github.com/adjoli/louvores-go/internal/repository"
)

// HinoRepository define as operações de persistência para hinos.
// Implementada por repository.HinoRepository (SQLite) e futuramente por postgres.HinoRepository.
type HinoRepository interface {
	Create(ctx context.Context, hino *models.Hino) error
	FindByID(ctx context.Context, id int64) (*models.Hino, error)
	List(ctx context.Context) ([]models.Hino, error)
	Update(ctx context.Context, hino *models.Hino) error
	Delete(ctx context.Context, id int64) error
	ListByColetanea(ctx context.Context, coletaneaID int64) ([]models.Hino, error)
	FindByNumero(ctx context.Context, coletaneaID int64, numero int) (*models.Hino, error)
	StatsPorColetanea(ctx context.Context) ([]repository.StatsRow, error)
}

// ColetaneaRepository define as operações de persistência para coletâneas.
// Implementada por repository.ColetaneaRepository (SQLite) e futuramente por postgres.ColetaneaRepository.
type ColetaneaRepository interface {
	Create(ctx context.Context, c *models.Coletanea) error
	FindByID(ctx context.Context, id int64) (*models.Coletanea, error)
	List(ctx context.Context) ([]models.Coletanea, error)
	FindByCodigo(ctx context.Context, codigo string) (*models.Coletanea, error)
	Update(ctx context.Context, c *models.Coletanea) error
	Delete(ctx context.Context, id int64) error
}