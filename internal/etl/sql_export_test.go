package etl_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/etl"
)

// splitStatements separa instruções SQL respeitando literais de string com aspas simples.
func splitStatements(sqlText string) []string {
	var statements []string
	var current strings.Builder
	inString := false
	runes := []rune(sqlText)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if inString {
			current.WriteRune(r)
			if r == '\'' {
				if i+1 < len(runes) && runes[i+1] == '\'' {
					current.WriteRune('\'')
					i++
				} else {
					inString = false
				}
			}
		} else {
			if r == '\'' {
				inString = true
				current.WriteRune(r)
			} else if r == ';' {
				current.WriteRune(';')
				stmt := strings.TrimSpace(current.String())
				if stmt != "" && stmt != ";" {
					statements = append(statements, stmt)
				}
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		}
	}
	stmt := strings.TrimSpace(current.String())
	if stmt != "" && stmt != ";" {
		statements = append(statements, stmt)
	}
	return statements
}

// execScript executa instruções SQL sequencialmente; no primeiro erro, aborta e executa ROLLBACK.
func execScript(t *testing.T, db *sql.DB, script string) error {
	t.Helper()
	stmts := splitStatements(script)
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			_, _ = db.Exec("ROLLBACK;")
			return err
		}
	}
	return nil
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco de teste: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrar banco de teste: %v", err)
	}
	return db
}

const dummySha256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const dummySha256B = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

func strPtr(s string) *string {
	return &s
}

