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
}
