package services

import (
	"context"
)

// ColetaneaStats é a visão agregada de uma coletânea para o endpoint /api/stats.
// Os campos derivados (NaoRevisados, Percentual, PercentualRevisados) são
// calculados aqui, a partir da linha crua do repositório (StatsRow).
type ColetaneaStats struct {
	Codigo              string  `json:"codigo"`
	Titulo              string  `json:"titulo"`
	Total               int     `json:"total"`
	ComLetra            int     `json:"com_letra"`
	Revisados           int     `json:"revisados"`
	NaoRevisados        int     `json:"nao_revisados"`
	Percentual          float64 `json:"percentual"`
	PercentualRevisados float64 `json:"percentual_revisados"`
}

type StatsService struct {
	repo HinoRepository
}

func NewStatsService(repo HinoRepository) *StatsService {
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

		// Percentual de hinos com letra em relação ao total.
		percentual := 0.0
		if r.Total > 0 {
			percentual = float64(r.ComLetra) / float64(r.Total) * 100
		}

		// Percentual de hinos revisados em relação aos hinos com letra.
		percentualRevisados := 0.0
		if r.ComLetra > 0 {
			percentualRevisados = float64(r.Revisados) / float64(r.ComLetra) * 100
		}

		out = append(out, ColetaneaStats{
			Codigo:              r.Codigo,
			Titulo:              r.Titulo,
			Total:               r.Total,
			ComLetra:            r.ComLetra,
			Revisados:           r.Revisados,
			NaoRevisados:        naoRevisados,
			Percentual:          percentual,
			PercentualRevisados: percentualRevisados,
		})
	}
	return out, nil
}
