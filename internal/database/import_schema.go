package database

import (
	"context"
	"database/sql"
	"fmt"
)

// importSchemaSQL define o DDL das tabelas e índices de importação auditável.
const importSchemaSQL = `CREATE TABLE IF NOT EXISTS import_batch (
	batch_id TEXT PRIMARY KEY,
	manifest_sha256 TEXT NOT NULL,
	criado_em TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(batch_id, manifest_sha256)
);

CREATE TABLE IF NOT EXISTS import_log (
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
	creditos_alterados INTEGER NOT NULL CHECK(creditos_alterados IN (0,1)),
	aplicado_em TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	revertido_em TEXT,
	PRIMARY KEY(batch_id,hino_id),
	FOREIGN KEY(batch_id,manifest_sha256) REFERENCES import_batch(batch_id,manifest_sha256)
);

CREATE INDEX IF NOT EXISTS idx_import_log_hino_ativo ON import_log(hino_id) WHERE revertido_em IS NULL;`

// ImportSchemaSQL retorna o DDL exato das tabelas e índices de importação auditável.
func ImportSchemaSQL() string {
	return importSchemaSQL
}

// validacaoImportSchemaSQL define a assertion SQL pura que valida o schema
// e gera integer overflow deliberado em caso de incompatibilidade.
const validacaoImportSchemaSQL = `SELECT CASE WHEN (
    (SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('import_batch', 'import_log')) = 2
    AND (SELECT COUNT(*) FROM pragma_table_info('import_batch') WHERE upper(type)='TEXT' AND (
        (name='batch_id' AND pk=1) OR
        (name='manifest_sha256' AND pk=0 AND [notnull]=1) OR
        (name='criado_em' AND pk=0 AND [notnull]=1)
    )) = 3
    AND (SELECT COUNT(*) FROM pragma_table_info('import_batch')) = 3
    AND EXISTS (
        SELECT 1 FROM pragma_index_list('import_batch') il
        WHERE il.[unique] = 1 AND il.partial = 0
        AND (SELECT COUNT(*) FROM pragma_index_info(il.name)) = 2
        AND (SELECT name FROM pragma_index_info(il.name) WHERE seqno = 0) = 'batch_id'
        AND (SELECT name FROM pragma_index_info(il.name) WHERE seqno = 1) = 'manifest_sha256'
    )
    AND (SELECT COUNT(*) FROM pragma_table_info('import_log')) = 13
    AND (SELECT COUNT(*) FROM pragma_table_info('import_log') WHERE
        (name='batch_id' AND upper(type)='TEXT' AND pk=1 AND [notnull]=1) OR
        (name='manifest_sha256' AND upper(type)='TEXT' AND pk=0 AND [notnull]=1) OR
        (name='hino_id' AND upper(type)='INTEGER' AND pk=2 AND [notnull]=1) OR
        (name='codigo' AND upper(type)='TEXT' AND pk=0 AND [notnull]=1) OR
        (name='numero' AND upper(type)='INTEGER' AND pk=0 AND [notnull]=1) OR
        (name='titulo_destino' AND upper(type)='TEXT' AND pk=0 AND [notnull]=1) OR
        (name='letra_anterior' AND upper(type)='TEXT' AND pk=0 AND [notnull]=0) OR
        (name='creditos_anterior' AND upper(type)='TEXT' AND pk=0 AND [notnull]=0) OR
        (name='letra_nova' AND upper(type)='TEXT' AND pk=0 AND [notnull]=1) OR
        (name='creditos_novos' AND upper(type)='TEXT' AND pk=0 AND [notnull]=0) OR
        (name='creditos_alterados' AND upper(type)='INTEGER' AND pk=0 AND [notnull]=1) OR
        (name='aplicado_em' AND upper(type)='TEXT' AND pk=0 AND [notnull]=1) OR
        (name='revertido_em' AND upper(type)='TEXT' AND pk=0 AND [notnull]=0)
    ) = 13
    AND EXISTS (
        SELECT 1 FROM pragma_foreign_key_list('import_log')
        WHERE "table" = 'hino' AND "from" = 'hino_id' AND "to" = 'id'
    )
    AND EXISTS (
        SELECT 1 FROM pragma_foreign_key_list('import_log') fk1
        JOIN pragma_foreign_key_list('import_log') fk2 ON fk1.id = fk2.id
        WHERE fk1."table" = 'import_batch' AND fk1."from" = 'batch_id' AND fk1."to" = 'batch_id' AND fk1.seq = 0
          AND fk2."table" = 'import_batch' AND fk2."from" = 'manifest_sha256' AND fk2."to" = 'manifest_sha256' AND fk2.seq = 1
    )
    AND EXISTS (
        SELECT 1 FROM sqlite_master
        WHERE type = 'table' AND name = 'import_log'
          AND replace(replace(replace(sql, ' ', ''), char(10), ''), char(9), '') LIKE '%CHECK(creditos_alteradosIN(0,1))%'
    )
    AND EXISTS (
        SELECT 1 FROM pragma_index_list('import_log') il
        WHERE il.name = 'idx_import_log_hino_ativo' AND il.partial = 1
        AND (SELECT name FROM pragma_index_info(il.name) WHERE seqno = 0) = 'hino_id'
        AND (SELECT COUNT(*) FROM pragma_index_info(il.name)) = 1
    )
    AND EXISTS (
        SELECT 1 FROM sqlite_master
        WHERE type = 'index' AND name = 'idx_import_log_hino_ativo'
          AND replace(replace(replace(sql, ' ', ''), char(10), ''), char(9), '') LIKE '%WHERErevertido_emISNULL%'
    )
    AND (SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('hino', 'coletanea')) = 2
    AND (SELECT COUNT(*) FROM pragma_table_info('hino') WHERE
        (name='id' AND upper(type)='INTEGER' AND pk=1) OR
        (name='coletanea_id' AND upper(type)='INTEGER' AND pk=0 AND [notnull]=1) OR
        (name='numeracao' AND upper(type)='INTEGER' AND pk=0 AND [notnull]=0) OR
        (name='titulo' AND upper(type)='TEXT' AND pk=0 AND [notnull]=1) OR
        (name='letra' AND upper(type)='TEXT' AND pk=0 AND [notnull]=0) OR
        (name='creditos' AND upper(type)='TEXT' AND pk=0 AND [notnull]=0) OR
        (name='revisado' AND upper(type)='INTEGER' AND pk=0 AND [notnull]=1)
    ) = 7
    AND (SELECT COUNT(*) FROM pragma_table_info('hino') WHERE pk > 0) = 1
    AND (SELECT COUNT(*) FROM pragma_table_info('coletanea') WHERE
        (name='id' AND upper(type)='INTEGER' AND pk=1) OR
        (name='codigo' AND upper(type)='TEXT' AND pk=0 AND [notnull]=1) OR
        (name='titulo' AND upper(type)='TEXT' AND pk=0 AND [notnull]=1)
    ) = 3
    AND (SELECT COUNT(*) FROM pragma_table_info('coletanea') WHERE pk > 0) = 1
    AND EXISTS (
        SELECT 1 FROM pragma_foreign_key_list('hino') fk
        WHERE fk."table" = 'coletanea' AND fk."from" = 'coletanea_id' AND fk."to" = 'id'
        AND (SELECT COUNT(*) FROM pragma_foreign_key_list('hino') WHERE id = fk.id) = 1
    )
    AND EXISTS (
        SELECT 1 FROM pragma_index_list('hino') il
        WHERE il.[unique] = 1 AND il.partial = 0
        AND (SELECT COUNT(*) FROM pragma_index_info(il.name)) = 2
        AND (SELECT name FROM pragma_index_info(il.name) WHERE seqno = 0) = 'coletanea_id'
        AND (SELECT name FROM pragma_index_info(il.name) WHERE seqno = 1) = 'numeracao'
    )
) THEN 1 ELSE abs(-9223372036854775808) END;`

