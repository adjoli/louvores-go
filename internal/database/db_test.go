package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
)

func TestImportSchema(t *testing.T) {
	ctx := context.Background()

	// 1. Fresh e migração dupla idempotente
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	defer conn.Close()

	if err := database.Migrate(conn); err != nil {
		t.Fatalf("1a migração falhou: %v", err)
	}
	if err := database.Migrate(conn); err != nil {
		t.Fatalf("2a migração (idempotente) falhou: %v", err)
	}

	// Validação Go deve passar no banco migrado
	if err := database.ValidarImportSchema(ctx, conn); err != nil {
		t.Fatalf("ValidarImportSchema falhou no banco migrado: %v", err)
	}

	// Validação SQL pura deve passar no banco migrado
	if _, err := conn.ExecContext(ctx, database.ValidacaoImportSchemaSQL()); err != nil {
		t.Fatalf("ValidacaoImportSchemaSQL falhou no banco migrado: %v", err)
	}

	// 2. PK / Unicidade em import_batch: mesmo batch_id com hash diferente falha
	if _, err := conn.Exec(`INSERT INTO import_batch (batch_id, manifest_sha256) VALUES ('b1', 'hash1')`); err != nil {
		t.Fatalf("inserir batch b1/hash1: %v", err)
	}
	// Mesmo batch_id com hash diferente deve violar PK
	if _, err := conn.Exec(`INSERT INTO import_batch (batch_id, manifest_sha256) VALUES ('b1', 'hash2')`); err == nil {
		t.Fatalf("esperava erro ao inserir mesmo batch_id com hash diferente, obteve sucesso")
	}

	// 3. ValidarImportSchema aceita snapshot legado com AMBAS tabelas ausentes
	legacyConn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco legado: %v", err)
	}
	defer legacyConn.Close()

	if _, err := legacyConn.Exec(`
		CREATE TABLE coletanea (id INTEGER PRIMARY KEY, codigo TEXT, titulo TEXT);
		CREATE TABLE hino (id INTEGER PRIMARY KEY, coletanea_id INTEGER, numeracao INTEGER, titulo TEXT, letra TEXT, creditos TEXT, revisado INTEGER);
		CREATE INDEX idx_hino_coletanea_num ON hino (coletanea_id, numeracao);
	`); err != nil {
		t.Fatalf("criar schema legado: %v", err)
	}

	if err := database.ValidarImportSchema(ctx, legacyConn); err != nil {
		t.Fatalf("ValidarImportSchema deveria aceitar snapshot legado com ambas tabelas ausentes: %v", err)
	}

	// Assertion SQL standalone, porém, exige ambas presentes e deve falhar no legado
	if _, err := legacyConn.ExecContext(ctx, database.ValidacaoImportSchemaSQL()); err == nil {
		t.Fatalf("ValidacaoImportSchemaSQL deveria falhar em banco legado sem tabelas de import")
	}

	// 4. Rejeição de schema parcial
	parcialConn1, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco parcial 1: %v", err)
	}
	defer parcialConn1.Close()
	if _, err := parcialConn1.Exec(`CREATE TABLE import_batch (batch_id TEXT PRIMARY KEY, manifest_sha256 TEXT NOT NULL, criado_em TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);`); err != nil {
		t.Fatalf("criar import_batch isolada: %v", err)
	}
	if err := database.ValidarImportSchema(ctx, parcialConn1); err == nil {
		t.Fatalf("ValidarImportSchema deveria rejeitar schema parcial (só import_batch)")
	}

	parcialConn2, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco parcial 2: %v", err)
	}
	defer parcialConn2.Close()
	if _, err := parcialConn2.Exec(`CREATE TABLE import_log (batch_id TEXT, manifest_sha256 TEXT);`); err != nil {
		t.Fatalf("criar import_log isolada: %v", err)
	}
	if err := database.ValidarImportSchema(ctx, parcialConn2); err == nil {
		t.Fatalf("ValidarImportSchema deveria rejeitar schema parcial (só import_log)")
	}

	// 5. Shape incompatível (tabelas com mesmas colunas mas sem PK/FK/CHECK/índice parcial)
	incompatConn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco incompat: %v", err)
	}
	defer incompatConn.Close()

	if _, err := incompatConn.Exec(`
		CREATE TABLE coletanea (id INTEGER, codigo TEXT, titulo TEXT);
		CREATE TABLE hino (id INTEGER, coletanea_id INTEGER, numeracao INTEGER, titulo TEXT, letra TEXT, creditos TEXT, revisado INTEGER);
		CREATE TABLE import_batch (batch_id TEXT, manifest_sha256 TEXT, criado_em TEXT);
		CREATE TABLE import_log (
			batch_id TEXT, manifest_sha256 TEXT, hino_id INTEGER, codigo TEXT, numero INTEGER,
			titulo_destino TEXT, letra_anterior TEXT, creditos_anterior TEXT, letra_nova TEXT,
			creditos_novos TEXT, creditos_alterados INTEGER, aplicado_em TEXT, revertido_em TEXT
		);
	`); err != nil {
		t.Fatalf("criar tabelas incompatíveis: %v", err)
	}

	if err := database.ValidarImportSchema(ctx, incompatConn); err == nil {
		t.Fatalf("ValidarImportSchema deveria rejeitar shape incompatível")
	}
	if _, err := incompatConn.ExecContext(ctx, database.ValidacaoImportSchemaSQL()); err == nil {
		t.Fatalf("ValidacaoImportSchemaSQL deveria falhar em shape incompatível")
	}
}

