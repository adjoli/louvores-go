package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/repository"
)

func TestLerDestinoAmbiguo(t *testing.T) {
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco em memória: %v", err)
	}
	defer conn.Close()

	// Fixture legacy deliberadamente não migrada (sem índice unique, sem Migrate):
	legacySchema := `
	CREATE TABLE coletanea (
		id     INTEGER PRIMARY KEY AUTOINCREMENT,
		codigo TEXT NOT NULL,
		titulo TEXT NOT NULL
	);
	CREATE TABLE hino (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		coletanea_id INTEGER NOT NULL REFERENCES coletanea(id),
		numeracao    INTEGER,
		titulo       TEXT NOT NULL,
		letra        TEXT,
		creditos     TEXT,
		revisado     INTEGER NOT NULL DEFAULT 0
	);
	`
	if _, err := conn.Exec(legacySchema); err != nil {
		t.Fatalf("criar schema legado: %v", err)
	}

	if _, err := conn.Exec(`INSERT INTO coletanea (id, codigo, titulo) VALUES (1, 'CC', 'Cantor Cristão')`); err != nil {
		t.Fatalf("inserir coletanea: %v", err)
	}

	// Insere hinos duplicados deliberadamente (mesma coletanea e mesma numeração 42):
	if _, err := conn.Exec(`
		INSERT INTO hino (coletanea_id, numeracao, titulo, letra, creditos, revisado)
		VALUES (1, 42, 'Primeiro', 'Letra Um', 'Autor Um', 0),
		       (1, 42, 'Segundo', 'Letra Dois', 'Autor Dois', 0)
	`); err != nil {
		t.Fatalf("inserir duplicatas de hino: %v", err)
	}

	// Insere um hino único regular:
	if _, err := conn.Exec(`
		INSERT INTO hino (coletanea_id, numeracao, titulo, letra, creditos, revisado)
		VALUES (1, 10, 'Dez', 'Letra Dez', 'Autor Dez', 1)
	`); err != nil {
		t.Fatalf("inserir hino 10: %v", err)
	}

	repo := repository.NewImportRepository(conn)
	ctx := context.Background()

	// 1. Busca hino duplicado -> deve retornar ErrDestinoAmbiguo
	_, err = repo.LerDestino(ctx, "CC", 42)
	if err == nil {
		t.Fatalf("esperava erro para destino ambíguo CC/42, obteve sucesso")
	}
	if !errors.Is(err, repository.ErrDestinoAmbiguo) {
		t.Fatalf("esperava ErrDestinoAmbiguo, obteve: %v", err)
	}

	// 2. Coletânea inexistente -> ErrColetaneaNotFound (e wraps sql.ErrNoRows)
	_, err = repo.LerDestino(ctx, "INEXISTENTE", 1)
	if err == nil {
		t.Fatalf("esperava erro para coletânea inexistente")
	}
	if !errors.Is(err, repository.ErrColetaneaNotFound) {
		t.Fatalf("esperava ErrColetaneaNotFound, obteve: %v", err)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("esperava que ErrColetaneaNotFound embrulhasse sql.ErrNoRows")
	}

	// 3. Hino inexistente -> ErrHinoNotFound (e wraps sql.ErrNoRows)
	_, err = repo.LerDestino(ctx, "CC", 999)
	if err == nil {
		t.Fatalf("esperava erro para hino inexistente")
	}
	if !errors.Is(err, repository.ErrHinoNotFound) {
		t.Fatalf("esperava ErrHinoNotFound, obteve: %v", err)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("esperava que ErrHinoNotFound embrulhasse sql.ErrNoRows")
	}

	// 4. Hino único existente -> retorna DestinoImport populado
	d, err := repo.LerDestino(ctx, "CC", 10)
	if err != nil {
		t.Fatalf("esperava sucesso para CC/10, obteve: %v", err)
	}
	if d.Codigo != "CC" || d.Numero != 10 || d.Titulo != "Dez" || !d.Revisado {
		t.Fatalf("dados inesperados em DestinoImport: %+v", d)
	}
	if d.Letra == nil || *d.Letra != "Letra Dez" {
		t.Fatalf("letra inesperada: %v", d.Letra)
	}
	if d.Creditos == nil || *d.Creditos != "Autor Dez" {
		t.Fatalf("creditos inesperados: %v", d.Creditos)
	}
}

func TestSQLGuardaDestino(t *testing.T) {
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	defer conn.Close()

	if err := database.Migrate(conn); err != nil {
		t.Fatalf("migrar: %v", err)
	}

	repo := repository.NewImportRepository(conn)

	// Validação de alias: aceita apenas "h"
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("esperava pânico com alias inválido 'x'")
		}
	}()
	_ = repo.SQLGuardaDestino("x")
}

func TestSQLGuardaDestinoPredicado(t *testing.T) {
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	defer conn.Close()

	if err := database.Migrate(conn); err != nil {
		t.Fatalf("migrar: %v", err)
	}

	repo := repository.NewImportRepository(conn)
	guarda := repo.SQLGuardaDestino("h")
	if !strings.Contains(guarda, "h.revisado = 0") {
		t.Errorf("guarda não contém h.revisado = 0: %s", guarda)
	}

	if _, err := conn.Exec(`INSERT INTO coletanea VALUES (1, 'CC', 'Cantor Cristão')`); err != nil {
		t.Fatalf("inserir coletanea: %v", err)
	}
	// 1: sem letra, não revisado (elegível) -> 1
	// 2: espaço em branco ASCII, não revisado (elegível) -> 1
	// 3: com letra, não revisado (não elegível) -> 0
	// 4: sem letra, revisado (não elegível) -> 0
	// 5: NBSP apenas (não é vazio ASCII) -> 0
	if _, err := conn.Exec(`
		INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, revisado)
		VALUES (1, 1, 1, 'H1', NULL, 0),
		       (2, 1, 2, 'H2', '   '||char(9)||char(10), 0),
		       (3, 1, 3, 'H3', 'Letra Real', 0),
		       (4, 1, 4, 'H4', NULL, 1),
		       (5, 1, 5, 'H5', char(160), 0)
	`); err != nil {
		t.Fatalf("inserir hinos: %v", err)
	}

	query := fmt.Sprintf("SELECT id FROM hino h WHERE %s ORDER BY id", guarda)
	rows, err := conn.Query(query)
	if err != nil {
		t.Fatalf("executar query com guarda: %v", err)
	}
	defer rows.Close()

	var elegiveis []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan id: %v", err)
		}
		elegiveis = append(elegiveis, id)
	}
	if len(elegiveis) != 2 || elegiveis[0] != 1 || elegiveis[1] != 2 {
		t.Errorf("elegíveis inesperados com guarda SQL: %v, esperado [1, 2]", elegiveis)
	}
}

