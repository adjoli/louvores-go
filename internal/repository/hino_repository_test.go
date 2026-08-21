package repository

import (
	"context"
	"errors"
	"testing"
)

func TestHinoRepositoryCreate(t *testing.T) {
	coletaneaRepo, hinoRepo := newTestRepositories(t)
	ctx := context.Background()

	coletanea := testColetanea()

	if err := coletaneaRepo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(Coletanea): %v", err)
	}

	hino := testHino(coletanea.ID)

	if err := hinoRepo.Create(ctx, &hino); err != nil {
		t.Fatalf("Create(Hino): %v", err)
	}

	if hino.ID == 0 {
		t.Fatal("expected ID to be populated")
	}
}

func TestHinoRepositoryFindByID(t *testing.T) {
	coletaneaRepo, hinoRepo := newTestRepositories(t)
	ctx := context.Background()

	coletanea := testColetanea()

	if err := coletaneaRepo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(Coletanea): %v", err)
	}

	expected := testHino(coletanea.ID)

	if err := hinoRepo.Create(ctx, &expected); err != nil {
		t.Fatalf("Create(Hino): %v", err)
	}

	actual, err := hinoRepo.FindByID(ctx, expected.ID)
	if err != nil {
		t.Fatalf("FindByID(): %v", err)
	}

	if actual.ID != expected.ID {
		t.Errorf("ID = %d, want %d", actual.ID, expected.ID)
	}

	if actual.ColetaneaID != expected.ColetaneaID {
		t.Errorf(
			"ColetaneaID = %d, want %d",
			actual.ColetaneaID,
			expected.ColetaneaID,
		)
	}

	if actual.Titulo != expected.Titulo {
		t.Errorf("Titulo = %q, want %q", actual.Titulo, expected.Titulo)
	}

	if actual.Numeracao == nil {
		t.Fatal("Numeracao = nil, want value")
	}

	if *actual.Numeracao != *expected.Numeracao {
		t.Errorf(
			"Numeracao = %d, want %d",
			*actual.Numeracao,
			*expected.Numeracao,
		)
	}

	if actual.Letra == nil {
		t.Fatal("Letra = nil, want value")
	}

	if *actual.Letra != *expected.Letra {
		t.Errorf(
			"Letra = %q, want %q",
			*actual.Letra,
			*expected.Letra,
		)
	}

	if actual.Creditos == nil {
		t.Fatal("Creditos = nil, want value")
	}

	if *actual.Creditos != *expected.Creditos {
		t.Errorf(
			"Creditos = %q, want %q",
			*actual.Creditos,
			*expected.Creditos,
		)
	}

	if actual.Revisado != expected.Revisado {
		t.Errorf(
			"Revisado = %v, want %v",
			actual.Revisado,
			expected.Revisado,
		)
	}
}

func TestHinoRepositoryFindByIDNotFound(t *testing.T) {
	repo := newTestHinoRepository(t)
	ctx := context.Background()

	_, err := repo.FindByID(ctx, 999)

	if !errors.Is(err, ErrHinoNotFound) {
		t.Fatalf(
			"FindByID() error = %v, want %v",
			err,
			ErrHinoNotFound,
		)
	}
}

func TestHinoRepositoryList(t *testing.T) {
	coletaneaRepo, hinoRepo := newTestRepositories(t)
	ctx := context.Background()

	coletanea := testColetanea()

	if err := coletaneaRepo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(Coletanea): %v", err)
	}

	first := testHino(coletanea.ID)
	first.Numeracao = intPtr(1)
	first.Titulo = "Primeiro Hino"

	second := testHino(coletanea.ID)
	second.Numeracao = intPtr(2)
	second.Titulo = "Segundo Hino"

	if err := hinoRepo.Create(ctx, &first); err != nil {
		t.Fatalf("Create(first): %v", err)
	}

	if err := hinoRepo.Create(ctx, &second); err != nil {
		t.Fatalf("Create(second): %v", err)
	}

	hinos, err := hinoRepo.List(ctx)
	if err != nil {
		t.Fatalf("List(): %v", err)
	}

	if len(hinos) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(hinos))
	}

	if hinos[0].ID != first.ID {
		t.Errorf("first ID = %d, want %d", hinos[0].ID, first.ID)
	}

	if hinos[1].ID != second.ID {
		t.Errorf("second ID = %d, want %d", hinos[1].ID, second.ID)
	}
}