func TestIndiceUnicoLegado(t *testing.T) {
	// 1. Banco com índice não único e sem duplicatas ganha proteção unique sem apagar índice legado nem dados
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Exec(`
		CREATE TABLE coletanea (id INTEGER PRIMARY KEY AUTOINCREMENT, codigo TEXT NOT NULL, titulo TEXT NOT NULL);
		CREATE TABLE hino (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			coletanea_id INTEGER NOT NULL REFERENCES coletanea(id),
			numeracao INTEGER,
			titulo TEXT NOT NULL,
			letra TEXT,
			creditos TEXT,
			revisado INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX idx_hino_coletanea_num ON hino (coletanea_id, numeracao);
		INSERT INTO coletanea (id, codigo, titulo) VALUES (1, 'CC', 'Cantor Cristão');
		INSERT INTO hino (coletanea_id, numeracao, titulo) VALUES (1, 42, 'Hino 42');
	`); err != nil {
		t.Fatalf("setup legado: %v", err)
	}

	if err := database.Migrate(conn); err != nil {
		t.Fatalf("Migrate falhou no banco legado sem duplicatas: %v", err)
	}

	// Verifica se ambos os índices existem em hino
	rows, err := conn.Query(`PRAGMA index_list('hino')`)
	if err != nil {
		t.Fatalf("consultar index_list: %v", err)
	}
	defer rows.Close()

	temLegado := false
	temNovoUnico := false
	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan index_list: %v", err)
		}
		if name == "idx_hino_coletanea_num" && unique == 0 {
			temLegado = true
		}
		if name == "idx_hino_coletanea_num_unique_v1" && unique == 1 {
			temNovoUnico = true
		}
	}
	if !temLegado {
		t.Fatalf("índice legado idx_hino_coletanea_num foi apagado indevidamente")
	}
	if !temNovoUnico {
		t.Fatalf("novo índice único idx_hino_coletanea_num_unique_v1 não foi criado")
	}

	// Verifica que agora duplicatas são rejeitadas pelo índice único
	if _, err := conn.Exec(`INSERT INTO hino (coletanea_id, numeracao, titulo) VALUES (1, 42, 'Hino 42 Dup')`); err == nil {
		t.Fatalf("esperava erro de unicidade ao inserir duplicata após migração")
	}

	// 2. Banco com nome novo já existente com definição errada bloqueia
	connErr, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	defer connErr.Close()

	if _, err := connErr.Exec(`
		CREATE TABLE coletanea (id INTEGER PRIMARY KEY, codigo TEXT, titulo TEXT);
		CREATE TABLE hino (
			id INTEGER PRIMARY KEY,
			coletanea_id INTEGER NOT NULL REFERENCES coletanea(id),
			numeracao INTEGER,
			titulo TEXT NOT NULL,
			letra TEXT,
			creditos TEXT,
			revisado INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX idx_hino_coletanea_num_unique_v1 ON hino (titulo);
	`); err != nil {
		t.Fatalf("setup def errada: %v", err)
	}

	if err := database.Migrate(connErr); err == nil {
		t.Fatalf("Migrate deveria falhar quando idx_hino_coletanea_num_unique_v1 já existe com definição errada")
	}

	// 3. Banco com duplicatas: migração aborta e linhas permanecem intactas
	connDup, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco dup: %v", err)
	}
	defer connDup.Close()

	if _, err := connDup.Exec(`
		CREATE TABLE coletanea (id INTEGER PRIMARY KEY AUTOINCREMENT, codigo TEXT NOT NULL, titulo TEXT NOT NULL);
		CREATE TABLE hino (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			coletanea_id INTEGER NOT NULL REFERENCES coletanea(id),
			numeracao INTEGER,
			titulo TEXT NOT NULL,
			letra TEXT,
			creditos TEXT,
			revisado INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX idx_hino_coletanea_num ON hino (coletanea_id, numeracao);
		INSERT INTO coletanea (id, codigo, titulo) VALUES (1, 'CC', 'Cantor Cristão');
		INSERT INTO hino (coletanea_id, numeracao, titulo) VALUES (1, 42, 'Hino 42 Original');
		INSERT INTO hino (coletanea_id, numeracao, titulo) VALUES (1, 42, 'Hino 42 Duplicado');
	`); err != nil {
		t.Fatalf("setup dup: %v", err)
	}

	err = database.Migrate(connDup)
	if err == nil {
		t.Fatalf("Migrate deveria abortar em banco com duplicatas")
	}
	if !strings.Contains(err.Error(), "duplicatas") && !strings.Contains(err.Error(), "abortando") {
		t.Fatalf("mensagem de erro inesperada: %v", err)
	}

	// Verifica que as linhas permaneceram intactas
	var count int
	if err := connDup.QueryRow(`SELECT COUNT(*) FROM hino WHERE coletanea_id = 1 AND numeracao = 42`).Scan(&count); err != nil {
		t.Fatalf("contar hinos: %v", err)
	}
	if count != 2 {
		t.Fatalf("esperava 2 hinos intactos após abort de migração, encontrou %d", count)
	}
}

