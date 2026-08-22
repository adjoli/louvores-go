package services

import (
	"context"

	"github.com/adjoli/louvores-go/internal/repository"
)

// ColetaneaStats é a visão agregada de uma coletânea para o endpoint /api/stats.
// Os campos derivados (NaoRevisados, Percentual) são calculados aqui, a partir
// da linha crua do repositório (repository.StatsRow).
type ColetaneaStats struct {
	Codigo       string  `json:"codigo"`
	Titulo       string  `json:"titulo"`
	Total        int     `json:"total"`
	ComLetra     int     `json:"com_letra"`
	Revisados    int     `json:"revisados"`
	NaoRevisados int     `json:"nao_revisados"`
	Percentual   float64 `json:"percentual"`
}

type StatsService struct {
	repo *repository.HinoRepository
}

func NewStatsService(repo *repository.HinoRepository) *StatsService {
	return &StatsService{repo: repo}
}

func (s *StatsService) ObterStats(ctx context.Context) ([]ColetaneaStats, error) {
	rows, err := s.repo.StatsPorColetanea(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]ColetaneaStats, 0, len(rows))
	for _, r := range rows {
		naoRevisados := r.ComLetra - r.Revisados
		percentual := 0.0
		if r.Total > 0 {
			percentual = float64(r.ComLetra) / float64(r.Total) * 100
		}
		out = append(out, ColetaneaStats{
			Codigo:       r.Codigo,
			Titulo:       r.Titulo,
			Total:        r.Total,
			ComLetra:     r.ComLetra,
			Revisados:    r.Revisados,
			NaoRevisados: naoRevisados,
			Percentual:   percentual,
		})
	}
	return out, nil
}