func TestHinoRepositoryUpdate(t *testing.T) {
	coletaneaRepo, hinoRepo := newTestRepositories(t)
	ctx := context.Background()

	coletanea := testColetanea()

	if err := coletaneaRepo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(Coletanea): %v", err)
	}

	hino := testHino(coletanea.ID)

	if err := hinoRepo.Create(ctx, &hino); err != nil {
		t.Fatalf("Create(Hino): %v", err)
	}

	hino.Titulo = "Título atualizado"
	hino.Revisado = false

	if err := hinoRepo.Update(ctx, &hino); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	actual, err := hinoRepo.FindByID(ctx, hino.ID)
	if err != nil {
		t.Fatalf("FindByID(): %v", err)
	}

	if actual.Titulo != hino.Titulo {
		t.Errorf(
			"Titulo = %q, want %q",
			actual.Titulo,
			hino.Titulo,
		)
	}

	if actual.Revisado != hino.Revisado {
		t.Errorf(
			"Revisado = %v, want %v",
			actual.Revisado,
			hino.Revisado,
		)
	}
}

func TestHinoRepositoryUpdateNotFound(t *testing.T) {
	repo := newTestHinoRepository(t)
	ctx := context.Background()

	hino := testHino(1)
	hino.ID = 999

	err := repo.Update(ctx, &hino)

	if !errors.Is(err, ErrHinoNotFound) {
		t.Fatalf(
			"Update() error = %v, want %v",
			err,
			ErrHinoNotFound,
		)
	}
}

func TestHinoRepositoryDelete(t *testing.T) {
	coletaneaRepo, hinoRepo := newTestRepositories(t)
	ctx := context.Background()

	coletanea := testColetanea()

	if err := coletaneaRepo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(Coletanea): %v", err)
	}

	hino := testHino(coletanea.ID)

	if err := hinoRepo.Create(ctx, &hino); err != nil {
		t.Fatalf("Create(Hino): %v", err)
	}

	if err := hinoRepo.Delete(ctx, hino.ID); err != nil {
		t.Fatalf("Delete(): %v", err)
	}

	_, err := hinoRepo.FindByID(ctx, hino.ID)

	if !errors.Is(err, ErrHinoNotFound) {
		t.Fatalf(
			"FindByID() after Delete: error = %v, want %v",
			err,
			ErrHinoNotFound,
		)
	}
}

func TestHinoRepositoryDeleteNotFound(t *testing.T) {
	repo := newTestHinoRepository(t)
	ctx := context.Background()

	err := repo.Delete(ctx, 999)

	if !errors.Is(err, ErrHinoNotFound) {
		t.Fatalf(
			"Delete() error = %v, want %v",
			err,
			ErrHinoNotFound,
		)
	}
}

