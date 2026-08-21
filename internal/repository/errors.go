package repository

import (
	"database/sql"
	"fmt"
)

// Os erros são definidos com fmt.Errorf e %w (em vez de errors.New)
// para embrulhar sql.ErrNoRows: isso preserva a cadeia de causas e
// permite que o chamador detecte "não encontrado" com errors.Is(err,
// sql.ErrNoRows), sem depender de comparar a string da mensagem.
var (
	ErrHinoNotFound      = fmt.Errorf("hino não encontrado: %w", sql.ErrNoRows)
	ErrColetaneaNotFound = fmt.Errorf("coletânea não encontrada: %w", sql.ErrNoRows)
)
