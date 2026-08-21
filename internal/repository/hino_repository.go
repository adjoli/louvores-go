package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/adjoli/louvores-go/internal/models"
)

// hinoCols lista as colunas da tabela hino usadas nos SELECTs — mantidas em
// um único lugar para que a consulta e o Scan nunca diverjam.
const hinoCols = "id, coletanea_id, numeracao, titulo, letra, creditos, revisado"

// rowScanner é a interface pequena satisfeita implicitamente tanto por
// *sql.Row quanto por *sql.Rows, permitindo que scanHino sirva às consultas
// de linha única e de múltiplas linhas.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanHino lê as colunas definidas em hinoCols para um models.Hino.
// Campos opcionais (Numeracao, Letra, Creditos) são escaneados direto nos
// ponteiros: NULL vira nil.
func scanHino(row rowScanner) (*models.Hino, error) {
	hino := &models.Hino{}

	if err := row.Scan(
		&hino.ID,
		&hino.ColetaneaID,
		&hino.Numeracao,
		&hino.Titulo,
		&hino.Letra,
		&hino.Creditos,
		&hino.Revisado,
	); err != nil {
		return nil, err
	}

	return hino, nil
}

// HinoRepository é o repositório de persistência de hinos.
// Ele encapsula as operações SQL e mapeia resultados para models.Hino.
type HinoRepository struct {
	db *sql.DB
}

// NewHinoRepository cria um novo HinoRepository com a conexão de banco fornecida.
func NewHinoRepository(db *sql.DB) *HinoRepository {
	return &HinoRepository{
		db: db,
	}
}

// Create insere um novo hino no banco e popula o campo ID
// do objeto passado como referência.
func (r *HinoRepository) Create(
	ctx context.Context,
	hino *models.Hino,
) error {
	result, err := r.db.ExecContext(
		ctx,
		"INSERT INTO hino (coletanea_id, numeracao, titulo, letra, creditos, revisado) VALUES (?,?,?,?,?,?)",
		hino.ColetaneaID,
		hino.Numeracao,
		hino.Titulo,
		hino.Letra,
		hino.Creditos,
		hino.Revisado,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	hino.ID = id

	return nil
}

// FindByID busca um hino pelo seu ID.
// Retorna ErrHinoNotFound se o hino não existir.
func (r *HinoRepository) FindByID(
	ctx context.Context,
	id int64,
) (*models.Hino, error) {
	row := r.db.QueryRowContext(
		ctx,
		"SELECT "+hinoCols+" FROM hino WHERE id = ?",
		id,
	)

	hino, err := scanHino(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrHinoNotFound
		}
		return nil, err
	}

	return hino, nil
}

// List retorna todos os hinos.
// O resultado é ordenado por ID crescente.
func (r *HinoRepository) List(
	ctx context.Context,
) ([]models.Hino, error) {
	rows, err := r.db.QueryContext(
		ctx,
		"SELECT "+hinoCols+" FROM hino ORDER BY id",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hinos []models.Hino

	for rows.Next() {
		hino, err := scanHino(rows)
		if err != nil {
			return nil, err
		}

		hinos = append(hinos, *hino)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return hinos, nil
}

// Update atualiza os dados de um hino existente.
// Retorna ErrHinoNotFound se o ID não existir no banco.
func (r *HinoRepository) Update(
	ctx context.Context,
	hino *models.Hino,
) error {
	result, err := r.db.ExecContext(
		ctx,
		"UPDATE hino SET coletanea_id = ?, numeracao = ?, titulo = ?, letra = ?, creditos = ?, revisado = ? WHERE id = ?",
		&hino.ColetaneaID,
		&hino.Numeracao,
		&hino.Titulo,
		&hino.Letra,
		&hino.Creditos,
		&hino.Revisado,
		&hino.ID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrHinoNotFound
	}

	return nil
}

// Delete remove um hino pelo seu ID.
// Retorna ErrHinoNotFound se o ID não existir no banco.
func (r *HinoRepository) Delete(
	ctx context.Context,
	id int64,
) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM hino WHERE id = ?", id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrHinoNotFound
	}

	return nil
}

// StatsRow é a projeção de uma linha agregada por coletânea (resultado do
// GROUP BY do SQL). Os campos derivados (não revisados, percentual) ficam
// por conta da camada de serviços.
type StatsRow struct {
	Codigo    string
	Titulo    string
	Total     int
	ComLetra  int
	Revisados int
}

// StatsPorColetanea agrega, por coletânea, o total de hinos, os que têm
// letra e os revisados — tudo em uma única consulta com GROUP BY, evitando
// N+1 consultas por coletânea.
func (r *HinoRepository) StatsPorColetanea(
	ctx context.Context,
) ([]StatsRow, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT c.codigo, c.titulo,
		        COUNT(h.id) AS total,
		        COUNT(h.letra) AS com_letra,
		        COALESCE(SUM(CASE WHEN h.revisado = 1 THEN 1 ELSE 0 END), 0) AS revisados
		 FROM coletanea c
		 JOIN hino h ON h.coletanea_id = c.id
		 GROUP BY c.codigo, c.titulo
		 ORDER BY c.codigo`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StatsRow

	for rows.Next() {
		var s StatsRow

		if err := rows.Scan(&s.Codigo, &s.Titulo, &s.Total, &s.ComLetra, &s.Revisados); err != nil {
			return nil, err
		}

		out = append(out, s)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

// ListByColetanea retorna todos os hinos pertencentes à coletânea
// informada, ordenados pela numeração.
func (r *HinoRepository) ListByColetanea(
	ctx context.Context,
	coletaneaID int64,
) ([]models.Hino, error) {
	rows, err := r.db.QueryContext(
		ctx,
		"SELECT "+hinoCols+" FROM hino WHERE coletanea_id = ? ORDER BY numeracao",
		coletaneaID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hinos []models.Hino

	for rows.Next() {
		hino, err := scanHino(rows)
		if err != nil {
			return nil, err
		}

		hinos = append(hinos, *hino)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return hinos, nil
}

// FindByNumero busca um hino pela combinação de coletânea e numeração —
// a chave de negócio usada na interface (ex.: CC/42).
// Retorna ErrHinoNotFound se não houver hino correspondente.
func (r *HinoRepository) FindByNumero(
	ctx context.Context,
	coletaneaID int64,
	numero int,
) (*models.Hino, error) {
	row := r.db.QueryRowContext(
		ctx,
		"SELECT "+hinoCols+" FROM hino WHERE coletanea_id = ? AND numeracao = ?",
		coletaneaID,
		numero,
	)

	hino, err := scanHino(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrHinoNotFound
		}
		return nil, err
	}

	return hino, nil
}