func TestHinoRepositoryListByColetanea(t *testing.T) {
	coletaneaRepo, hinoRepo := newTestRepositories(t)
	ctx := context.Background()

	first := testColetanea()
	second := testColetanea()
	second.Codigo = "HP"
	second.Titulo = "Harpa Cristã"

	if err := coletaneaRepo.Create(ctx, &first); err != nil {
		t.Fatalf("Create(first): %v", err)
	}

	if err := coletaneaRepo.Create(ctx, &second); err != nil {
		t.Fatalf("Create(second): %v", err)
	}

	inColetanea2 := testHino(second.ID)
	inColetanea2.Numeracao = intPtr(1)

	if err := hinoRepo.Create(ctx, &inColetanea2); err != nil {
		t.Fatalf("Create(inColetanea2): %v", err)
	}

	numeracao42 := 42
	hinoCC := testHino(first.ID)
	hinoCC.Numeracao = &numeracao42
	hinoCC.Titulo = "Grandioso Pai"

	if err := hinoRepo.Create(ctx, &hinoCC); err != nil {
		t.Fatalf("Create(hinoCC): %v", err)
	}

	hinos, err := hinoRepo.ListByColetanea(ctx, first.ID)
	if err != nil {
		t.Fatalf("ListByColetanea(): %v", err)
	}

	if len(hinos) != 1 {
		t.Fatalf("len(ListByColetanea()) = %d, want 1", len(hinos))
	}

	if hinos[0].ID != hinoCC.ID {
		t.Errorf("ID = %d, want %d", hinos[0].ID, hinoCC.ID)
	}
}

func TestHinoRepositoryFindByNumero(t *testing.T) {
	coletaneaRepo, hinoRepo := newTestRepositories(t)
	ctx := context.Background()

	coletanea := testColetanea()

	if err := coletaneaRepo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(Coletanea): %v", err)
	}

	expected := testHino(coletanea.ID)
	expected.Numeracao = intPtr(42)

	if err := hinoRepo.Create(ctx, &expected); err != nil {
		t.Fatalf("Create(Hino): %v", err)
	}

	actual, err := hinoRepo.FindByNumero(ctx, coletanea.ID, 42)
	if err != nil {
		t.Fatalf("FindByNumero(): %v", err)
	}

	if actual.ID != expected.ID {
		t.Errorf("ID = %d, want %d", actual.ID, expected.ID)
	}

	if actual.Numeracao == nil || *actual.Numeracao != 42 {
		t.Errorf("Numeracao = %v, want 42", actual.Numeracao)
	}
}

func TestHinoRepositoryFindByNumeroNotFound(t *testing.T) {
	coletaneaRepo, hinoRepo := newTestRepositories(t)
	ctx := context.Background()

	coletanea := testColetanea()

	if err := coletaneaRepo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(Coletanea): %v", err)
	}

	_, err := hinoRepo.FindByNumero(ctx, coletanea.ID, 999)

	if !errors.Is(err, ErrHinoNotFound) {
		t.Fatalf(
			"FindByNumero() error = %v, want %v",
			err,
			ErrHinoNotFound,
		)
	}
}

func TestHinoRepositoryStatsPorColetanea(t *testing.T) {
	coletaneaRepo, hinoRepo := newTestRepositories(t)
	ctx := context.Background()

	coletanea := testColetanea()

	if err := coletaneaRepo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(Coletanea): %v", err)
	}

	withLetra := testHino(coletanea.ID)
	withLetra.Numeracao = intPtr(1)
	withLetra.Titulo = "Um"

	secondWithLetra := testHino(coletanea.ID)
	secondWithLetra.Numeracao = intPtr(2)
	secondWithLetra.Titulo = "Dois"
	secondWithLetra.Revisado = false

	semLetra := testHino(coletanea.ID)
	semLetra.Numeracao = intPtr(3)
	semLetra.Titulo = "Três"
	semLetra.Letra = nil
	semLetra.Revisado = false

	if err := hinoRepo.Create(ctx, &withLetra); err != nil {
		t.Fatalf("Create(withLetra): %v", err)
	}

	if err := hinoRepo.Create(ctx, &secondWithLetra); err != nil {
		t.Fatalf("Create(secondWithLetra): %v", err)
	}

	if err := hinoRepo.Create(ctx, &semLetra); err != nil {
		t.Fatalf("Create(semLetra): %v", err)
	}

	rows, err := hinoRepo.StatsPorColetanea(ctx)
	if err != nil {
		t.Fatalf("StatsPorColetanea(): %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}

	row := rows[0]

	if row.Codigo != "CC" || row.Total != 3 || row.ComLetra != 2 || row.Revisados != 1 {
		t.Errorf("row = %+v", row)
	}
}
