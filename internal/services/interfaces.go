package services

import (
	"context"

	"github.com/adjoli/louvores-go/internal/models"
	"github.com/adjoli/louvores-go/internal/repository"
)

// HinoRepository define as operações de persistência de hinos usadas em
// produção. Implementada por repository.SQLiteHinoRepository.
// A interface expõe leitura, Update (edição via interface web) e Create
// (inclusão de hinos em coletâneas que permitem, ex.: Corinhos). Delete
// continua restrito ao repositório concreto.
type HinoRepository interface {
	ListByColetanea(ctx context.Context, coletaneaID int64) ([]models.Hino, error)
	FindByNumero(ctx context.Context, coletaneaID int64, numero int) (*models.Hino, error)
	Create(ctx context.Context, hino *models.Hino) error
	MaxNumeracao(ctx context.Context, coletaneaID int64) (int, error)
	Update(ctx context.Context, hino *models.Hino) error
	StatsPorColetanea(ctx context.Context) ([]repository.StatsRow, error)
}

// ColetaneaRepository define as operações de LEITURA para coletâneas usadas em produção.
// Implementada por repository.SQLiteColetaneaRepository.
// Escrita (Create/Update/Delete) é feita no repositório concreto, não exposta na interface.
type ColetaneaRepository interface {
	List(ctx context.Context) ([]models.Coletanea, error)
	FindByCodigo(ctx context.Context, codigo string) (*models.Coletanea, error)
}