const dominioSchemaTeste = `
CREATE TABLE coletanea (
	id INTEGER PRIMARY KEY, codigo TEXT NOT NULL, titulo TEXT NOT NULL
);
CREATE TABLE hino (
	id INTEGER, coletanea_id INTEGER NOT NULL REFERENCES coletanea(id),
	numeracao INTEGER, titulo TEXT NOT NULL,
	letra TEXT, creditos TEXT, revisado INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY(id)
);
CREATE UNIQUE INDEX unique_hino_legado ON hino(coletanea_id, numeracao);
`

func TestImportSchemaRejeitaColunasIncompativeis(t *testing.T) {
	for _, tc := range []struct {
		name, from, to string
		importTable    bool
	}{
		{name: "letra inteira", from: "letra TEXT", to: "letra INTEGER"},
		{name: "creditos inteiros", from: "creditos TEXT", to: "creditos INTEGER"},
		{name: "numero textual", from: "numeracao INTEGER", to: "numeracao TEXT"},
		{name: "coletanea textual", from: "coletanea_id INTEGER", to: "coletanea_id TEXT"},
		{name: "titulo hino inteiro", from: "numeracao INTEGER, titulo TEXT", to: "numeracao INTEGER, titulo INTEGER"},
		{name: "revisado textual", from: "revisado INTEGER", to: "revisado TEXT"},
		{name: "codigo inteiro", from: "codigo TEXT", to: "codigo INTEGER"},
		{name: "id coletanea textual", from: "id INTEGER PRIMARY KEY", to: "id TEXT PRIMARY KEY"},
		{name: "id hino textual", from: "id INTEGER,", to: "id TEXT,"},
		{name: "coletanea sem PK", from: "id INTEGER PRIMARY KEY", to: "id INTEGER"},
		{name: "hino sem PK", from: ",\n\tPRIMARY KEY(id)", to: ""},
		{name: "hino com PK composta", from: "PRIMARY KEY(id)", to: "PRIMARY KEY(id,numeracao)"},
		{name: "coletanea sem FK", from: " REFERENCES coletanea(id)", to: ""},
		{name: "coletanea nullable", from: "coletanea_id INTEGER NOT NULL", to: "coletanea_id INTEGER"},
		{name: "codigo nullable", from: "codigo TEXT NOT NULL", to: "codigo TEXT"},
		{name: "titulo hino nullable", from: "numeracao INTEGER, titulo TEXT NOT NULL", to: "numeracao INTEGER, titulo TEXT"},
		{name: "revisado nullable", from: "revisado INTEGER NOT NULL", to: "revisado INTEGER"},
		{name: "letra nao nullable", from: "letra TEXT", to: "letra TEXT NOT NULL"},
		{name: "creditos nao nullable", from: "creditos TEXT", to: "creditos TEXT NOT NULL"},
		{name: "numero nao nullable", from: "numeracao INTEGER", to: "numeracao INTEGER NOT NULL"},
		{name: "batch id inteiro", importTable: true, from: "batch_id TEXT PRIMARY KEY", to: "batch_id INTEGER PRIMARY KEY"},
		{name: "manifest hash inteiro", importTable: true, from: "manifest_sha256 TEXT NOT NULL", to: "manifest_sha256 INTEGER NOT NULL"},
		{name: "data batch inteira", importTable: true, from: "criado_em TEXT", to: "criado_em INTEGER"},
		{name: "log hino textual", importTable: true, from: "hino_id INTEGER", to: "hino_id TEXT"},
		{name: "log letra inteira", importTable: true, from: "letra_nova TEXT", to: "letra_nova INTEGER"},
		{name: "log creditos inteiros", importTable: true, from: "creditos_anterior TEXT", to: "creditos_anterior INTEGER"},
		{name: "log flag textual", importTable: true, from: "creditos_alterados INTEGER", to: "creditos_alterados TEXT"},
		{name: "batch unique parcial", importTable: true, from: "UNIQUE(batch_id, manifest_sha256)\n);", to: "CHECK(1)\n);\nCREATE UNIQUE INDEX unique_batch_parcial ON import_batch(batch_id,manifest_sha256) WHERE batch_id IS NOT NULL;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := database.Open(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			dominioDDL, importDDL := dominioSchemaTeste, database.ImportSchemaSQL()
			if tc.importTable {
				importDDL = strings.Replace(importDDL, tc.from, tc.to, 1)
			} else {
				dominioDDL = strings.Replace(dominioDDL, tc.from, tc.to, 1)
			}
			if _, err := conn.Exec(dominioDDL + importDDL); err != nil {
				t.Fatalf("criar fixture: %v", err)
			}
			if err := database.ValidarImportSchema(context.Background(), conn); err == nil {
				t.Error("guarda Go aceitou schema incompatível")
			}
			if _, err := conn.Exec(database.ValidacaoImportSchemaSQL()); err == nil {
				t.Error("guarda SQL standalone aceitou schema incompatível")
			}
			if err := database.Migrate(conn); err == nil {
				t.Error("migração aceitou schema incompatível")
			}
		})
	}
}

