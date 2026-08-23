package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/adjoli/louvores-go/internal/models"
)

// SQLiteColetaneaRepository é o repositório de persistência de coletâneas (implementação SQLite).
// Ele encapsula as operações SQL e mapeia resultados para models.Coletanea.
type SQLiteColetaneaRepository struct {
	db *sql.DB
}

// NewSQLiteColetaneaRepository cria um novo SQLiteColetaneaRepository com a conexão de banco fornecida.
func NewSQLiteColetaneaRepository(db *sql.DB) *SQLiteColetaneaRepository {
	return &SQLiteColetaneaRepository{
		db: db,
	}
}

// Create insere uma nova coletanea no banco e popula o campo ID
// do objeto passado como referência.
func (r *SQLiteColetaneaRepository) Create(
	ctx context.Context,
	coletanea *models.Coletanea,
) error {
	result, err := r.db.ExecContext(
		ctx,
		"INSERT INTO coletanea (codigo, titulo) VALUES (?,?)",
		coletanea.Codigo,
		coletanea.Titulo,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	coletanea.ID = id

	return nil
}

// FindByID busca uma coletanea pelo seu ID.
// Retorna ErrColetaneaNotFound se a coletanea não existir.
func (r *SQLiteColetaneaRepository) FindByID(
	ctx context.Context,
	id int64,
) (*models.Coletanea, error) {
	row := r.db.QueryRowContext(
		ctx,
		"SELECT id, codigo, titulo FROM coletanea WHERE id = ?",
		id,
	)

	coletanea := &models.Coletanea{}

	if err := row.Scan(
		&coletanea.ID,
		&coletanea.Codigo,
		&coletanea.Titulo,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrColetaneaNotFound
		}
		return nil, err
	}

	return coletanea, nil
}

// List retorna todas as coletâneas.
// O resultado é ordenado por ID crescente.
func (r *SQLiteColetaneaRepository) List(
	ctx context.Context,
) ([]models.Coletanea, error) {
	rows, err := r.db.QueryContext(
		ctx,
		"SELECT id, codigo, titulo FROM coletanea ORDER BY id",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var coletaneas []models.Coletanea

	for rows.Next() {
		var coletanea models.Coletanea

		if err := rows.Scan(
			&coletanea.ID,
			&coletanea.Codigo,
			&coletanea.Titulo,
		); err != nil {
			return nil, err
		}

		coletaneas = append(coletaneas, coletanea)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return coletaneas, nil
}

// FindByCodigo busca uma coletanea pelo seu código curto (ex.: "CC").
// Retorna ErrColetaneaNotFound se não houver coletanea com o código.
func (r *SQLiteColetaneaRepository) FindByCodigo(
	ctx context.Context,
	codigo string,
) (*models.Coletanea, error) {
	row := r.db.QueryRowContext(
		ctx,
		"SELECT id, codigo, titulo FROM coletanea WHERE codigo = ?",
		codigo,
	)

	coletanea := &models.Coletanea{}

	if err := row.Scan(
		&coletanea.ID,
		&coletanea.Codigo,
		&coletanea.Titulo,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrColetaneaNotFound
		}
		return nil, err
	}

	return coletanea, nil
}

// Update atualiza os dados de uma coletanea existente.
// Retorna ErrColetaneaNotFound se o ID não existir no banco.
func (r *SQLiteColetaneaRepository) Update(
	ctx context.Context,
	coletanea *models.Coletanea,
) error {
	result, err := r.db.ExecContext(
		ctx,
		"UPDATE coletanea SET codigo = ?, titulo = ? WHERE id = ?",
		coletanea.Codigo,
		coletanea.Titulo,
		coletanea.ID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrColetaneaNotFound
	}

	return nil
}

// Delete remove uma coletanea pelo seu ID.
// Retorna ErrColetaneaNotFound se o ID não existir no banco.
func (r *SQLiteColetaneaRepository) Delete(
	ctx context.Context,
	id int64,
) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM coletanea WHERE id = ?", id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrColetaneaNotFound
	}

	return nil
}
