package etl_test

import (
	"testing"

	"github.com/adjoli/louvores-go/internal/etl"
)

func guardManifest() *etl.Manifesto {
	return &etl.Manifesto{
		Meta:  etl.MetaManifesto{Versao: 1, BatchID: "guard-test"},
		Itens: []etl.ItemManifesto{{HinoID: 1, ColetaneaID: 1, Codigo: "CC", Numero: 1, TituloEsperado: "Hino", Letra: "Ó Vem"}},
	}
}

func TestSQLRollbackPreservaIdentidade(t *testing.T) {
	for name, change := range map[string]string{
		"titulo":       "UPDATE hino SET titulo = 'Título Humano' WHERE id = 1",
		"numero":       "UPDATE hino SET numeracao = 99 WHERE id = 1",
		"codigo":       "UPDATE coletanea SET codigo = 'HL' WHERE id = 1",
		"coletanea_id": "INSERT INTO coletanea VALUES (2, 'CC', 'Outra'); UPDATE hino SET coletanea_id = 2 WHERE id = 1",
	} {
		t.Run(name, func(t *testing.T) {
			db := setupTestDB(t)
			if _, err := db.Exec("INSERT INTO coletanea VALUES (1, 'CC', 'Cantor'); INSERT INTO hino VALUES (1, 1, 1, 'Hino', NULL, NULL, 0)"); err != nil {
				t.Fatal(err)
			}
			art, err := etl.GerarSQL(guardManifest(), dummySha256)
			if err != nil {
				t.Fatal(err)
			}
			if err := execScript(t, db, art.Import); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(change); err != nil {
				t.Fatal(err)
			}
			var id, num int
			var code, title, status, detail string
			if err := db.QueryRow(art.Verify).Scan(&id, &code, &num, &title, &status, &detail); err != nil {
				t.Fatal(err)
			}
			if status != "conflito" {
				t.Errorf("verify = %q, esperado conflito", status)
			}
			if err := execScript(t, db, art.Rollback); err != nil {
				t.Fatal(err)
			}
			var lyric string
			var reverted int
			if err := db.QueryRow("SELECT letra FROM hino WHERE id = 1").Scan(&lyric); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow("SELECT count(*) FROM import_log WHERE revertido_em IS NOT NULL").Scan(&reverted); err != nil {
				t.Fatal(err)
			}
			if lyric != "Ó Vem" || reverted != 0 {
				t.Fatalf("rollback alterou identidade divergente: letra=%q revertidos=%d", lyric, reverted)
			}
		})
	}
}

func TestSQLRollbackRecusaOutroManifesto(t *testing.T) {
	db := setupTestDB(t)
	if _, err := db.Exec("INSERT INTO coletanea VALUES (1, 'CC', 'Cantor'); INSERT INTO hino VALUES (1, 1, 1, 'Hino', NULL, NULL, 0)"); err != nil {
		t.Fatal(err)
	}
	art, err := etl.GerarSQL(guardManifest(), dummySha256)
	if err != nil {
		t.Fatal(err)
	}
	if err := execScript(t, db, art.Import); err != nil {
		t.Fatal(err)
	}
	wrong, err := etl.GerarSQL(guardManifest(), dummySha256B)
	if err != nil {
		t.Fatal(err)
	}
	_ = execScript(t, db, wrong.Rollback)
	var lyric string
	var reverted int
	if err := db.QueryRow("SELECT letra FROM hino WHERE id = 1").Scan(&lyric); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM import_log WHERE revertido_em IS NOT NULL").Scan(&reverted); err != nil {
		t.Fatal(err)
	}
	if lyric != "Ó Vem" || reverted != 0 {
		t.Fatalf("outro manifesto reverteu batch: letra=%q revertidos=%d", lyric, reverted)
	}
	var id, num int
	var code, title, status, detail string
	if err := db.QueryRow(wrong.Verify).Scan(&id, &code, &num, &title, &status, &detail); err != nil {
		t.Fatal(err)
	}
	if status != "conflito" {
		t.Fatalf("hash divergente reportado como %q", status)
	}
}

func TestSQLImportPreservaAlteracaoAposLog(t *testing.T) {
	db := setupTestDB(t)
	if _, err := db.Exec(`INSERT INTO coletanea VALUES (1, 'CC', 'Cantor');
		INSERT INTO hino VALUES (1, 1, 1, 'Hino', NULL, NULL, 0);
		CREATE TRIGGER nota_viva AFTER INSERT ON import_log BEGIN
			UPDATE hino SET letra = 'Texto Vivo', creditos = 'Autor Vivo' WHERE id = NEW.hino_id;
		END;`); err != nil {
		t.Fatal(err)
	}
	art, err := etl.GerarSQL(guardManifest(), dummySha256)
	if err != nil {
		t.Fatal(err)
	}
	if err := execScript(t, db, art.Import); err == nil {
		t.Fatal("mudança entre log e UPDATE deveria abortar o import")
	}
	var lyric, credit *string
	if err := db.QueryRow("SELECT letra, creditos FROM hino WHERE id = 1").Scan(&lyric, &credit); err != nil {
		t.Fatal(err)
	}
	var logs int
	if err := db.QueryRow("SELECT count(*) FROM import_log").Scan(&logs); err != nil {
		t.Fatal(err)
	}
	if lyric != nil || credit != nil || logs != 0 {
		t.Fatalf("transação incompleta: letra=%v creditos=%v logs=%d", lyric, credit, logs)
	}
}

func TestSQLRecusaControlesXML(t *testing.T) {
	for _, value := range []string{"Ó Vem\x01", "Ó Vem\v", "Ó Vem\f", "Ó Vem\ufffe", "Ó Vem\uffff"} {
		t.Run(value, func(t *testing.T) {
			m := guardManifest()
			m.Itens[0].Letra = value
			if _, err := etl.GerarSQL(m, dummySha256); err == nil {
				t.Fatalf("controle XML aceito: %q", value)
			}
		})
	}
}
