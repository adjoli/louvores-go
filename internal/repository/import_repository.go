package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// DestinoImport representa o alvo resolvido de um hino no banco para fins de importação.
type DestinoImport struct {
	ID          int64
	ColetaneaID int64
	Codigo      string
	Numero      int
	Titulo      string
	Letra       *string
	Creditos    *string
	Revisado    bool
}

// ImportRepository gerencia consultas e operações específicas para importação auditada.
type ImportRepository struct {
	db *sql.DB
}

// NewImportRepository cria um novo repositório de importação.
func NewImportRepository(db *sql.DB) *ImportRepository {
	return &ImportRepository{
		db: db,
	}
}

// LerDestino busca o hino correspondente à chave de negócio (coletânea + número).
// Detecta zero ou múltiplos destinos (não escolhe o primeiro em caso de ambiguidade).
// Retorna ErrColetaneaNotFound se a coletânea não existir,
// ErrHinoNotFound se o hino não existir,
// ou ErrDestinoAmbiguo se houver duplicatas da coletânea ou do hino.
func (r *ImportRepository) LerDestino(ctx context.Context, codigo string, numero int) (DestinoImport, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, codigo FROM coletanea WHERE codigo = ?", codigo)
	if err != nil {
		return DestinoImport{}, err
	}
	defer rows.Close()

	var coletaneas []struct {
		id     int64
		codigo string
	}
	for rows.Next() {
		var c struct {
			id     int64
			codigo string
		}
		if err := rows.Scan(&c.id, &c.codigo); err != nil {
			return DestinoImport{}, err
		}
		coletaneas = append(coletaneas, c)
	}
	if err := rows.Err(); err != nil {
		return DestinoImport{}, err
	}

	if len(coletaneas) == 0 {
		return DestinoImport{}, ErrColetaneaNotFound
	}
	if len(coletaneas) > 1 {
		return DestinoImport{}, ErrDestinoAmbiguo
	}

	coletaneaID := coletaneas[0].id
	codigoNormalizado := coletaneas[0].codigo

	hinoRows, err := r.db.QueryContext(ctx, `
		SELECT id, coletanea_id, numeracao, titulo, letra, creditos, revisado
		FROM hino
		WHERE coletanea_id = ? AND numeracao = ?
	`, coletaneaID, numero)
	if err != nil {
		return DestinoImport{}, err
	}
	defer hinoRows.Close()

	var hinos []DestinoImport
	for hinoRows.Next() {
		var d DestinoImport
		var revInt int
		var num sql.NullInt64
		if err := hinoRows.Scan(&d.ID, &d.ColetaneaID, &num, &d.Titulo, &d.Letra, &d.Creditos, &revInt); err != nil {
			return DestinoImport{}, err
		}
		d.Codigo = codigoNormalizado
		d.Numero = int(num.Int64)
		d.Revisado = revInt != 0
		hinos = append(hinos, d)
	}
	if err := hinoRows.Err(); err != nil {
		return DestinoImport{}, err
	}

	if len(hinos) == 0 {
		return DestinoImport{}, ErrHinoNotFound
	}
	if len(hinos) > 1 {
		return DestinoImport{}, ErrDestinoAmbiguo
	}

	return hinos[0], nil
}

// SQLGuardaDestino retorna o predicado estático compartilhado para a guarda do destino.
// A assinatura recebe o alias interno fixo "h", rejeitando qualquer outro valor para
// impedir injeção de SQL arbitrário.
func (r *ImportRepository) SQLGuardaDestino(alias string) string {
	if alias != "h" {
		panic("alias inválido em SQLGuardaDestino: esperado 'h'")
	}
	return fmt.Sprintf("(%[1]s.revisado = 0 AND (%[1]s.letra IS NULL OR trim(%[1]s.letra, char(9)||char(10)||char(11)||char(12)||char(13)||char(32)) = ''))", alias)
}

