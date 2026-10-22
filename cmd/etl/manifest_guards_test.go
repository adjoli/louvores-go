package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adjoli/louvores-go/internal/etl"
)

func TestCLIRecusaManifestoDivergenteDosRecibos(t *testing.T) {
	dir := t.TempDir()
	snap, tpl := criarSnapshotTeste(t, false, false), templateReal(t)
	id, sha := strings.Repeat("1", 64), strings.Repeat("a", 64)
	final, err := etl.HashFinal("CC", 42, "Ó Vem", "    Ó Vem\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	csv := criarCSVTeste(t, dir, fmt.Sprintf("p,a.pptx,CC,42,Ó Vem,\"    Ó Vem\n\",OK,,,%s,%s\n", id, sha))
	prov := criarProvenanceTeste(t, dir, id, sha, final, "CC", 42, "Ó Vem", "    Ó Vem\n", nil)
	path := filepath.Join(dir, "manifest.json")
	common := []string{"-csv", csv, "-provenance", prov, "-db", snap, "-template", tpl}
	if err := run(context.Background(), append(append([]string{}, common...), "-batch-id", "guard", "-manifest-out", path), nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*etl.Manifesto){
		"letra":     func(m *etl.Manifesto) { m.Itens[0].Letra = "Letra Sem Recibo" },
		"creditos":  func(m *etl.Manifesto) { s := "Autor Nunca Aceito"; m.Itens[0].Creditos = &s },
		"id":        func(m *etl.Manifesto) { m.Itens[0].HinoID = 99 },
		"fonte":     func(m *etl.Manifesto) { m.Itens[0].FonteID = strings.Repeat("f", 64) },
		"versao":    func(m *etl.Manifesto) { m.Meta.Versao = 2 },
		"duplicado": func(m *etl.Manifesto) { m.Itens = append(m.Itens, m.Itens[0]) },
		"omitido":   func(m *etl.Manifesto) { m.Itens = nil },
	} {
		t.Run(name, func(t *testing.T) {
			var m etl.Manifesto
			if err := json.Unmarshal(original, &m); err != nil {
				t.Fatal(err)
			}
			mutate(&m)
			changed, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(t.TempDir(), "changed.json")
			if err := os.WriteFile(p, changed, 0o600); err != nil {
				t.Fatal(err)
			}
			export := filepath.Join(t.TempDir(), "sql", "import.sql")
			args := append(append([]string{}, common...), "-manifest", p, "-check", "all", "-sql-out", export)
			if err := run(context.Background(), args, nil, nil, nil); err == nil {
				t.Fatal("manifesto divergente foi consumido/exportado")
			}
			if _, err := os.Stat(export); !os.IsNotExist(err) {
				t.Fatalf("SQL publicado para manifesto divergente: %v", err)
			}
		})
	}
}
