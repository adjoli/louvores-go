package services

import (
	"context"
	"errors"
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/models"
	"github.com/adjoli/louvores-go/internal/repository"
)

// seedCorinhos cria um banco em memória com a coletânea Corinhos e dois hinos
// numerados 7 e 10, para verificar o cálculo da próxima numeração.
func seedCorinhos(t *testing.T) (*repository.SQLiteHinoRepository, *repository.SQLiteColetaneaRepository) {
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
	cr := repository.NewSQLiteColetaneaRepository(conn)
	coletanea := &models.Coletanea{Codigo: CodigoCorinhos, Titulo: "Corinhos"}
	if err := cr.Create(ctx, coletanea); err != nil {
		t.Fatalf("criar coletânea: %v", err)
	}

	hr := repository.NewSQLiteHinoRepository(conn)
	for _, n := range []int{7, 10} {
		numero := n
		if err := hr.Create(ctx, &models.Hino{
			ColetaneaID: coletanea.ID,
			Numeracao:   &numero,
			Titulo:      "Existente",
		}); err != nil {
			t.Fatalf("criar hino %d: %v", n, err)
		}
	}

	return hr, cr
}

func TestCriarHinoCorinhos(t *testing.T) {
	hr, cr := seedCorinhos(t)
	svc := NewHinoService(hr, cr, "")

	ctx := context.Background()

	hino, err := svc.CriarHino(ctx, CodigoCorinhos, HinoCreate{
		Titulo:   "Novo Corinho",
		Letra:    "letra em minúsculas",
		Creditos: "Autor",
		Revisado: true,
	})
	if err != nil {
		t.Fatalf("CriarHino: %v", err)
	}

	if hino.Numeracao == nil || *hino.Numeracao != 11 {
		t.Errorf("numeracao = %v, esperado 11 (maior 10 + 1)", hino.Numeracao)
	}
	if hino.Letra == nil || *hino.Letra != "Letra Em Minúsculas" {
		t.Errorf("letra = %v, esperado Title Case", hino.Letra)
	}
	if hino.Creditos == nil || *hino.Creditos != "Autor" {
		t.Errorf("creditos = %v", hino.Creditos)
	}
	if hino.ID == 0 {
		t.Error("ID deveria ser populado após o Create")
	}

	// Campos vazios viram nil (estado "sem letra"/"sem créditos").
	outro, err := svc.CriarHino(ctx, CodigoCorinhos, HinoCreate{Titulo: "Sem Letra"})
	if err != nil {
		t.Fatalf("CriarHino (vazio): %v", err)
	}
	if outro.Numeracao == nil || *outro.Numeracao != 12 {
		t.Errorf("numeracao = %v, esperado 12", outro.Numeracao)
	}
	if outro.Letra != nil {
		t.Errorf("letra = %v, esperado nil", outro.Letra)
	}
	if outro.Creditos != nil {
		t.Errorf("creditos = %v, esperado nil", outro.Creditos)
	}
}

func TestCriarHinoColetaneaNaoPermitida(t *testing.T) {
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
	if err := cr.Create(ctx, &models.Coletanea{Codigo: "CC", Titulo: "Cantor Cristão"}); err != nil {
		t.Fatalf("criar coletânea: %v", err)
	}

	svc := NewHinoService(repository.NewSQLiteHinoRepository(conn), cr, "")

	_, err = svc.CriarHino(ctx, "CC", HinoCreate{Titulo: "Proibido"})
	if !errors.Is(err, ErrNovoHinoNaoPermitido) {
		t.Fatalf("erro = %v, esperado %v", err, ErrNovoHinoNaoPermitido)
	}
}

func TestCriarHinoColetaneaInexistente(t *testing.T) {
	hr, cr := seedCorinhos(t)
	svc := NewHinoService(hr, cr, "")

	_, err := svc.CriarHino(context.Background(), "ZZ", HinoCreate{Titulo: "X"})
	if !errors.Is(err, repository.ErrColetaneaNotFound) {
		t.Fatalf("erro = %v, esperado %v", err, repository.ErrColetaneaNotFound)
	}
}
