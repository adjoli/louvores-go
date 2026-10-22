package etl

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/repository"
)

// SQLArtefatos encapsula os scripts transacionais e de verificação gerados a partir do manifesto.
type SQLArtefatos struct {
	Import   string
	Rollback string
	Verify   string
}

// literalSQL serializa um ponteiro de string para literal SQL seguro:
// - nil -> "NULL"
// - UTF-8 inválido e caracteres proibidos em XML 1.0 -> erro
// - aspas simples (') duplicadas (”) e envolvidas em '...'
func literalSQL(v *string) (string, error) {
	if v == nil {
		return "NULL", nil
	}
	if !utf8.ValidString(*v) {
		return "", errors.New("string contém sequência UTF-8 inválida")
	}
	for _, r := range *v {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' || r == 0xfffe || r == 0xffff {
			return "", fmt.Errorf("string contém caractere incompatível com XML: U+%04X", r)
		}
	}
	escaped := strings.ReplaceAll(*v, "'", "''")
	return "'" + escaped + "'", nil
}

func literalString(s string) (string, error) {
	return literalSQL(&s)
}

// GerarSQL gera os três scripts SQL (Import, Rollback, Verify) a partir de um Manifesto
// e do hash SHA-256 do arquivo do manifesto.
func GerarSQL(m *Manifesto, manifestSHA256 string) (SQLArtefatos, error) {
	if m == nil {
		return SQLArtefatos{}, errors.New("manifesto nulo")
	}
	if !ehSHA256Hex(manifestSHA256) {
		return SQLArtefatos{}, fmt.Errorf("manifest_sha256 %q inválido, esperado SHA-256 hex de 64 caracteres", manifestSHA256)
	}
	if strings.TrimSpace(m.Meta.BatchID) == "" {
		return SQLArtefatos{}, errors.New("batch_id vazio no metadata do manifesto")
	}

	batchIDSQL, err := literalString(m.Meta.BatchID)
	if err != nil {
		return SQLArtefatos{}, fmt.Errorf("batch_id inválido: %w", err)
	}
	manifestSHASQL, err := literalString(manifestSHA256)
	if err != nil {
		return SQLArtefatos{}, fmt.Errorf("manifest_sha256 inválido: %w", err)
	}

	repo := (*repository.ImportRepository)(nil)
	guarda := repo.SQLGuardaDestino("h")

	// 1. import.sql
	var importBuilder strings.Builder
	importBuilder.WriteString("PRAGMA foreign_keys = ON;\n")
	importBuilder.WriteString("BEGIN IMMEDIATE;\n\n")
	importBuilder.WriteString(database.ImportSchemaSQL())
	importBuilder.WriteString("\n\n")
	importBuilder.WriteString(database.ValidacaoImportSchemaSQL())
	importBuilder.WriteString("\n\n")
	importBuilder.WriteString(fmt.Sprintf("INSERT OR ROLLBACK INTO import_batch (batch_id, manifest_sha256)\nVALUES (%s, %s)\nON CONFLICT(batch_id, manifest_sha256) DO NOTHING;\n\n", batchIDSQL, manifestSHASQL))

	for _, item := range m.Itens {
		codigoSQL, err := literalString(item.Codigo)
		if err != nil {
			return SQLArtefatos{}, fmt.Errorf("hino %d: código inválido: %w", item.HinoID, err)
		}
		tituloSQL, err := literalString(item.TituloEsperado)
		if err != nil {
			return SQLArtefatos{}, fmt.Errorf("hino %d: título inválido: %w", item.HinoID, err)
		}
		letraSQL, err := literalString(item.Letra)
		if err != nil {
			return SQLArtefatos{}, fmt.Errorf("hino %d: letra inválida: %w", item.HinoID, err)
		}

		var creditosNovosExpr string
		var creditosAlteradosExpr string

		if item.Creditos == nil {
			creditosNovosExpr = "h.creditos"
			creditosAlteradosExpr = "0"
		} else {
			propSQL, err := literalString(*item.Creditos)
			if err != nil {
				return SQLArtefatos{}, fmt.Errorf("hino %d: créditos inválidos: %w", item.HinoID, err)
			}
			creditosNovosExpr = fmt.Sprintf(
				"CASE WHEN (h.creditos IS NULL OR trim(h.creditos, char(9)||char(10)||char(11)||char(12)||char(13)||char(32)) = '') THEN %s ELSE h.creditos END",
				propSQL,
			)
			creditosAlteradosExpr = fmt.Sprintf(
				"CASE WHEN (%s) IS NOT h.creditos THEN 1 ELSE 0 END",
				creditosNovosExpr,
			)
		}

		importBuilder.WriteString(fmt.Sprintf(`INSERT OR ROLLBACK INTO import_log (
    batch_id,
    manifest_sha256,
    hino_id,
    codigo,
    numero,
    titulo_destino,
    letra_anterior,
    creditos_anterior,
    letra_nova,
    creditos_novos,
    creditos_alterados
)
SELECT
    %s,
    %s,
    h.id,
    c.codigo,
    h.numeracao,
    h.titulo,
    h.letra,
    h.creditos,
    %s,
    %s,
    %s
FROM hino h
JOIN coletanea c ON c.id = h.coletanea_id
WHERE h.id = %d
  AND h.coletanea_id = %d
  AND c.codigo = %s
  AND h.numeracao = %d
  AND h.titulo = %s
  AND %s
  AND EXISTS (
      SELECT 1 FROM import_batch
      WHERE batch_id = %s AND manifest_sha256 = %s
  )
  AND NOT EXISTS (
      SELECT 1 FROM import_log
      WHERE batch_id = %s AND hino_id = %d
  );
`,
			batchIDSQL,
			manifestSHASQL,
			letraSQL,
			creditosNovosExpr,
			creditosAlteradosExpr,
			item.HinoID,
			item.ColetaneaID,
			codigoSQL,
			item.Numero,
			tituloSQL,
			guarda,
			batchIDSQL,
			manifestSHASQL,
			batchIDSQL,
			item.HinoID,
		))
		// Se um trigger mudou a linha após o log, abortar em vez de deixar um
		// evento de sucesso sem UPDATE. O executor fail-fast reverte a transação.
		importBuilder.WriteString(fmt.Sprintf(`UPDATE OR ROLLBACK hino AS h
SET letra = CASE WHEN
        h.coletanea_id = %d AND h.numeracao IS log.numero
        AND h.titulo IS log.titulo_destino AND h.revisado = 0
        AND EXISTS (SELECT 1 FROM coletanea WHERE id = h.coletanea_id AND codigo IS log.codigo)
        AND h.letra IS log.letra_anterior AND h.creditos IS log.creditos_anterior
    THEN log.letra_nova ELSE abs(-9223372036854775808) END,
    creditos = log.creditos_novos
FROM import_log AS log
WHERE h.id = %d
  AND log.batch_id = %s AND log.manifest_sha256 = %s
  AND log.hino_id = h.id AND log.revertido_em IS NULL
  AND (SELECT changes() = 1);

`, item.ColetaneaID, item.HinoID, batchIDSQL, manifestSHASQL))
	}

	importBuilder.WriteString("COMMIT;\n")

	// 2. rollback.sql
	var rollbackBuilder strings.Builder
	rollbackBuilder.WriteString("PRAGMA foreign_keys = ON;\n")
	rollbackBuilder.WriteString("BEGIN IMMEDIATE;\n\n")
	rollbackBuilder.WriteString(database.ValidacaoImportSchemaSQL())
	rollbackBuilder.WriteString("\n\n")

	for _, item := range m.Itens {
		codigoSQL, err := literalString(item.Codigo)
		if err != nil {
			return SQLArtefatos{}, err
		}
		tituloSQL, err := literalString(item.TituloEsperado)
		if err != nil {
			return SQLArtefatos{}, err
		}
		rollbackBuilder.WriteString(fmt.Sprintf(`UPDATE OR ROLLBACK hino
SET letra = log.letra_anterior,
    creditos = CASE WHEN log.creditos_alterados = 1 THEN log.creditos_anterior ELSE hino.creditos END
FROM import_log AS log
WHERE hino.id = %d
  AND log.batch_id = %s
  AND log.manifest_sha256 = %s
  AND log.hino_id = hino.id
  AND hino.coletanea_id = %d AND hino.numeracao IS %d
  AND hino.titulo IS %s
  AND EXISTS (SELECT 1 FROM coletanea WHERE id = hino.coletanea_id AND codigo IS %s)
  AND log.numero IS hino.numeracao AND log.titulo_destino IS hino.titulo
  AND log.codigo IS %s
  AND log.revertido_em IS NULL
  AND hino.revisado = 0
  AND hino.letra IS log.letra_nova
  AND hino.creditos IS log.creditos_novos;
UPDATE OR ROLLBACK import_log
SET revertido_em = CURRENT_TIMESTAMP
WHERE batch_id = %s
  AND manifest_sha256 = %s
  AND hino_id = %d
  AND revertido_em IS NULL
  AND (SELECT changes() = 1);

`,
			item.HinoID,
			batchIDSQL,
			manifestSHASQL,
			item.ColetaneaID,
			item.Numero,
			tituloSQL,
			codigoSQL,
			codigoSQL,
			batchIDSQL,
			manifestSHASQL,
			item.HinoID,
		))
	}

	rollbackBuilder.WriteString("COMMIT;\n")

	// 3. verify.sql
	var verifyBuilder strings.Builder
	if len(m.Itens) == 0 {
		verifyBuilder.WriteString(`SELECT
    NULL AS hino_id,
    NULL AS codigo,
    NULL AS numero,
    NULL AS titulo_esperado,
    NULL AS status,
    NULL AS detalhe
WHERE 0;
`)
	} else {
		var alvosRows []string
		for _, item := range m.Itens {
			codSQL, err := literalString(item.Codigo)
			if err != nil {
				return SQLArtefatos{}, err
			}
			titSQL, err := literalString(item.TituloEsperado)
			if err != nil {
				return SQLArtefatos{}, err
			}
			alvosRows = append(alvosRows, fmt.Sprintf("(%d, %s, %d, %s, %d)", item.HinoID, codSQL, item.Numero, titSQL, item.ColetaneaID))
		}

		verifyBuilder.WriteString(fmt.Sprintf(`WITH alvos(hino_id, codigo, numero, titulo_esperado, coletanea_id) AS (
    VALUES
        %s
)
SELECT
    alvos.hino_id,
    alvos.codigo,
    alvos.numero,
    alvos.titulo_esperado,
    CASE
        WHEN h.id IS NULL THEN 'ausente'
        WHEN log.hino_id IS NULL THEN 'ausente'
        WHEN log.manifest_sha256 IS NOT %s THEN 'conflito'
        WHEN log.revertido_em IS NOT NULL THEN 'revertido'
        WHEN h.revisado = 1
          OR h.letra IS NOT log.letra_nova
          OR h.creditos IS NOT log.creditos_novos
          OR h.titulo != alvos.titulo_esperado
          OR c.codigo IS NOT alvos.codigo
          OR h.numeracao IS NOT alvos.numero
          OR h.coletanea_id IS NOT alvos.coletanea_id
          THEN 'conflito'
        ELSE 'ativo'
    END AS status,
    CASE
        WHEN h.id IS NULL THEN 'hino não encontrado no banco'
        WHEN log.hino_id IS NULL THEN 'log não encontrado para este batch'
        WHEN log.manifest_sha256 IS NOT %s THEN 'hash do manifesto divergente'
        WHEN log.revertido_em IS NOT NULL THEN 'importação revertida'
        WHEN h.revisado = 1 THEN 'hino revisado por humano'
        WHEN h.letra IS NOT log.letra_nova THEN 'letra divergente da importada'
        WHEN h.creditos IS NOT log.creditos_novos THEN 'créditos divergentes dos importados'
        WHEN h.titulo != alvos.titulo_esperado THEN 'título divergente do esperado'
        WHEN c.codigo IS NOT alvos.codigo THEN 'coletânea divergente do esperado'
        WHEN h.numeracao IS NOT alvos.numero THEN 'número divergente do esperado'
        WHEN h.coletanea_id IS NOT alvos.coletanea_id THEN 'identidade da coletânea divergente'
        ELSE 'ok'
    END AS detalhe
FROM alvos
LEFT JOIN hino h ON h.id = alvos.hino_id
LEFT JOIN coletanea c ON c.id = h.coletanea_id
LEFT JOIN import_log log ON log.batch_id = %s AND log.hino_id = alvos.hino_id
ORDER BY alvos.codigo, alvos.numero, alvos.hino_id;
`, strings.Join(alvosRows, ",\n        "), manifestSHASQL, manifestSHASQL, batchIDSQL))
	}

	return SQLArtefatos{
		Import:   importBuilder.String(),
		Rollback: rollbackBuilder.String(),
		Verify:   verifyBuilder.String(),
	}, nil
}
