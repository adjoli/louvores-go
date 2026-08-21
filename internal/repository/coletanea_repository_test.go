package repository

import (
	"context"
	"errors"
	"testing"
)

func TestColetaneaRepositoryCreate(t *testing.T) {
	repo := newTestColetaneaRepository(t)
	ctx := context.Background()

	coletanea := testColetanea()

	if err := repo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	if coletanea.ID == 0 {
		t.Fatal("expected ID to be populated")
	}
}

func TestColetaneaRepositoryFindByID(t *testing.T) {
	repo := newTestColetaneaRepository(t)
	ctx := context.Background()

	expected := testColetanea()

	if err := repo.Create(ctx, &expected); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	actual, err := repo.FindByID(ctx, expected.ID)
	if err != nil {
		t.Fatalf("FindByID(): %v", err)
	}

	if actual.ID != expected.ID {
		t.Errorf("ID = %d, want %d", actual.ID, expected.ID)
	}

	if actual.Codigo != expected.Codigo {
		t.Errorf("Codigo = %q, want %q", actual.Codigo, expected.Codigo)
	}

	if actual.Titulo != expected.Titulo {
		t.Errorf("Titulo = %q, want %q", actual.Titulo, expected.Titulo)
	}
}

func TestColetaneaRepositoryFindByIDNotFound(t *testing.T) {
	repo := newTestColetaneaRepository(t)
	ctx := context.Background()

	_, err := repo.FindByID(ctx, 999)

	if !errors.Is(err, ErrColetaneaNotFound) {
		t.Fatalf("FindByID() error = %v, want %v", err, ErrColetaneaNotFound)
	}
}

func TestColetaneaRepositoryFindByCodigo(t *testing.T) {
	repo := newTestColetaneaRepository(t)
	ctx := context.Background()

	expected := testColetanea()

	if err := repo.Create(ctx, &expected); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	actual, err := repo.FindByCodigo(ctx, "CC")
	if err != nil {
		t.Fatalf("FindByCodigo(): %v", err)
	}

	if actual.ID != expected.ID {
		t.Errorf("ID = %d, want %d", actual.ID, expected.ID)
	}

	if actual.Codigo != expected.Codigo {
		t.Errorf("Codigo = %q, want %q", actual.Codigo, expected.Codigo)
	}

	if actual.Titulo != expected.Titulo {
		t.Errorf("Titulo = %q, want %q", actual.Titulo, expected.Titulo)
	}
}

func TestColetaneaRepositoryFindByCodigoNotFound(t *testing.T) {
	repo := newTestColetaneaRepository(t)
	ctx := context.Background()

	_, err := repo.FindByCodigo(ctx, "XX")

	if !errors.Is(err, ErrColetaneaNotFound) {
		t.Fatalf("FindByCodigo() error = %v, want %v", err, ErrColetaneaNotFound)
	}
}

func TestColetaneaRepositoryList(t *testing.T) {
	repo := newTestColetaneaRepository(t)
	ctx := context.Background()

	first := testColetanea()
	first.Codigo = "CC"
	first.Titulo = "Cantor Cristão"

	second := testColetanea()
	second.Codigo = "HCC"
	second.Titulo = "Hinário para o Culto Cristão"

	if err := repo.Create(ctx, &first); err != nil {
		t.Fatalf("Create(first): %v", err)
	}

	if err := repo.Create(ctx, &second); err != nil {
		t.Fatalf("Create(second): %v", err)
	}

	coletaneas, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List(): %v", err)
	}

	if len(coletaneas) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(coletaneas))
	}

	if coletaneas[0].ID != first.ID {
		t.Errorf("first ID = %d, want %d", coletaneas[0].ID, first.ID)
	}

	if coletaneas[1].ID != second.ID {
		t.Errorf("second ID = %d, want %d", coletaneas[1].ID, second.ID)
	}
}

func TestColetaneaRepositoryUpdate(t *testing.T) {
	repo := newTestColetaneaRepository(t)
	ctx := context.Background()

	coletanea := testColetanea()

	if err := repo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	coletanea.Codigo = "HC"
	coletanea.Titulo = "Hinário Congregacional"

	if err := repo.Update(ctx, &coletanea); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	actual, err := repo.FindByID(ctx, coletanea.ID)
	if err != nil {
		t.Fatalf("FindByID(): %v", err)
	}

	if actual.Codigo != coletanea.Codigo {
		t.Errorf("Codigo = %q, want %q", actual.Codigo, coletanea.Codigo)
	}

	if actual.Titulo != coletanea.Titulo {
		t.Errorf("Titulo = %q, want %q", actual.Titulo, coletanea.Titulo)
	}
}

func TestColetaneaRepositoryUpdateNotFound(t *testing.T) {
	repo := newTestColetaneaRepository(t)
	ctx := context.Background()

	coletanea := testColetanea()
	coletanea.ID = 999

	err := repo.Update(ctx, &coletanea)

	if !errors.Is(err, ErrColetaneaNotFound) {
		t.Fatalf("Update() error = %v, want %v", err, ErrColetaneaNotFound)
	}
}

func TestColetaneaRepositoryDelete(t *testing.T) {
	repo := newTestColetaneaRepository(t)
	ctx := context.Background()

	coletanea := testColetanea()

	if err := repo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	if err := repo.Delete(ctx, coletanea.ID); err != nil {
		t.Fatalf("Delete(): %v", err)
	}

	_, err := repo.FindByID(ctx, coletanea.ID)

	if !errors.Is(err, ErrColetaneaNotFound) {
		t.Fatalf("FindByID() after Delete: error = %v, want %v", err, ErrColetaneaNotFound)
	}
}

func TestColetaneaRepositoryDeleteNotFound(t *testing.T) {
	repo := newTestColetaneaRepository(t)
	ctx := context.Background()

	err := repo.Delete(ctx, 999)

	if !errors.Is(err, ErrColetaneaNotFound) {
		t.Fatalf("Delete() error = %v, want %v", err, ErrColetaneaNotFound)
	}
}
