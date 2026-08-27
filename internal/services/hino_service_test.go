package services

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
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

// templateRealLocaliza o template default para os testes de geração de slides.
// Skippa se o template não estiver presente (ex.: repositório clonado sem dados).
func templateRealLocaliza(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "data", "templates", "default.pptx")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("template não encontrado: %v", err)
	}
	return p
}

func TestGerarSlidesColetanea(t *testing.T) {
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := database.Migrate(conn); err != nil {
		t.Fatalf("migrar: %v", err)
	}

	ctx := context.Background()
	cr := repository.NewSQLiteColetaneaRepository(conn)
	coletanea := &models.Coletanea{Codigo: "CC", Titulo: "Cantor Cristão"}
	if err := cr.Create(ctx, coletanea); err != nil {
		t.Fatalf("criar coletânea: %v", err)
	}

	hr := repository.NewSQLiteHinoRepository(conn)
	num1 := 1
	letra := "Estrofe um\n\n    Refrão"
	if err := hr.Create(ctx, &models.Hino{
		ColetaneaID: coletanea.ID, Numeracao: &num1,
		Titulo: "Grandioso Pai", Letra: &letra, Revisado: true,
	}); err != nil {
		t.Fatalf("criar hino 1: %v", err)
	}
	num2 := 2
	if err := hr.Create(ctx, &models.Hino{
		ColetaneaID: coletanea.ID, Numeracao: &num2,
		Titulo: "Sem Revisar", Revisado: false,
	}); err != nil {
		t.Fatalf("criar hino 2: %v", err)
	}

	svc := NewHinoService(hr, cr, templateRealLocaliza(t))
	res, err := svc.GerarSlidesColetanea(ctx, "CC", svc.TemplatePath())
	if err != nil {
		t.Fatalf("GerarSlidesColetanea: %v", err)
	}
	if res.Gerados != 1 {
		t.Errorf("gerados = %d, esperado 1", res.Gerados)
	}
	if res.Pulados != 1 {
		t.Errorf("pulados = %d, esperado 1", res.Pulados)
	}

	zr, err := zip.NewReader(bytes.NewReader(res.Zip), int64(len(res.Zip)))
	if err != nil {
		t.Fatalf("abrir zip: %v", err)
	}
	if len(zr.File) != 1 {
		t.Fatalf("arquivos no zip = %d, esperado 1", len(zr.File))
	}
	if zr.File[0].Name != "CC-001-GRANDIOSO_PAI.pptx" {
		t.Errorf("nome do arquivo = %q", zr.File[0].Name)
	}
}

func TestGerarSlidesColetaneaInexistente(t *testing.T) {
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := database.Migrate(conn); err != nil {
		t.Fatalf("migrar: %v", err)
	}

	hr := repository.NewSQLiteHinoRepository(conn)
	cr := repository.NewSQLiteColetaneaRepository(conn)
	svc := NewHinoService(hr, cr, templateRealLocaliza(t))

	if _, err := svc.GerarSlidesColetanea(context.Background(), "XX", svc.TemplatePath()); !errors.Is(err, repository.ErrColetaneaNotFound) {
		t.Fatalf("erro = %v, esperado %v", err, repository.ErrColetaneaNotFound)
	}
}
