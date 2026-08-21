package services

import (
	"context"

	"github.com/adjoli/louvores-go/internal/models"
	"github.com/adjoli/louvores-go/internal/repository"
)

// HinoService concentra as operações de leitura sobre coletâneas e hinos.
//
// Ele resolve a navegação pela chave de negócio (código da coletânea +
// numeração do hino, ex.: CC/42), traduzindo-a para os IDs internos usados
// pelo repositório. Os erros sentinela do repositório
// (repository.ErrColetaneaNotFound, repository.ErrHinoNotFound) propagam
// sem alteração para que a camada superior os mapeie para respostas HTTP.
type HinoService struct {
	hinoRepo      *repository.HinoRepository
	coletaneaRepo *repository.ColetaneaRepository
}

// NewHinoService cria um novo HinoService com os repositórios fornecidos.
func NewHinoService(
	hinoRepo *repository.HinoRepository,
	coletaneaRepo *repository.ColetaneaRepository,
) *HinoService {
	return &HinoService{
		hinoRepo:      hinoRepo,
		coletaneaRepo: coletaneaRepo,
	}
}

// ListarColetaneas retorna todas as coletâneas cadastradas.
func (s *HinoService) ListarColetaneas(
	ctx context.Context,
) ([]models.Coletanea, error) {
	return s.coletaneaRepo.List(ctx)
}

// ListarHinos retorna os hinos da coletânea identificada pelo código curto
// (ex.: "CC"), ordenados pela numeração.
// Retorna repository.ErrColetaneaNotFound se o código não existir.
func (s *HinoService) ListarHinos(
	ctx context.Context,
	codigoColetanea string,
) ([]models.Hino, error) {
	coletanea, err := s.coletaneaRepo.FindByCodigo(ctx, codigoColetanea)
	if err != nil {
		return nil, err
	}

	return s.hinoRepo.ListByColetanea(ctx, coletanea.ID)
}

// ObterHino retorna o hino identificado pela combinação código da coletânea
// + numeração (ex.: CC/42).
// Retorna repository.ErrColetaneaNotFound ou repository.ErrHinoNotFound.
func (s *HinoService) ObterHino(
	ctx context.Context,
	codigoColetanea string,
	numero int,
) (*models.Hino, error) {
	coletanea, err := s.coletaneaRepo.FindByCodigo(ctx, codigoColetanea)
	if err != nil {
		return nil, err
	}

	return s.hinoRepo.FindByNumero(ctx, coletanea.ID, numero)
}
