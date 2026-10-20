package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/etl"
)

func criarSnapshotTeste(t *testing.T, comLetra bool, revisado bool) string {
	t.Helper()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "snapshot.db")

	conn, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("abrir banco para snapshot: %v", err)
	}

	ddl := `
	CREATE TABLE coletanea (
		id     INTEGER PRIMARY KEY AUTOINCREMENT,
		codigo TEXT NOT NULL,
		titulo TEXT NOT NULL
	);
	CREATE TABLE hino (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		coletanea_id INTEGER NOT NULL REFERENCES coletanea(id),
		numeracao    INTEGER,
		titulo       TEXT NOT NULL,
		letra        TEXT,
		creditos     TEXT,
		revisado     INTEGER NOT NULL DEFAULT 0
	);
	`
	if _, err := conn.Exec(ddl); err != nil {
		conn.Close()
		t.Fatalf("criar tabelas para snapshot: %v", err)
	}

	res, err := conn.Exec(`INSERT INTO coletanea (codigo, titulo) VALUES ('CC', 'Cantor Cristão')`)
	if err != nil {
		conn.Close()
		t.Fatalf("inserir coletanea: %v", err)
	}
	colID, err := res.LastInsertId()
	if err != nil {
		conn.Close()
		t.Fatalf("last insert id: %v", err)
	}

	revInt := 0
	if revisado {
		revInt = 1
	}

	if comLetra {
		_, err = conn.Exec(
			`INSERT INTO hino (coletanea_id, numeracao, titulo, letra, revisado) VALUES (?, ?, ?, ?, ?)`,
			colID, 42, "Ó Vem", "    Ó Vem\n", revInt,
		)
	} else {
		_, err = conn.Exec(
			`INSERT INTO hino (coletanea_id, numeracao, titulo, letra, revisado) VALUES (?, ?, ?, NULL, ?)`,
			colID, 42, "Ó Vem", revInt,
		)
	}
	if err != nil {
		conn.Close()
		t.Fatalf("inserir hino: %v", err)
	}

	// Força modo DELETE e checkpoint para garantir snapshot sem WAL
	if _, err := conn.Exec("PRAGMA journal_mode = DELETE;"); err != nil {
		conn.Close()
		t.Fatalf("configurar journal_mode DELETE: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("fechar banco: %v", err)
	}

	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	return dbPath
}

func criarCSVTeste(t *testing.T, dir, conteudo string) string {
	t.Helper()
	csvPath := filepath.Join(dir, "sanitized.csv")
	header := "pasta,arquivo,hinario,numero,titulo,letra,status,observacao,creditos,source_id,source_sha256\n"
	if err := os.WriteFile(csvPath, []byte(header+conteudo), 0o644); err != nil {
		t.Fatalf("criar CSV: %v", err)
	}
	return csvPath
}

func criarProvenanceTeste(t *testing.T, dir, sourceID, sourceSHA, finalSHA, codigo string, numero int, titulo, letra string, creditos *string) string {
	t.Helper()
	provPath := filepath.Join(dir, "provenance.jsonl")
	recibo := map[string]any{
		"version":       1,
		"source_id":     sourceID,
		"source_sha256": sourceSHA,
		"state":         "OK",
		"codigo":        codigo,
		"numero":        numero,
		"titulo":        titulo,
		"letra":         letra,
		"creditos":      creditos,
		"final_sha256":  finalSHA,
	}
	data, err := json.Marshal(recibo)
	if err != nil {
		t.Fatalf("marshal provenance: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(provPath, data, 0o644); err != nil {
		t.Fatalf("criar provenance: %v", err)
	}
	return provPath
}

func templateReal(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "data", "templates", "default.pptx")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("template real não encontrado em %s: %v", p, err)
	}
	return p
}