func TestImportSchemaStandaloneInteroperavel(t *testing.T) {
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(dominioSchemaTeste + `
		ALTER TABLE hino ADD COLUMN nota TEXT;
		ALTER TABLE coletanea ADD COLUMN nota TEXT;
	`); err != nil {
		t.Fatal(err)
	}
	if err := database.ValidarImportSchema(context.Background(), conn); err != nil {
		t.Fatalf("legado sem tabelas de import: %v", err)
	}
	for range 2 {
		if _, err := conn.Exec(database.ImportSchemaSQL() + database.ValidacaoImportSchemaSQL()); err != nil {
			t.Fatalf("DDL e guarda standalone: %v", err)
		}
		if err := database.Migrate(conn); err != nil {
			t.Fatalf("migração após DDL standalone: %v", err)
		}
		if err := database.ValidarImportSchema(context.Background(), conn); err != nil {
			t.Fatalf("guarda Go após DDL standalone: %v", err)
		}
	}
}

func TestIndiceReservadoIncompativelComEquivalente(t *testing.T) {
	for _, indice := range []string{
		"CREATE INDEX idx_hino_coletanea_num_unique_v1 ON hino(titulo)",
		"CREATE INDEX idx_hino_coletanea_num_unique_v1 ON hino(coletanea_id,numeracao)",
		"CREATE UNIQUE INDEX idx_hino_coletanea_num_unique_v1 ON hino(coletanea_id,numeracao) WHERE numeracao IS NOT NULL",
		"CREATE INDEX idx_hino_coletanea_num_unique_v1 ON coletanea(titulo)",
	} {
		t.Run(indice, func(t *testing.T) {
			conn, err := database.Open(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if _, err := conn.Exec(dominioSchemaTeste + indice); err != nil {
				t.Fatal(err)
			}
			if err := database.Migrate(conn); err == nil {
				t.Error("migração ignorou colisão do nome reservado porque havia outro índice equivalente")
			}
			var count int
			if err := conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name IN ('import_batch','import_log')").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Error("migração não reverteu o DDL de import após colisão do índice")
			}
		})
	}
}