func TestSQLRoundTrip(t *testing.T) {
	db := setupTestDB(t)

	// Insere coletânea e 2 hinos elegíveis (sem letra, não revisados)
	if _, err := db.Exec(`INSERT INTO coletanea (id, codigo, titulo) VALUES (1, 'CC', 'Cantor Cristão')`); err != nil {
		t.Fatalf("inserir coletanea: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado)
		VALUES (1, 1, 1, 'Minh''Alma', NULL, NULL, 0),
		       (2, 1, 2, '“Com Amor”', NULL, '', 0)
	`); err != nil {
		t.Fatalf("inserir hinos: %v", err)
	}

	letra1 := "Minh'alma louva ao Senhor\n    Bendize o Seu Nome\n"
	letra2 := "    Ó Vem\nEle “te chama” com amor\n"

	manifest := &etl.Manifesto{
		Meta: etl.MetaManifesto{
			Versao:           1,
			BatchID:          "batch-roundtrip-1",
			SnapshotSHA256:   dummySha256,
			CSVSHA256:        dummySha256,
			ProvenanceSHA256: dummySha256,
			TemplateSHA256:   dummySha256,
		},
		Itens: []etl.ItemManifesto{
			{
				HinoID:         1,
				ColetaneaID:    1,
				Codigo:         "CC",
				Numero:         1,
				TituloEsperado: "Minh'Alma",
				Letra:          letra1,
				Creditos:       nil, // Proposta nil com DB NULL -> preserva NULL
			},
			{
				HinoID:         2,
				ColetaneaID:    1,
				Codigo:         "CC",
				Numero:         2,
				TituloEsperado: "“Com Amor”",
				Letra:          letra2,
				Creditos:       strPtr(""), // Proposta vazia com DB vazio -> preserva vazio exato
			},
		},
	}

	artefatos, err := etl.GerarSQL(manifest, dummySha256)
	if err != nil {
		t.Fatalf("GerarSQL falhou: %v", err)
	}

	// Começa com foreign_keys desligado para testar que o script religa
	if _, err := db.Exec("PRAGMA foreign_keys = OFF;"); err != nil {
		t.Fatalf("desligar FK: %v", err)
	}

	// 1. Aplica import.sql
	if err := execScript(t, db, artefatos.Import); err != nil {
		t.Fatalf("executar import.sql: %v", err)
	}

	// Conferir round-trip byte a byte após importação
	var l1, l2 string
	var c1, c2 *string
	row1 := db.QueryRow("SELECT letra, creditos FROM hino WHERE id = 1")
	if err := row1.Scan(&l1, &c1); err != nil {
		t.Fatalf("ler hino 1 pós-import: %v", err)
	}
	if l1 != letra1 {
		t.Errorf("hino 1 letra divergente: esperado %q, obtido %q", letra1, l1)
	}
	if c1 != nil {
		t.Errorf("hino 1 creditos esperado nil, obtido %q", *c1)
	}

	row2 := db.QueryRow("SELECT letra, creditos FROM hino WHERE id = 2")
	if err := row2.Scan(&l2, &c2); err != nil {
		t.Fatalf("ler hino 2 pós-import: %v", err)
	}
	if l2 != letra2 {
		t.Errorf("hino 2 letra divergente: esperado %q, obtido %q", letra2, l2)
	}
	if c2 == nil || *c2 != "" {
		t.Errorf("hino 2 creditos esperado string vazia exata, obtido %v", c2)
	}

	// 2. Aplica rollback.sql
	if err := execScript(t, db, artefatos.Rollback); err != nil {
		t.Fatalf("executar rollback.sql: %v", err)
	}

	// Conferir que voltou ao estado original byte a byte
	var l1After, l2After, c1After, c2After *string
	if err := db.QueryRow("SELECT letra, creditos FROM hino WHERE id = 1").Scan(&l1After, &c1After); err != nil {
		t.Fatalf("ler hino 1 pós-rollback: %v", err)
	}
	if l1After != nil {
		t.Errorf("hino 1 letra esperado nil pós-rollback, obtido %q", *l1After)
	}
	if c1After != nil {
		t.Errorf("hino 1 creditos esperado nil pós-rollback, obtido %q", *c1After)
	}

	if err := db.QueryRow("SELECT letra, creditos FROM hino WHERE id = 2").Scan(&l2After, &c2After); err != nil {
		t.Fatalf("ler hino 2 pós-rollback: %v", err)
	}
	if l2After != nil {
		t.Errorf("hino 2 letra esperado nil pós-rollback, obtido %q", *l2After)
	}
	if c2After == nil || *c2After != "" {
		t.Errorf("hino 2 creditos esperado vazio exato pós-rollback, obtido %v", c2After)
	}

	// Conferir que log foi marcado revertido_em
	var revertidos int
	if err := db.QueryRow("SELECT COUNT(*) FROM import_log WHERE batch_id = 'batch-roundtrip-1' AND revertido_em IS NOT NULL").Scan(&revertidos); err != nil {
		t.Fatalf("consultar logs revertidos: %v", err)
	}
	if revertidos != 2 {
		t.Errorf("esperado 2 logs revertidos, obtido %d", revertidos)
	}
}

func TestSQLConcorrencia(t *testing.T) {
	db := setupTestDB(t)

	if _, err := db.Exec(`INSERT INTO coletanea (id, codigo, titulo) VALUES (1, 'CC', 'Cantor Cristão')`); err != nil {
		t.Fatalf("inserir coletanea: %v", err)
	}
	// 4 hinos para testar concorrência:
	// Hino 1: crédito preenchido entre snapshot e apply
	// Hino 2: letra preenchida entre snapshot e apply
	// Hino 3: título alterado entre snapshot e apply
	// Hino 4: marcado revisado entre snapshot e apply
	if _, err := db.Exec(`
		INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado)
		VALUES (1, 1, 1, 'Hino 1', NULL, 'Autor Humano Vivo', 0),
		       (2, 1, 2, 'Hino 2', 'Letra Humana Prévia', NULL, 0),
		       (3, 1, 3, 'Título Antigo Alterado', NULL, NULL, 0),
		       (4, 1, 4, 'Hino 4', NULL, NULL, 1)
	`); err != nil {
		t.Fatalf("inserir hinos: %v", err)
	}

	manifest := &etl.Manifesto{
		Meta: etl.MetaManifesto{
			Versao:           1,
			BatchID:          "batch-concorrencia",
			SnapshotSHA256:   dummySha256,
			CSVSHA256:        dummySha256,
			ProvenanceSHA256: dummySha256,
			TemplateSHA256:   dummySha256,
		},
		Itens: []etl.ItemManifesto{
			{
				HinoID:         1,
				ColetaneaID:    1,
				Codigo:         "CC",
				Numero:         1,
				TituloEsperado: "Hino 1",
				Letra:          "Letra Importada 1",
				Creditos:       strPtr("Autor Proposto Descartavel"),
			},
			{
				HinoID:         2,
				ColetaneaID:    1,
				Codigo:         "CC",
				Numero:         2,
				TituloEsperado: "Hino 2",
				Letra:          "Letra Importada 2",
				Creditos:       nil,
			},
			{
				HinoID:         3,
				ColetaneaID:    1,
				Codigo:         "CC",
				Numero:         3,
				TituloEsperado: "Título Que Não Bate",
				Letra:          "Letra Importada 3",
				Creditos:       nil,
			},
			{
				HinoID:         4,
				ColetaneaID:    1,
				Codigo:         "CC",
				Numero:         4,
				TituloEsperado: "Hino 4",
				Letra:          "Letra Importada 4",
				Creditos:       nil,
			},
		},
	}

	artefatos, err := etl.GerarSQL(manifest, dummySha256)
	if err != nil {
		t.Fatalf("GerarSQL: %v", err)
	}

	if err := execScript(t, db, artefatos.Import); err != nil {
		t.Fatalf("executar import.sql: %v", err)
	}

	// Hino 1: deve importar a letra, MAS preservar o crédito vivo 'Autor Humano Vivo'
	var l1, c1 string
	if err := db.QueryRow("SELECT letra, creditos FROM hino WHERE id = 1").Scan(&l1, &c1); err != nil {
		t.Fatalf("ler hino 1: %v", err)
	}
	if l1 != "Letra Importada 1" {
		t.Errorf("hino 1 letra esperada %q, obtida %q", "Letra Importada 1", l1)
	}
	if c1 != "Autor Humano Vivo" {
		t.Errorf("hino 1 crédito vivo DEVE ser preservado, esperado %q, obtido %q", "Autor Humano Vivo", c1)
	}

	// Verificar log do Hino 1: creditos_novos deve ser o crédito vivo e creditos_alterados = 0
	var credAnt, credNov string
	var credAlt int
	if err := db.QueryRow("SELECT creditos_anterior, creditos_novos, creditos_alterados FROM import_log WHERE batch_id = 'batch-concorrencia' AND hino_id = 1").Scan(&credAnt, &credNov, &credAlt); err != nil {
		t.Fatalf("ler log hino 1: %v", err)
	}
	if credAnt != "Autor Humano Vivo" || credNov != "Autor Humano Vivo" || credAlt != 0 {
		t.Errorf("log hino 1 incorreto: ant=%q, nov=%q, alt=%d", credAnt, credNov, credAlt)
	}

	// Hinos 2, 3, 4: NÃO devem ter sido modificados e NÃO devem ter gerado log de importação
	for _, id := range []int{2, 3, 4} {
		var cnt int
		if err := db.QueryRow("SELECT COUNT(*) FROM import_log WHERE batch_id = 'batch-concorrencia' AND hino_id = ?", id).Scan(&cnt); err != nil {
			t.Fatalf("contar log hino %d: %v", id, err)
		}
		if cnt != 0 {
			t.Errorf("hino %d NÃO deveria ter gerado log de sucesso", id)
		}
	}

	var l2 string
	_ = db.QueryRow("SELECT letra FROM hino WHERE id = 2").Scan(&l2)
	if l2 != "Letra Humana Prévia" {
		t.Errorf("hino 2 letra não deveria ter sido sobrescrita: %q", l2)
	}

	var rev4 int
	_ = db.QueryRow("SELECT revisado FROM hino WHERE id = 4").Scan(&rev4)
	if rev4 != 1 {
		t.Errorf("hino 4 revisão foi alterada")
	}
}

func TestSQLRollbackHumano(t *testing.T) {
	db := setupTestDB(t)

	if _, err := db.Exec(`INSERT INTO coletanea (id, codigo, titulo) VALUES (1, 'CC', 'Cantor Cristão')`); err != nil {
		t.Fatalf("inserir coletanea: %v", err)
	}
	// 5 hinos elegíveis
	if _, err := db.Exec(`
		INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado)
		VALUES (1, 1, 1, 'Hino 1', NULL, NULL, 0),
		       (2, 1, 2, 'Hino 2', NULL, NULL, 0),
		       (3, 1, 3, 'Hino 3', NULL, NULL, 0),
		       (4, 1, 4, 'Hino 4', NULL, NULL, 0),
		       (5, 1, 5, 'Hino 5', NULL, 'Credito Original', 0)
	`); err != nil {
		t.Fatalf("inserir hinos: %v", err)
	}

	manifest := &etl.Manifesto{
		Meta: etl.MetaManifesto{
			Versao:           1,
			BatchID:          "batch-rollback-humano",
			SnapshotSHA256:   dummySha256,
			CSVSHA256:        dummySha256,
			ProvenanceSHA256: dummySha256,
			TemplateSHA256:   dummySha256,
		},
		Itens: []etl.ItemManifesto{
			{HinoID: 1, ColetaneaID: 1, Codigo: "CC", Numero: 1, TituloEsperado: "Hino 1", Letra: "Letra Imp 1", Creditos: strPtr("Autor 1")},
			{HinoID: 2, ColetaneaID: 1, Codigo: "CC", Numero: 2, TituloEsperado: "Hino 2", Letra: "Letra Imp 2", Creditos: strPtr("Autor 2")},
			{HinoID: 3, ColetaneaID: 1, Codigo: "CC", Numero: 3, TituloEsperado: "Hino 3", Letra: "Letra Imp 3", Creditos: strPtr("Autor 3")},
			{HinoID: 4, ColetaneaID: 1, Codigo: "CC", Numero: 4, TituloEsperado: "Hino 4", Letra: "Letra Imp 4", Creditos: strPtr("Autor 4")},
			{HinoID: 5, ColetaneaID: 1, Codigo: "CC", Numero: 5, TituloEsperado: "Hino 5", Letra: "Letra Imp 5", Creditos: strPtr("Ignorado")},
		},
	}

	artefatos, err := etl.GerarSQL(manifest, dummySha256)
	if err != nil {
		t.Fatalf("GerarSQL: %v", err)
	}

	// 1. Aplica import
	if err := execScript(t, db, artefatos.Import); err != nil {
		t.Fatalf("executar import.sql: %v", err)
	}

	// Simula intervenções humanas pós-import:
	// Hino 1: Adão edita a letra sem marcar revisão
	if _, err := db.Exec("UPDATE hino SET letra = 'Letra Editada Pelo Adão' WHERE id = 1"); err != nil {
		t.Fatalf("update hino 1: %v", err)
	}
	// Hino 2: Adão edita apenas o crédito
	if _, err := db.Exec("UPDATE hino SET creditos = 'Crédito Ajustado Manual' WHERE id = 2"); err != nil {
		t.Fatalf("update hino 2: %v", err)
	}
	// Hino 3: Adão revisa o hino
	if _, err := db.Exec("UPDATE hino SET revisado = 1 WHERE id = 3"); err != nil {
		t.Fatalf("update hino 3: %v", err)
	}
	// Hino 4: Adão apaga a letra
	if _, err := db.Exec("UPDATE hino SET letra = NULL WHERE id = 4"); err != nil {
		t.Fatalf("update hino 4: %v", err)
	}
	// Hino 5: Não sofreu intervenção humana (crédito já estava preenchido antes do import, não deve virar falso conflito)

	// 2. Aplica rollback.sql
	if err := execScript(t, db, artefatos.Rollback); err != nil {
		t.Fatalf("executar rollback.sql: %v", err)
	}

	// Verificações:
	// Hino 1: conflito -> letra humana intacta, log NÃO revertido
	var l1 string
	_ = db.QueryRow("SELECT letra FROM hino WHERE id = 1").Scan(&l1)
	if l1 != "Letra Editada Pelo Adão" {
		t.Errorf("hino 1 trabalho humano foi perdido: %q", l1)
	}
	var rev1 *string
	_ = db.QueryRow("SELECT revertido_em FROM import_log WHERE batch_id = 'batch-rollback-humano' AND hino_id = 1").Scan(&rev1)
	if rev1 != nil {
		t.Errorf("hino 1 log NÃO deveria ter sido revertido")
	}

	// Hino 2: conflito de crédito -> crédito humano intacto, log NÃO revertido
	var c2 string
	_ = db.QueryRow("SELECT creditos FROM hino WHERE id = 2").Scan(&c2)
	if c2 != "Crédito Ajustado Manual" {
		t.Errorf("hino 2 crédito humano foi perdido: %q", c2)
	}
	var rev2 *string
	_ = db.QueryRow("SELECT revertido_em FROM import_log WHERE batch_id = 'batch-rollback-humano' AND hino_id = 2").Scan(&rev2)
	if rev2 != nil {
		t.Errorf("hino 2 log NÃO deveria ter sido revertido")
	}

	// Hino 3: conflito de revisão -> log NÃO revertido
	var rev3 *string
	_ = db.QueryRow("SELECT revertido_em FROM import_log WHERE batch_id = 'batch-rollback-humano' AND hino_id = 3").Scan(&rev3)
	if rev3 != nil {
		t.Errorf("hino 3 log NÃO deveria ter sido revertido")
	}

	// Hino 4: apagou letra -> log NÃO recebe revertido_em
	var rev4 *string
	_ = db.QueryRow("SELECT revertido_em FROM import_log WHERE batch_id = 'batch-rollback-humano' AND hino_id = 4").Scan(&rev4)
	if rev4 != nil {
		t.Errorf("hino 4 log apagado NÃO recebe revertido_em")
	}

	// Hino 5: reverteu com sucesso! (sem falso conflito de crédito preexistente)
	var l5 *string
	var c5 string
	_ = db.QueryRow("SELECT letra, creditos FROM hino WHERE id = 5").Scan(&l5, &c5)
	if l5 != nil {
		t.Errorf("hino 5 deveria ter voltado para letra NULL, obtido %v", l5)
	}
	if c5 != "Credito Original" {
		t.Errorf("hino 5 crédito preexistente deve permanecer intacto: %q", c5)
	}
	var rev5 *string
	_ = db.QueryRow("SELECT revertido_em FROM import_log WHERE batch_id = 'batch-rollback-humano' AND hino_id = 5").Scan(&rev5)
	if rev5 == nil {
		t.Errorf("hino 5 log DEVERIA ter sido revertido")
	}
}

func TestSQLReplay(t *testing.T) {
	db := setupTestDB(t)

	if _, err := db.Exec(`INSERT INTO coletanea (id, codigo, titulo) VALUES (1, 'CC', 'Cantor Cristão')`); err != nil {
		t.Fatalf("inserir coletanea: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado)
		VALUES (1, 1, 1, 'Hino 1', NULL, NULL, 0)
	`); err != nil {
		t.Fatalf("inserir hino: %v", err)
	}

	manifest := &etl.Manifesto{
		Meta: etl.MetaManifesto{
			Versao:           1,
			BatchID:          "batch-replay",
			SnapshotSHA256:   dummySha256,
			CSVSHA256:        dummySha256,
			ProvenanceSHA256: dummySha256,
			TemplateSHA256:   dummySha256,
		},
		Itens: []etl.ItemManifesto{
			{HinoID: 1, ColetaneaID: 1, Codigo: "CC", Numero: 1, TituloEsperado: "Hino 1", Letra: "Letra Replay", Creditos: nil},
		},
	}

	artefatos, err := etl.GerarSQL(manifest, dummySha256)
	if err != nil {
		t.Fatalf("GerarSQL: %v", err)
	}

	// 1ª execução de import
	if err := execScript(t, db, artefatos.Import); err != nil {
		t.Fatalf("1º import: %v", err)
	}

	var logs1 int
	_ = db.QueryRow("SELECT COUNT(*) FROM import_log WHERE batch_id = 'batch-replay'").Scan(&logs1)
	if logs1 != 1 {
		t.Fatalf("esperado 1 log na 1ª execução, obtido %d", logs1)
	}

	// 2ª execução de import (replay) -> zero novos eventos/hinos
	if err := execScript(t, db, artefatos.Import); err != nil {
		t.Fatalf("2º import (replay) falhou: %v", err)
	}
	var logs2 int
	_ = db.QueryRow("SELECT COUNT(*) FROM import_log WHERE batch_id = 'batch-replay'").Scan(&logs2)
	if logs2 != 1 {
		t.Errorf("2ª execução gerou eventos duplicados: %d logs", logs2)
	}

	// 1ª execução de rollback
	if err := execScript(t, db, artefatos.Rollback); err != nil {
		t.Fatalf("1º rollback: %v", err)
	}
	var rev1 *string
	_ = db.QueryRow("SELECT revertido_em FROM import_log WHERE batch_id = 'batch-replay' AND hino_id = 1").Scan(&rev1)
	if rev1 == nil {
		t.Fatalf("log deveria estar revertido")
	}

	// 2ª execução de rollback -> no-op
	if err := execScript(t, db, artefatos.Rollback); err != nil {
		t.Fatalf("2º rollback (replay) falhou: %v", err)
	}

	// Import após rollback do mesmo batch NÃO deve ressuscitar
	if err := execScript(t, db, artefatos.Import); err != nil {
		t.Fatalf("import pós-rollback falhou: %v", err)
	}
	var letraPos *string
	_ = db.QueryRow("SELECT letra FROM hino WHERE id = 1").Scan(&letraPos)
	if letraPos != nil {
		t.Errorf("import após rollback do mesmo batch ressuscitou letra: %v", *letraPos)
	}

	// Mesma batch_id com hash diferente aborta por conflito de PK
	artefatosConflito, err := etl.GerarSQL(manifest, dummySha256B)
	if err != nil {
		t.Fatalf("GerarSQL com outro hash: %v", err)
	}
	err = execScript(t, db, artefatosConflito.Import)
	if err == nil {
		t.Fatalf("esperava erro ao tentar reutilizar batch_id com hash diferente")
	}
}