func TestCLI(t *testing.T) {
	ctx := context.Background()

	t.Run("NormalizarStdin", func(t *testing.T) {
		var out, errOut bytes.Buffer
		in := strings.NewReader("    Ó VEM\r\n\r\nJá em Caixa Mista")
		err := run(ctx, []string{"-normalizar-stdin"}, in, &out, &errOut)
		if err != nil {
			t.Fatalf("run -normalizar-stdin falhou: %v", err)
		}
		esperado := "    Ó Vem\n\nJá em Caixa Mista"
		if out.String() != esperado {
			t.Fatalf("saída da normalização esperada %q, obtido %q", esperado, out.String())
		}
		if errOut.Len() > 0 {
			t.Fatalf("saída inesperada em errOut: %q", errOut.String())
		}
	})

	t.Run("HelpSemFlags", func(t *testing.T) {
		var out, errOut bytes.Buffer
		err := run(ctx, []string{}, nil, &out, &errOut)
		if err != nil {
			t.Fatalf("run sem flags deveria ter exit 0 (retornar nil), erro: %v", err)
		}
		if !strings.Contains(out.String(), "Usage") && !strings.Contains(out.String(), "normalizar-stdin") {
			t.Fatalf("esperava mensagem de ajuda em out, obtido:\n%s", out.String())
		}
	})

	t.Run("PrepararEExecutarManifestoSemAlterarSnapshot", func(t *testing.T) {
		workDir := t.TempDir()
		snapPath := criarSnapshotTeste(t, false, false)
		tpl := templateReal(t)

		sourceID := strings.Repeat("1", 64)
		sourceSHA := strings.Repeat("a", 64)
		finalSHA, err := etl.HashFinal("CC", 42, "Ó Vem", "    Ó Vem\n", nil)
		if err != nil {
			t.Fatal(err)
		}

		csvRow := fmt.Sprintf("p,a.pptx,CC,42,Ó Vem,\"    Ó Vem\n\",OK,obs,,%s,%s\n", sourceID, sourceSHA)
		csvPath := criarCSVTeste(t, workDir, csvRow)
		provPath := criarProvenanceTeste(t, workDir, sourceID, sourceSHA, finalSHA, "CC", 42, "Ó Vem", "    Ó Vem\n", nil)
		manifestOut := filepath.Join(workDir, "manifest.json")

		snapBytesAntes, err := os.ReadFile(snapPath)
		if err != nil {
			t.Fatal(err)
		}

		// 1. Preparar manifesto
		var outPrep, errPrep bytes.Buffer
		prepArgs := []string{
			"-csv", csvPath,
			"-db", snapPath,
			"-batch-id", "batch-test-1",
			"-provenance", provPath,
			"-manifest-out", manifestOut,
			"-template", tpl,
		}
		if err := run(ctx, prepArgs, nil, &outPrep, &errPrep); err != nil {
			t.Fatalf("preparar manifesto falhou: %v, errOut: %s", err, errPrep.String())
		}

		if _, err := os.Stat(manifestOut); err != nil {
			t.Fatalf("arquivo de manifesto não foi criado: %v", err)
		}

		snapBytesAposPrep, err := os.ReadFile(snapPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(snapBytesAntes, snapBytesAposPrep) {
			t.Fatal("snapshot foi modificado durante preparação do manifesto")
		}

		// 2. Executar manifesto com -check all, -validar e -sql-out em uma única chamada
		sqlOutTarget := filepath.Join(workDir, "sql_batch", "import.sql")
		var outConsumo, errConsumo bytes.Buffer
		consumoArgs := []string{
			"-manifest", manifestOut,
			"-csv", csvPath,
			"-provenance", provPath,
			"-db", snapPath,
			"-template", tpl,
			"-check", "all",
			"-validar",
			"-sql-out", sqlOutTarget,
		}
		if err := run(ctx, consumoArgs, nil, &outConsumo, &errConsumo); err != nil {
			t.Fatalf("consumo do manifesto falhou: %v, errOut: %s", err, errConsumo.String())
		}

		saida := outConsumo.String()
		if !strings.Contains(saida, "CC/42: 1 blocos") {
			t.Fatalf("saída não contém inspeção do -check all: %s", saida)
		}
		if !strings.Contains(saida, "OK CC/42") || !strings.Contains(saida, "todos os pacotes íntegros") {
			t.Fatalf("saída não contém validação de slides: %s", saida)
		}
		if !strings.Contains(saida, "SQL exportado com sucesso") {
			t.Fatalf("saída não contém confirmação de exportação SQL: %s", saida)
		}

		// Confere arquivos irmãos gerados
		for _, name := range []string{"import.sql", "rollback.sql", "verify.sql"} {
			fPath := filepath.Join(workDir, "sql_batch", name)
			fi, err := os.Stat(fPath)
			if err != nil || fi.Size() == 0 {
				t.Fatalf("arquivo %s não gerado ou vazio: %v", name, err)
			}
		}

		// Snapshot não pode ter sido modificado
		snapBytesAposConsumo, err := os.ReadFile(snapPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(snapBytesAntes, snapBytesAposConsumo) {
			t.Fatal("snapshot foi modificado durante consumo do manifesto")
		}

		// Snapshot não pode ter sido migrado (tabela import_batch não deve existir)
		rawConn, err := sql.Open("sqlite", snapPath)
		if err != nil {
			t.Fatal(err)
		}
		defer rawConn.Close()
		var n int
		if err := rawConn.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='import_batch'").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatal("Migrate foi executado no snapshot (import_batch existe)")
		}
	})

	t.Run("ZeroProntosNaoProduzArtefatoEnganoso", func(t *testing.T) {
		workDir := t.TempDir()
		// Snapshot com hino já revisado
		snapPath := criarSnapshotTeste(t, true, true)
		tpl := templateReal(t)

		sourceID := strings.Repeat("2", 64)
		sourceSHA := strings.Repeat("b", 64)
		finalSHA, err := etl.HashFinal("CC", 42, "Ó Vem", "    Ó Vem\n", nil)
		if err != nil {
			t.Fatal(err)
		}

		csvRow := fmt.Sprintf("p,a.pptx,CC,42,Ó Vem,\"    Ó Vem\n\",OK,obs,,%s,%s\n", sourceID, sourceSHA)
		csvPath := criarCSVTeste(t, workDir, csvRow)
		provPath := criarProvenanceTeste(t, workDir, sourceID, sourceSHA, finalSHA, "CC", 42, "Ó Vem", "    Ó Vem\n", nil)
		manifestOut := filepath.Join(workDir, "manifest-zero.json")

		var out, errOut bytes.Buffer
		prepArgs := []string{
			"-csv", csvPath,
			"-db", snapPath,
			"-batch-id", "batch-zero",
			"-provenance", provPath,
			"-manifest-out", manifestOut,
			"-template", tpl,
		}
		if err := run(ctx, prepArgs, nil, &out, &errOut); err != nil {
			t.Fatalf("esperava sucesso relatando zero elegíveis, erro: %v", err)
		}

		if _, err := os.Stat(manifestOut); err == nil {
			t.Fatal("manifesto não deveria ter sido criado com zero itens elegíveis")
		}
		if !strings.Contains(out.String(), "Nenhum hino elegível") {
			t.Fatalf("saída esperada informando zero elegíveis, obtido: %s", out.String())
		}
	})

	t.Run("FlagsInvalidas", func(t *testing.T) {
		var out, errOut bytes.Buffer
		// Flags legadas removidas devem falhar
		for _, f := range []string{"-force", "-yes", "-init", "-batch", "-dry-run=false"} {
			err := run(ctx, []string{f}, nil, &out, &errOut)
			if err == nil {
				t.Fatalf("flag legada %s deveria falhar", f)
			}
		}

		// Combinação inválida de -manifest e -manifest-out
		err := run(ctx, []string{"-manifest", "m.json", "-manifest-out", "out.json"}, nil, &out, &errOut)
		if err == nil {
			t.Fatal("combinar -manifest e -manifest-out deveria falhar")
		}
	})
}

func TestCLISymlink(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	// 1. Recusar data/hinos.db literal
	err := run(ctx, []string{"-csv", "x", "-db", "data/hinos.db", "-batch-id", "b", "-provenance", "p", "-manifest-out", "m.json"}, nil, nil, nil)
	if err == nil {
		t.Fatal("deveria recusar data/hinos.db")
	}

	// 2. Recusar caminho absoluto para data/hinos.db
	absProd, _ := filepath.Abs("data/hinos.db")
	err = run(ctx, []string{"-csv", "x", "-db", absProd, "-batch-id", "b", "-provenance", "p", "-manifest-out", "m.json"}, nil, nil, nil)
	if err == nil {
		t.Fatal("deveria recusar caminho absoluto para data/hinos.db")
	}

	// 3. Recusar symlink para data/hinos.db
	symlinkPath := filepath.Join(workDir, "symlink-prod.db")
	if err := os.Symlink(absProd, symlinkPath); err == nil {
		err = run(ctx, []string{"-csv", "x", "-db", symlinkPath, "-batch-id", "b", "-provenance", "p", "-manifest-out", "m.json"}, nil, nil, nil)
		if err == nil {
			t.Fatal("deveria recusar symlink para data/hinos.db")
		}
	}

	// 4. Recusar hardlink para data/hinos.db (se data/hinos.db existir)
	if _, errStat := os.Stat("data/hinos.db"); errStat == nil {
		hardlinkPath := filepath.Join("data", "hinos-test-hardlink.db")
		if err := os.Link("data/hinos.db", hardlinkPath); err == nil {
			defer os.Remove(hardlinkPath)
			err = run(ctx, []string{"-csv", "x", "-db", hardlinkPath, "-batch-id", "b", "-provenance", "p", "-manifest-out", "m.json"}, nil, nil, nil)
			if err == nil {
				t.Fatal("deveria recusar hardlink para data/hinos.db")
			}
		}
	}

	// 5. Recusar DSN file: e URLs remotas
	for _, url := range []string{
		"file:test.db",
		"libsql://demo.turso.io",
		"http://localhost:8080/db",
		"https://localhost:8080/db",
		"ws://localhost:8080/db",
		"wss://localhost:8080/db",
	} {
		err := run(ctx, []string{"-csv", "x", "-db", url, "-batch-id", "b", "-provenance", "p", "-manifest-out", "m.json"}, nil, nil, nil)
		if err == nil {
			t.Fatalf("deveria recusar url/dsn %s", url)
		}
	}

	// 6. Recusar banco inexistente
	err = run(ctx, []string{"-csv", "x", "-db", filepath.Join(workDir, "inexistente.db"), "-batch-id", "b", "-provenance", "p", "-manifest-out", "m.json"}, nil, nil, nil)
	if err == nil {
		t.Fatal("deveria recusar banco inexistente")
	}
}

func TestCLIHashMismatch(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	snapPath := criarSnapshotTeste(t, false, false)
	tpl := templateReal(t)

	sourceID := strings.Repeat("3", 64)
	sourceSHA := strings.Repeat("c", 64)
	finalSHA, err := etl.HashFinal("CC", 42, "Ó Vem", "    Ó Vem\n", nil)
	if err != nil {
		t.Fatal(err)
	}

	csvRow := fmt.Sprintf("p,a.pptx,CC,42,Ó Vem,\"    Ó Vem\n\",OK,obs,,%s,%s\n", sourceID, sourceSHA)
	csvPath := criarCSVTeste(t, workDir, csvRow)
	provPath := criarProvenanceTeste(t, workDir, sourceID, sourceSHA, finalSHA, "CC", 42, "Ó Vem", "    Ó Vem\n", nil)
	manifestPath := filepath.Join(workDir, "manifest.json")

	// Prepara manifesto válido
	prepArgs := []string{
		"-csv", csvPath,
		"-db", snapPath,
		"-batch-id", "batch-mismatch",
		"-provenance", provPath,
		"-manifest-out", manifestPath,
		"-template", tpl,
	}
	if err := run(ctx, prepArgs, nil, nil, nil); err != nil {
		t.Fatalf("preparar manifesto: %v", err)
	}

	// 1. Snapshot alterado
	t.Run("SnapshotAlterado", func(t *testing.T) {
		altDir := t.TempDir()
		altSnap := filepath.Join(altDir, "altered-snap.db")
		data, _ := os.ReadFile(snapPath)
		data = append(data, 0x00)
		_ = os.WriteFile(altSnap, data, 0o644)

		err := run(ctx, []string{
			"-manifest", manifestPath,
			"-csv", csvPath,
			"-provenance", provPath,
			"-db", altSnap,
			"-template", tpl,
			"-check", "all",
		}, nil, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "snapshot hash diverge") {
			t.Fatalf("esperava erro de divergência de snapshot, obtido: %v", err)
		}
	})

	// 2. CSV alterado
	t.Run("CSVAlterado", func(t *testing.T) {
		altDir := t.TempDir()
		altCSV := filepath.Join(altDir, "alt.csv")
		_ = os.WriteFile(altCSV, []byte("alterado\n"), 0o644)

		err := run(ctx, []string{
			"-manifest", manifestPath,
			"-csv", altCSV,
			"-provenance", provPath,
			"-db", snapPath,
			"-template", tpl,
			"-check", "all",
		}, nil, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "csv hash diverge") {
			t.Fatalf("esperava erro de divergência de csv, obtido: %v", err)
		}
	})

	// 3. Provenance alterado
	t.Run("ProvenanceAlterado", func(t *testing.T) {
		altDir := t.TempDir()
		altProv := filepath.Join(altDir, "alt.jsonl")
		_ = os.WriteFile(altProv, []byte("alterado\n"), 0o644)

		err := run(ctx, []string{
			"-manifest", manifestPath,
			"-csv", csvPath,
			"-provenance", altProv,
			"-db", snapPath,
			"-template", tpl,
			"-check", "all",
		}, nil, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "provenance hash diverge") {
			t.Fatalf("esperava erro de divergência de provenance, obtido: %v", err)
		}
	})

	// 4. Template alterado
	t.Run("TemplateAlterado", func(t *testing.T) {
		altDir := t.TempDir()
		altTpl := filepath.Join(altDir, "alt.pptx")
		_ = os.WriteFile(altTpl, []byte("fake pptx"), 0o644)

		err := run(ctx, []string{
			"-manifest", manifestPath,
			"-csv", csvPath,
			"-provenance", provPath,
			"-db", snapPath,
			"-template", altTpl,
			"-check", "all",
		}, nil, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "template hash diverge") {
			t.Fatalf("esperava erro de divergência de template, obtido: %v", err)
		}
	})
}

func TestCLIExportAtomic(t *testing.T) {
	parentDir := t.TempDir()
	targetDir := filepath.Join(parentDir, "sql_export")

	// 1. Recusar targetDir já existente (mesmo vazio)
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}

	stageDir, err := os.MkdirTemp(parentDir, ".sql_stage_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(stageDir)

	if err := os.WriteFile(filepath.Join(stageDir, "import.sql"), []byte("SELECT 1;"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := publicarSQL(stageDir, targetDir); err == nil {
		t.Fatal("publicarSQL deveria falhar se targetDir já existe")
	}

	// 2. Recusar targetDir que seja symlink
	_ = os.Remove(targetDir)
	fakeTarget := filepath.Join(parentDir, "nonexistent")
	if err := os.Symlink(fakeTarget, targetDir); err == nil {
		if err := publicarSQL(stageDir, targetDir); err == nil {
			t.Fatal("publicarSQL deveria falhar se targetDir é symlink")
		}
		_ = os.Remove(targetDir)
	}

	// 3. Sucesso atômico quando targetDir não existe
	if err := publicarSQL(stageDir, targetDir); err != nil {
		t.Fatalf("publicarSQL falhou: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "import.sql")); err != nil {
		t.Fatalf("arquivo import.sql não encontrado no destino publicado: %v", err)
	}
	// Lock deve ser liberado (não deve existir)
	if _, err := os.Stat(filepath.Join(parentDir, ".sql_export.lock")); err == nil {
		t.Fatal("arquivo de lock não foi removido após publicação")
	}

	// 4. Concorrência: dois exporters simultâneos disputando o mesmo targetDir
	t.Run("ConcorrenciaDoisExporters", func(t *testing.T) {
		pDir := t.TempDir()
		tDir := filepath.Join(pDir, "sql_concorrente")

		stage1, _ := os.MkdirTemp(pDir, ".stage1_*")
		stage2, _ := os.MkdirTemp(pDir, ".stage2_*")
		defer os.RemoveAll(stage1)
		defer os.RemoveAll(stage2)

		_ = os.WriteFile(filepath.Join(stage1, "import.sql"), []byte("-- 1"), 0o644)
		_ = os.WriteFile(filepath.Join(stage2, "import.sql"), []byte("-- 2"), 0o644)

		var wg sync.WaitGroup
		errs := make([]error, 2)
		wg.Add(2)

		go func() {
			defer wg.Done()
			errs[0] = publicarSQL(stage1, tDir)
		}()
		go func() {
			defer wg.Done()
			errs[1] = publicarSQL(stage2, tDir)
		}()

		wg.Wait()

		// Exatamente um deve ter sucesso e um deve falhar
		sucessos := 0
		falhas := 0
		for _, err := range errs {
			if err == nil {
				sucessos++
			} else {
				falhas++
			}
		}
		if sucessos != 1 || falhas != 1 {
			t.Fatalf("esperava exatamente 1 sucesso e 1 falha, obteve %d sucessos e %d falhas (errs: %v)", sucessos, falhas, errs)
		}
	})
}
