package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/adjoli/louvores-go/internal/models"
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

func TestHinoRepositoryImportadoFlag(t *testing.T) {
	db := newTestDB(t)
	coletaneaRepo := NewSQLiteColetaneaRepository(db)
	hinoRepo := NewSQLiteHinoRepository(db)
	ctx := context.Background()

	coletanea := testColetanea()
	if err := coletaneaRepo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(Coletanea): %v", err)
	}

	criar := func(num int, tit string) *models.Hino {
		h := testHino(coletanea.ID)
		h.Numeracao = intPtr(num)
		h.Titulo = tit
		if err := hinoRepo.Create(ctx, &h); err != nil {
			t.Fatalf("Create(%s): %v", tit, err)
		}
		return &h
	}

	h1 := criar(1, "Sem Import")
	h2 := criar(2, "Import Ativo")
	h3 := criar(3, "Import Revertido")
	h4 := criar(4, "Revertido e Ativo")
	h5 := criar(5, "Dois Ativos")

	if _, err := db.Exec(`
		INSERT INTO import_batch (batch_id, manifest_sha256) VALUES ('batch1', 'sha1');
		INSERT INTO import_batch (batch_id, manifest_sha256) VALUES ('batch2', 'sha2');
	`); err != nil {
		t.Fatalf("criar batches: %v", err)
	}

	// h2: batch1 ativo
	if _, err := db.Exec(`INSERT INTO import_log (batch_id, manifest_sha256, hino_id, codigo, numero, titulo_destino, letra_nova, creditos_alterados) VALUES ('batch1', 'sha1', ?, 'CC', 2, 'Import Ativo', 'Letra', 0)`, h2.ID); err != nil {
		t.Fatalf("inserir log h2: %v", err)
	}

	// h3: batch1 revertido
	if _, err := db.Exec(`INSERT INTO import_log (batch_id, manifest_sha256, hino_id, codigo, numero, titulo_destino, letra_nova, creditos_alterados, revertido_em) VALUES ('batch1', 'sha1', ?, 'CC', 3, 'Import Revertido', 'Letra', 0, '2026-10-18 00:00:00')`, h3.ID); err != nil {
		t.Fatalf("inserir log h3: %v", err)
	}

	// h4: batch1 revertido, batch2 ativo
	if _, err := db.Exec(`INSERT INTO import_log (batch_id, manifest_sha256, hino_id, codigo, numero, titulo_destino, letra_nova, creditos_alterados, revertido_em) VALUES ('batch1', 'sha1', ?, 'CC', 4, 'Revertido e Ativo', 'Letra', 0, '2026-10-18 00:00:00')`, h4.ID); err != nil {
		t.Fatalf("inserir log h4 revertido: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO import_log (batch_id, manifest_sha256, hino_id, codigo, numero, titulo_destino, letra_nova, creditos_alterados) VALUES ('batch2', 'sha2', ?, 'CC', 4, 'Revertido e Ativo', 'Letra nova 2', 0)`, h4.ID); err != nil {
		t.Fatalf("inserir log h4 ativo: %v", err)
	}

	// h5: batch1 ativo, batch2 ativo (múltiplos batches sem reversão)
	if _, err := db.Exec(`INSERT INTO import_log (batch_id, manifest_sha256, hino_id, codigo, numero, titulo_destino, letra_nova, creditos_alterados) VALUES ('batch1', 'sha1', ?, 'CC', 5, 'Dois Ativos', 'Letra v1', 0)`, h5.ID); err != nil {
		t.Fatalf("inserir log h5 batch1: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO import_log (batch_id, manifest_sha256, hino_id, codigo, numero, titulo_destino, letra_nova, creditos_alterados) VALUES ('batch2', 'sha2', ?, 'CC', 5, 'Dois Ativos', 'Letra v2', 0)`, h5.ID); err != nil {
		t.Fatalf("inserir log h5 batch2: %v", err)
	}

	// Teste FindByID
	testCases := []struct {
		id       int64
		num      int
		esperado bool
		desc     string
	}{
		{h1.ID, 1, false, "h1 (sem import)"},
		{h2.ID, 2, true, "h2 (import ativo)"},
		{h3.ID, 3, false, "h3 (import revertido)"},
		{h4.ID, 4, true, "h4 (revertido + ativo)"},
		{h5.ID, 5, true, "h5 (dois ativos)"},
	}

	for _, tc := range testCases {
		res, err := hinoRepo.FindByID(ctx, tc.id)
		if err != nil {
			t.Fatalf("FindByID(%s): %v", tc.desc, err)
		}
		if res.Importado != tc.esperado {
			t.Errorf("FindByID(%s): Importado = %v, esperado %v", tc.desc, res.Importado, tc.esperado)
		}

		resNum, err := hinoRepo.FindByNumero(ctx, coletanea.ID, tc.num)
		if err != nil {
			t.Fatalf("FindByNumero(%s): %v", tc.desc, err)
		}
		if resNum.Importado != tc.esperado {
			t.Errorf("FindByNumero(%s): Importado = %v, esperado %v", tc.desc, resNum.Importado, tc.esperado)
		}
	}

	// Teste List: deve retornar exatamente 5 hinos, sem duplicação do h5
	todos, err := hinoRepo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(todos) != 5 {
		t.Fatalf("List retornou %d hinos, esperado 5 (sem duplicações)", len(todos))
	}
	for i, tc := range testCases {
		if todos[i].Importado != tc.esperado {
			t.Errorf("List[%d] (%s): Importado = %v, esperado %v", i, tc.desc, todos[i].Importado, tc.esperado)
		}
	}

	// Teste ListByColetanea: deve retornar exatamente 5 hinos
	daColetanea, err := hinoRepo.ListByColetanea(ctx, coletanea.ID)
	if err != nil {
		t.Fatalf("ListByColetanea: %v", err)
	}
	if len(daColetanea) != 5 {
		t.Fatalf("ListByColetanea retornou %d hinos, esperado 5", len(daColetanea))
	}
	for i, tc := range testCases {
		if daColetanea[i].Importado != tc.esperado {
			t.Errorf("ListByColetanea[%d] (%s): Importado = %v, esperado %v", i, tc.desc, daColetanea[i].Importado, tc.esperado)
		}
	}

	// Teste Update: não deve alterar import_log nem falhar por causa do campo Importado
	h2Carregado, _ := hinoRepo.FindByID(ctx, h2.ID)
	h2Carregado.Titulo = "Título Atualizado"
	if err := hinoRepo.Update(ctx, h2Carregado); err != nil {
		t.Fatalf("Update: %v", err)
	}
	h2AposUpdate, _ := hinoRepo.FindByID(ctx, h2.ID)
	if !h2AposUpdate.Importado {
		t.Errorf("após Update, h2.Importado deveria continuar true")
	}
}

func TestHinoRepositoryStatsPorColetaneaWhitespace(t *testing.T) {
	db := newTestDB(t)
	coletaneaRepo := NewSQLiteColetaneaRepository(db)
	hinoRepo := NewSQLiteHinoRepository(db)
	ctx := context.Background()

	coletanea := testColetanea()
	if err := coletaneaRepo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(Coletanea): %v", err)
	}

	criar := func(num int, tit string, letra *string) {
		h := testHino(coletanea.ID)
		h.Numeracao = intPtr(num)
		h.Titulo = tit
		h.Letra = letra
		h.Revisado = false
		if err := hinoRepo.Create(ctx, &h); err != nil {
			t.Fatalf("Create(%s): %v", tit, err)
		}
	}

	vazio := ""
	espacos := "   \t\r\n\v\f "
	texto := "Vinde cantai e louvai"

	criar(1, "Nil", nil)
	criar(2, "Vazio", &vazio)
	criar(3, "Espacos", &espacos)
	criar(4, "Texto", &texto)

	rows, err := hinoRepo.StatsPorColetanea(ctx)
	if err != nil {
		t.Fatalf("StatsPorColetanea: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, esperado 1", len(rows))
	}

	row := rows[0]
	if row.Total != 4 {
		t.Errorf("Total = %d, esperado 4", row.Total)
	}
	if row.ComLetra != 1 {
		t.Errorf("ComLetra = %d, esperado 1 (apenas texto real conta)", row.ComLetra)
	}
}