func TestSQLIncompativel(t *testing.T) {
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	defer conn.Close()

	// Cria schema de domínio mas schema incompatível de import_log (faltando CHECK e índice parcial)
	schemaIncompativel := `
	CREATE TABLE coletanea (id INTEGER PRIMARY KEY, codigo TEXT NOT NULL, titulo TEXT NOT NULL);
	CREATE TABLE hino (id INTEGER PRIMARY KEY, coletanea_id INTEGER NOT NULL REFERENCES coletanea(id), numeracao INTEGER, titulo TEXT NOT NULL, letra TEXT, creditos TEXT, revisado INTEGER NOT NULL DEFAULT 0);
	CREATE UNIQUE INDEX idx_hino_coletanea_num_unique_v1 ON hino(coletanea_id, numeracao);

	CREATE TABLE import_batch (
		batch_id TEXT PRIMARY KEY,
		manifest_sha256 TEXT NOT NULL,
		criado_em TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(batch_id, manifest_sha256)
	);

	-- import_log SEM CHECK constraint de creditos_alterados
	CREATE TABLE import_log (
		batch_id TEXT NOT NULL,
		manifest_sha256 TEXT NOT NULL,
		hino_id INTEGER NOT NULL REFERENCES hino(id),
		codigo TEXT NOT NULL,
		numero INTEGER NOT NULL,
		titulo_destino TEXT NOT NULL,
		letra_anterior TEXT,
		creditos_anterior TEXT,
		letra_nova TEXT NOT NULL,
		creditos_novos TEXT,
		creditos_alterados INTEGER NOT NULL, -- sem CHECK!
		aplicado_em TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		revertido_em TEXT,
		PRIMARY KEY(batch_id,hino_id),
		FOREIGN KEY(batch_id,manifest_sha256) REFERENCES import_batch(batch_id,manifest_sha256)
	);
	`
	if _, err := conn.Exec(schemaIncompativel); err != nil {
		t.Fatalf("criar schema incompatível: %v", err)
	}
	if _, err := conn.Exec("INSERT INTO coletanea VALUES (1, 'CC', 'Cantor Cristão'); INSERT INTO hino VALUES (1, 1, 1, 'Hino 1', NULL, NULL, 0);"); err != nil {
		t.Fatalf("dados iniciais: %v", err)
	}

	manifest := &etl.Manifesto{
		Meta: etl.MetaManifesto{
			Versao:           1,
			BatchID:          "batch-incompativel",
			SnapshotSHA256:   dummySha256,
			CSVSHA256:        dummySha256,
			ProvenanceSHA256: dummySha256,
			TemplateSHA256:   dummySha256,
		},
		Itens: []etl.ItemManifesto{
			{HinoID: 1, ColetaneaID: 1, Codigo: "CC", Numero: 1, TituloEsperado: "Hino 1", Letra: "Letra Tentativa", Creditos: nil},
		},
	}

	artefatos, err := etl.GerarSQL(manifest, dummySha256)
	if err != nil {
		t.Fatalf("GerarSQL: %v", err)
	}

	// Executa import.sql -> DEVE falhar na validação do schema antes de tocar o hino
	err = execScript(t, conn, artefatos.Import)
	if err == nil {
		t.Fatalf("esperava erro em schema incompatível")
	}

	// O hino não pode ter sido modificado
	var letra *string
	_ = conn.QueryRow("SELECT letra FROM hino WHERE id = 1").Scan(&letra)
	if letra != nil {
		t.Errorf("hino não deveria ter sido modificado após erro de schema")
	}

	// Rollback com schema incompatível também deve falhar
	err = execScript(t, conn, artefatos.Rollback)
	if err == nil {
		t.Fatalf("esperava erro em rollback com schema incompatível")
	}
}