// ValidacaoImportSchemaSQL retorna a assertion SQL pura standalone para validação do schema.
func ValidacaoImportSchemaSQL() string {
	return validacaoImportSchemaSQL
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ValidarImportSchema verifica se as tabelas e índices de importação estão conformes.
// Aceita snapshots legados onde AMBAS as tabelas de import estão ausentes,
// mas rejeita schema parcial ou com shape incompatível.
func ValidarImportSchema(ctx context.Context, db *sql.DB) error {
	return validarImportSchemaInternal(ctx, db, true)
}

func validarImportSchemaInternal(ctx context.Context, q queryer, allowAbsentBoth bool) error {
	var nTables int
	err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('import_batch', 'import_log')").Scan(&nTables)
	if err != nil {
		return fmt.Errorf("consultar sqlite_master: %w", err)
	}

	if nTables == 0 {
		if allowAbsentBoth {
			return nil
		}
		return fmt.Errorf("tabelas de import ausentes")
	}

	if nTables == 1 {
		return fmt.Errorf("schema parcial de import: apenas uma das tabelas existe")
	}

	// Executa a asserção SQL que valida a integridade estrutural das tabelas e índices
	var ok int
	if err := q.QueryRowContext(ctx, validacaoImportSchemaSQL).Scan(&ok); err != nil {
		return fmt.Errorf("schema de import incompatível: %w", err)
	}

	return nil
}