func TestHinoRepositoryStatsRevisadosExigemLetra(t *testing.T) {
	coletaneaRepo, hinoRepo := newTestRepositories(t)
	ctx := context.Background()
	coletanea := testColetanea()
	if err := coletaneaRepo.Create(ctx, &coletanea); err != nil {
		t.Fatalf("Create(Coletanea): %v", err)
	}

	vazio := ""
	espacosASCII := " \t\n\v\f\r "
	nbsp := "\u00a0"
	texto := "Vinde Cantai"
	for i, letra := range []*string{nil, &vazio, &espacosASCII, &nbsp, &texto} {
		for j, revisado := range []bool{true, false} {
			hino := testHino(coletanea.ID)
			hino.Numeracao = intPtr(2*i + j + 1)
			hino.Revisado = revisado
			if err := hinoRepo.Create(ctx, &hino); err != nil {
				t.Fatalf("Create(Hino %d): %v", *hino.Numeracao, err)
			}

			// Reproduz a edição que limpa a letra sem desfazer a revisão.
			hino.Letra = letra
			if err := hinoRepo.Update(ctx, &hino); err != nil {
				t.Fatalf("Update(Hino %d): %v", *hino.Numeracao, err)
			}
			persistido, err := hinoRepo.FindByID(ctx, hino.ID)
			if err != nil {
				t.Fatalf("FindByID(%d): %v", hino.ID, err)
			}
			if persistido.Revisado != revisado {
				t.Errorf("Hino %d: Revisado = %v, esperado %v", hino.ID, persistido.Revisado, revisado)
			}
			if (persistido.Letra == nil) != (letra == nil) || (letra != nil && *persistido.Letra != *letra) {
				t.Errorf("Hino %d: letra persistida divergente", hino.ID)
			}
		}
	}

	rows, err := hinoRepo.StatsPorColetanea(ctx)
	if err != nil {
		t.Fatalf("StatsPorColetanea: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, esperado 1", len(rows))
	}
	row := rows[0]
	// NBSP não é whitespace ASCII: tanto NBSP quanto texto contam como letra.
	if row.Total != 10 || row.ComLetra != 4 || row.Revisados != 2 {
		t.Errorf("row = %+v, esperado Total=10, ComLetra=4, Revisados=2", row)
	}
	if row.Total-row.ComLetra < 0 || row.ComLetra-row.Revisados < 0 {
		t.Errorf("totais derivados negativos: %+v", row)
	}
	if row.ComLetra > 0 && float64(row.Revisados)/float64(row.ComLetra)*100 > 100 {
		t.Errorf("percentual de revisados acima de 100%%: %+v", row)
	}
}