func TestSQLVerify(t *testing.T) {
	db := setupTestDB(t)

	if _, err := db.Exec(`INSERT INTO coletanea (id, codigo, titulo) VALUES (1, 'CC', 'Cantor Cristão')`); err != nil {
		t.Fatalf("inserir coletanea: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado)
		VALUES (1, 1, 1, 'Hino 1', NULL, NULL, 0),
		       (2, 1, 2, 'Hino 2', NULL, NULL, 0),
		       (3, 1, 3, 'Hino 3', NULL, NULL, 0)
	`); err != nil {
		t.Fatalf("inserir hinos: %v", err)
	}

	manifest := &etl.Manifesto{
		Meta: etl.MetaManifesto{
			Versao:           1,
			BatchID:          "batch-verify",
			SnapshotSHA256:   dummySha256,
			CSVSHA256:        dummySha256,
			ProvenanceSHA256: dummySha256,
			TemplateSHA256:   dummySha256,
		},
		Itens: []etl.ItemManifesto{
			{HinoID: 1, ColetaneaID: 1, Codigo: "CC", Numero: 1, TituloEsperado: "Hino 1", Letra: "L1", Creditos: strPtr("C1")},
			{HinoID: 2, ColetaneaID: 1, Codigo: "CC", Numero: 2, TituloEsperado: "Hino 2", Letra: "L2", Creditos: strPtr("C2")},
			{HinoID: 3, ColetaneaID: 1, Codigo: "CC", Numero: 3, TituloEsperado: "Hino 3", Letra: "L3", Creditos: strPtr("C3")},
			{HinoID: 99, ColetaneaID: 1, Codigo: "CC", Numero: 99, TituloEsperado: "Hino Ausente", Letra: "L99", Creditos: nil},
		},
	}

	artefatos, err := etl.GerarSQL(manifest, dummySha256)
	if err != nil {
		t.Fatalf("GerarSQL: %v", err)
	}

	// 1. Aplica import
	if err := execScript(t, db, artefatos.Import); err != nil {
		t.Fatalf("executar import.sql: %v", err)
	}

	// Simula estados para o verify:
	// Target 1: ativo (permanece sem mexer)
	// Target 2: revertido
	if _, err := db.Exec("UPDATE import_log SET revertido_em = CURRENT_TIMESTAMP WHERE batch_id = 'batch-verify' AND hino_id = 2; UPDATE hino SET letra = NULL, creditos = NULL WHERE id = 2;"); err != nil {
		t.Fatalf("reverter hino 2: %v", err)
	}
	// Target 3: conflito (Adão editou a letra)
	if _, err := db.Exec("UPDATE hino SET letra = 'Letra Humana Conflitante' WHERE id = 3"); err != nil {
		t.Fatalf("editar hino 3: %v", err)
	}
	// Target 99: ausente (nem hino nem log existem)

	// Executa verify.sql
	rows, err := db.Query(artefatos.Verify)
	if err != nil {
		t.Fatalf("executar verify.sql: %v", err)
	}
	defer rows.Close()

	type verifyRow struct {
		hinoID int64
		codigo string
		numero int
		titulo string
		status string
		det    string
	}
	var results []verifyRow
	for rows.Next() {
		var vr verifyRow
		if err := rows.Scan(&vr.hinoID, &vr.codigo, &vr.numero, &vr.titulo, &vr.status, &vr.det); err != nil {
			t.Fatalf("scan verify row: %v", err)
		}
		results = append(results, vr)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}

	// Exatamente 4 linhas (uma por alvo do manifesto, sem sumir linhas)
	if len(results) != 4 {
		t.Fatalf("esperado 4 resultados no verify, obtido %d", len(results))
	}

	statusPorID := make(map[int64]string)
	for _, r := range results {
		statusPorID[r.hinoID] = r.status
	}

	if statusPorID[1] != "ativo" {
		t.Errorf("alvo 1 status esperado 'ativo', obtido %q", statusPorID[1])
	}
	if statusPorID[2] != "revertido" {
		t.Errorf("alvo 2 status esperado 'revertido', obtido %q", statusPorID[2])
	}
	if statusPorID[3] != "conflito" {
		t.Errorf("alvo 3 status esperado 'conflito', obtido %q", statusPorID[3])
	}
	if statusPorID[99] != "ausente" {
		t.Errorf("alvo 99 status esperado 'ausente', obtido %q", statusPorID[99])
	}
}

func TestSQLControle(t *testing.T) {
	// 1. Testa strings com NUL byte
	nulStr := "teste\x00corrupcao"
	manifestNul := &etl.Manifesto{
		Meta: etl.MetaManifesto{
			Versao:           1,
			BatchID:          "batch-ctrl",
			SnapshotSHA256:   dummySha256,
			CSVSHA256:        dummySha256,
			ProvenanceSHA256: dummySha256,
			TemplateSHA256:   dummySha256,
		},
		Itens: []etl.ItemManifesto{
			{HinoID: 1, ColetaneaID: 1, Codigo: "CC", Numero: 1, TituloEsperado: "Hino 1", Letra: nulStr, Creditos: nil},
		},
	}
	if _, err := etl.GerarSQL(manifestNul, dummySha256); err == nil {
		t.Errorf("esperava erro para letra contendo byte NUL")
	}

	// 2. Testa strings com UTF-8 inválido
	invalidUTF8 := "letra \xff\xfe invalida"
	manifestUTF8 := &etl.Manifesto{
		Meta: etl.MetaManifesto{
			Versao:           1,
			BatchID:          "batch-ctrl",
			SnapshotSHA256:   dummySha256,
			CSVSHA256:        dummySha256,
			ProvenanceSHA256: dummySha256,
			TemplateSHA256:   dummySha256,
		},
		Itens: []etl.ItemManifesto{
			{HinoID: 1, ColetaneaID: 1, Codigo: "CC", Numero: 1, TituloEsperado: "Hino 1", Letra: invalidUTF8, Creditos: nil},
		},
	}
	if _, err := etl.GerarSQL(manifestUTF8, dummySha256); err == nil {
		t.Errorf("esperava erro para letra contendo UTF-8 inválido")
	}

	// 3. Testa hash SHA-256 do manifesto inválido
	manifestOk := &etl.Manifesto{
		Meta: etl.MetaManifesto{
			Versao:           1,
			BatchID:          "batch-ctrl",
			SnapshotSHA256:   dummySha256,
			CSVSHA256:        dummySha256,
			ProvenanceSHA256: dummySha256,
			TemplateSHA256:   dummySha256,
		},
		Itens: []etl.ItemManifesto{
			{HinoID: 1, ColetaneaID: 1, Codigo: "CC", Numero: 1, TituloEsperado: "Hino 1", Letra: "Letra ok", Creditos: nil},
		},
	}
	if _, err := etl.GerarSQL(manifestOk, "hash-invalido-curto"); err == nil {
		t.Errorf("esperava erro para manifestSHA256 com tamanho/formato inválido")
	}
}

func TestExportSQLForTurso(t *testing.T) {
	manifestIn := os.Getenv("TURSO_MANIFEST_IN")
	exportOut := os.Getenv("TURSO_EXPORT_OUT")
	if manifestIn == "" || exportOut == "" {
		return
	}
	data, err := os.ReadFile(manifestIn)
	if err != nil {
		t.Fatalf("ler manifest in: %v", err)
	}
	var req struct {
		Manifesto *etl.Manifesto `json:"manifesto"`
		SHA256    string         `json:"sha256"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("unmarshal req: %v", err)
	}
	art, err := etl.GerarSQL(req.Manifesto, req.SHA256)
	if err != nil {
		t.Fatalf("GerarSQL: %v", err)
	}
	outData, err := json.Marshal(art)
	if err != nil {
		t.Fatalf("marshal art: %v", err)
	}
	if err := os.WriteFile(exportOut, outData, 0o644); err != nil {
		t.Fatalf("escrever export out: %v", err)
	}
}

