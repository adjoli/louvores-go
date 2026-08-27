package services

import (
	"context"
	"errors"
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/models"
	"github.com/adjoli/louvores-go/internal/repository"
)

// seedHinos cria um banco em memória com a coletânea "CC" e dois hinos:
// 42 com letra e 43 sem letra. Retorna os dois repositórios populados.
func seedHinos(t *testing.T) (*repository.SQLiteHinoRepository, *repository.SQLiteColetaneaRepository) {
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
	hinoRepo := repository.NewSQLiteHinoRepository(conn)
	if err := hinoRepo.Create(ctx, &models.Hino{
		ColetaneaID: coletanea.ID,
		Numeracao:   &num,
		Titulo:      "Grandioso Pai",
		Letra:       &letra,
	}); err != nil {
		t.Fatalf("criar hino 42: %v", err)
	}

	semLetraNum := 43
	if err := hinoRepo.Create(ctx, &models.Hino{
		ColetaneaID: coletanea.ID,
		Numeracao:   &semLetraNum,
		Titulo:      "Sem Letra",
	}); err != nil {
		t.Fatalf("criar hino 43: %v", err)
	}

	return hinoRepo, coletaneaRepo
}

func newTestHinoService(t *testing.T) *HinoService {
	t.Helper()
	hinoRepo, coletaneaRepo := seedHinos(t)
	return NewHinoService(hinoRepo, coletaneaRepo, "")
}

func TestListarColetaneas(t *testing.T) {
	svc := newTestHinoService(t)

	coletaneas, err := svc.ListarColetaneas(context.Background())
	if err != nil {
		t.Fatalf("ListarColetaneas: %v", err)
	}
	if len(coletaneas) != 1 {
		t.Fatalf("len = %d, esperado 1", len(coletaneas))
	}
	if coletaneas[0].Codigo != "CC" || coletaneas[0].Titulo != "Cantor Cristão" {
		t.Errorf("coletâneas = %+v", coletaneas)
	}
}

func TestListarHinos(t *testing.T) {
	svc := newTestHinoService(t)

	hinos, err := svc.ListarHinos(context.Background(), "CC")
	if err != nil {
		t.Fatalf("ListarHinos: %v", err)
	}
	if len(hinos) != 2 {
		t.Fatalf("len = %d, esperado 2", len(hinos))
	}
	if hinos[0].Titulo != "Grandioso Pai" || hinos[1].Titulo != "Sem Letra" {
		t.Errorf("hinos fora de ordem: %+v", hinos)
	}
}

func TestListarHinosColetaneaInexistente(t *testing.T) {
	svc := newTestHinoService(t)

	_, err := svc.ListarHinos(context.Background(), "XX")
	if !errors.Is(err, repository.ErrColetaneaNotFound) {
		t.Fatalf("erro = %v, esperado %v", err, repository.ErrColetaneaNotFound)
	}
}

func TestObterHino(t *testing.T) {
	svc := newTestHinoService(t)

	hino, err := svc.ObterHino(context.Background(), "CC", 42)
	if err != nil {
		t.Fatalf("ObterHino: %v", err)
	}
	if hino.Titulo != "Grandioso Pai" {
		t.Errorf("titulo = %q", hino.Titulo)
	}
	if hino.Numeracao == nil || *hino.Numeracao != 42 {
		t.Errorf("numeracao = %v, esperado 42", hino.Numeracao)
	}
	if hino.Letra == nil || *hino.Letra == "" {
		t.Error("hino deveria ter letra")
	}
}

func TestObterHinoNaoEncontrado(t *testing.T) {
	svc := newTestHinoService(t)

	if _, err := svc.ObterHino(context.Background(), "CC", 999); !errors.Is(err, repository.ErrHinoNotFound) {
		t.Fatalf("erro = %v, esperado %v", err, repository.ErrHinoNotFound)
	}
	if _, err := svc.ObterHino(context.Background(), "XX", 42); !errors.Is(err, repository.ErrColetaneaNotFound) {
		t.Fatalf("erro = %v, esperado %v", err, repository.ErrColetaneaNotFound)
	}
}

func numero(n int) *int { return &n }

func TestTextosSlidesNaoCorinhos(t *testing.T) {
	c := models.Coletanea{Codigo: "CC", Titulo: "Cantor Cristão"}
	h := models.Hino{Titulo: "Grandioso Pai", Numeracao: numero(42)}

	titulo, subtitulo, tituloSlides := textosSlides(c, h)
	if titulo != "Grandioso Pai" {
		t.Errorf("titulo = %q", titulo)
	}
	if subtitulo != "Cantor Cristão - 42" {
		t.Errorf("subtitulo = %q", subtitulo)
	}
	if tituloSlides != "42CC - Grandioso Pai" {
		t.Errorf("tituloSlides = %q", tituloSlides)
	}
}

func TestTextosSlidesNomeEmMaiusculas(t *testing.T) {
	c := models.Coletanea{Codigo: "CC", Titulo: "CANTOR CRISTÃO"}
	h := models.Hino{Titulo: "Grandioso Pai", Numeracao: numero(7)}

	_, subtitulo, tituloSlides := textosSlides(c, h)
	if subtitulo != "Cantor Cristão - 7" {
		t.Errorf("subtitulo = %q (Title Case esperado)", subtitulo)
	}
	if tituloSlides != "7CC - Grandioso Pai" {
		t.Errorf("tituloSlides = %q", tituloSlides)
	}
}

func TestTextosSlidesNomeJaCapitalizadoPreservado(t *testing.T) {
	c := models.Coletanea{Codigo: "HCC", Titulo: "Hinário para o Culto Cristão"}
	h := models.Hino{Titulo: "Louvor", Numeracao: numero(3)}

	_, subtitulo, _ := textosSlides(c, h)
	if subtitulo != "Hinário para o Culto Cristão - 3" {
		t.Errorf("subtitulo = %q (preposições preservadas)", subtitulo)
	}
}

func TestTextosSlidesCorinhos(t *testing.T) {
	c := models.Coletanea{Codigo: "COR", Titulo: "Corinhos"}
	h := models.Hino{Titulo: "Grandioso Pai", Numeracao: numero(12)}

	titulo, subtitulo, tituloSlides := textosSlides(c, h)
	if titulo != "Grandioso Pai" {
		t.Errorf("titulo = %q", titulo)
	}
	if subtitulo != "" {
		t.Errorf("subtitulo = %q, esperado vazio", subtitulo)
	}
	if tituloSlides != "Grandioso Pai" {
		t.Errorf("tituloSlides = %q, esperado título original", tituloSlides)
	}
}
