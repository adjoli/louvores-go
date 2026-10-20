package models_test

import (
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/models"
)

func strPtr(s string) *string {
	return &s
}

func TestSemConteudo(t *testing.T) {
	tests := []struct {
		name     string
		input    *string
		expected bool
	}{
		{name: "nil", input: nil, expected: true},
		{name: "empty", input: strPtr(""), expected: true},
		{name: "spaces only", input: strPtr("   "), expected: true},
		{name: "tabs only", input: strPtr("\t\t"), expected: true},
		{name: "newlines only", input: strPtr("\n\r\n"), expected: true},
		{name: "mixed ascii whitespace", input: strPtr(" \t\r\n\v\f "), expected: true},
		{name: "nbsp only", input: strPtr("\u00a0"), expected: false},
		{name: "spaces with nbsp", input: strPtr("  \u00a0  "), expected: false},
		{name: "single letter with accent", input: strPtr("Ó"), expected: false},
		{name: "leading and trailing whitespace with letter", input: strPtr("  Ó  "), expected: false},
		{name: "text", input: strPtr("Vinde cantai"), expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := models.SemConteudo(tc.input)
			if got != tc.expected {
				t.Errorf("SemConteudo(%v) = %v; esperado %v", tc.input, got, tc.expected)
			}
		})
	}

	// Testa a equivalência exata com a expressão SQL:
	// (valor IS NULL OR trim(valor, char(9)||char(10)||char(11)||char(12)||char(13)||char(32)) = '')
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco em memória: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Exec(`CREATE TABLE teste_conteudo (id INTEGER PRIMARY KEY, valor TEXT);`); err != nil {
		t.Fatalf("criar tabela teste: %v", err)
	}

	for i, tc := range tests {
		res, err := conn.Exec(`INSERT INTO teste_conteudo (id, valor) VALUES (?, ?)`, i+1, tc.input)
		if err != nil {
			t.Fatalf("inserir teste %s: %v", tc.name, err)
		}
		_ = res

		var sqlEmpty int
		sqlExpr := `SELECT (valor IS NULL OR trim(valor, char(9)||char(10)||char(11)||char(12)||char(13)||char(32)) = '') FROM teste_conteudo WHERE id = ?`
		if err := conn.QueryRow(sqlExpr, i+1).Scan(&sqlEmpty); err != nil {
			t.Fatalf("consultar teste %s: %v", tc.name, err)
		}

		goResult := models.SemConteudo(tc.input)
		sqlResult := sqlEmpty == 1
		if goResult != sqlResult {
			t.Errorf("divergência Go vs SQL no caso %q: Go=%v, SQL=%v", tc.name, goResult, sqlResult)
		}
	}
}
