package repository

import (
	"database/sql"
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/models"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}

	database.Migrate(db)

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})

	return db
}

func newTestHinoRepository(t *testing.T) *HinoRepository {
	t.Helper()

	return NewHinoRepository(newTestDB(t))
}

func newTestColetaneaRepository(t *testing.T) *ColetaneaRepository {
	t.Helper()

	return NewColetaneaRepository(newTestDB(t))
}

func newTestRepositories(t *testing.T) (
	*ColetaneaRepository,
	*HinoRepository,
) {
	t.Helper()

	db := newTestDB(t)

	return NewColetaneaRepository(db), NewHinoRepository(db)

}

func testColetanea() models.Coletanea {
	return models.Coletanea{
		Codigo: "CC",
		Titulo: "Cantor Cristão",
	}
}

func testHino(coletaneaID int64) models.Hino {
	numeracao := 1
	letra := "Grandioso és Tu, Senhor."
	creditos := "Autor desconhecido"

	return models.Hino{
		ColetaneaID: coletaneaID,
		Numeracao:   &numeracao,
		Titulo:      "Grandioso És Tu",
		Letra:       &letra,
		Creditos:    &creditos,
		Revisado:    true,
	}
}

func intPtr(value int) *int {
	return &value
}
