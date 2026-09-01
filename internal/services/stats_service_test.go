package services

import (
	"context"
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/models"
	"github.com/adjoli/louvores-go/internal/repository"
)

func seedHinoRepo(t *testing.T) *repository.SQLiteHinoRepository {
	t.Helper()
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := database.Migrate(conn); err != nil {
		t.Fatalf("migrar: %v", err)
	}

	ctx := context.Background()
	coletaneaRepo := repository.NewSQLiteColetaneaRepository(conn)
	coletanea := &models.Coletanea{Codigo: "CC", Titulo: "Cantor Cristão"}
	if err := coletaneaRepo.Create(ctx, coletanea); err != nil {
		t.Fatalf("criar coletânea: %v", err)
	}

	num := 42
	letra := "Estrofe um\n\n    Refrão"
	creditos := "Autor"
	hinoRepo := repository.NewSQLiteHinoRepository(conn)
	if err := hinoRepo.Create(ctx, &models.Hino{
		ColetaneaID: coletanea.ID,
		Numeracao:   &num,
		Titulo:      "Grandioso Pai",
		Letra:       &letra,
		Creditos:    &creditos,
	}); err != nil {
		t.Fatalf("criar hino: %v", err)
	}

	return hinoRepo
}

func TestObterStats(t *testing.T) {
	repo := seedHinoRepo(t)
	svc := NewStatsService(repo)

	stats, err := svc.ObterStats(context.Background())
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("len = %d, esperado 1", len(stats))
	}
	s := stats[0]
	if s.Codigo != "CC" || s.Total != 1 || s.ComLetra != 1 || s.Revisados != 0 {
		t.Errorf("stats = %+v", s)
	}
	if s.NaoRevisados != 1 {
		t.Errorf("nao_revisados = %d, esperado 1", s.NaoRevisados)
	}
	if s.Percentual != 100 {
		t.Errorf("percentual = %v, esperado 100", s.Percentual)
	}
	// Sem hinos revisados, o percentual de revisados é 0.
	if s.PercentualRevisados != 0 {
		t.Errorf("percentual_revisados = %v, esperado 0", s.PercentualRevisados)
	}
}

// TestObterStatsPercentualRevisados valida o cálculo do percentual de hinos
// revisados em relação aos hinos com letra: 4 hinos no total, 3 com letra,
// 2 revisados → percentual = 75, percentual_revisados = 66.66...
func TestObterStatsPercentualRevisados(t *testing.T) {
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := database.Migrate(conn); err != nil {
		t.Fatalf("migrar: %v", err)
	}

	ctx := context.Background()
	coletaneaRepo := repository.NewSQLiteColetaneaRepository(conn)
	coletanea := &models.Coletanea{Codigo: "CC", Titulo: "Cantor Cristão"}
	if err := coletaneaRepo.Create(ctx, coletanea); err != nil {
		t.Fatalf("criar coletânea: %v", err)
	}

	letra := "Estrofe"
	hinoRepo := repository.NewSQLiteHinoRepository(conn)

	semear := func(numero int, letra *string, revisado bool) {
		t.Helper()
		if err := hinoRepo.Create(ctx, &models.Hino{
			ColetaneaID: coletanea.ID,
			Numeracao:   &numero,
			Titulo:      "Hino",
			Letra:       letra,
			Revisado:    revisado,
		}); err != nil {
			t.Fatalf("criar hino %d: %v", numero, err)
		}
	}

	semear(1, &letra, true)  // com letra, revisado
	semear(2, &letra, true)  // com letra, revisado
	semear(3, &letra, false) // com letra, não revisado
	semear(4, nil, false)    // sem letra

	svc := NewStatsService(hinoRepo)
	stats, err := svc.ObterStats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("len = %d, esperado 1", len(stats))
	}
	s := stats[0]
	if s.Total != 4 || s.ComLetra != 3 || s.Revisados != 2 {
		t.Errorf("stats = %+v", s)
	}
	if s.Percentual != 75 {
		t.Errorf("percentual = %v, esperado 75", s.Percentual)
	}
	if got, want := s.PercentualRevisados, 200.0/3.0; abs(got-want) > 1e-9 {
		t.Errorf("percentual_revisados = %v, esperado %v", got, want)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
