package etl_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/etl"
	"github.com/adjoli/louvores-go/internal/repository"
)

func bancoManifestoTeste(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(conn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := conn.Exec(`INSERT INTO coletanea (codigo, titulo) VALUES ('CC', 'Cantor Cristão')`); err != nil {
		t.Fatal(err)
	}
	// 1: sem letra, revisado=0 (elegível)
	// 2: com letra, revisado=1 (pulado)
	// 3: com letra, revisado=0 (pulado)
	// 4: letra com apenas espaços ASCII, revisado=0 (elegível)
	// 5: letra com NBSP (\u00a0), revisado=0 (pulado)
	if _, err := conn.Exec(`INSERT INTO hino (coletanea_id, numeracao, titulo, letra, revisado) VALUES
		(1, 1, 'Hino Um', NULL, 0),
		(1, 2, 'Hino Dois', 'Letra Antiga', 1),
		(1, 3, 'Hino Três', 'Letra Existente', 0),
		(1, 4, 'Hino Quatro', '   ' || char(9) || char(10), 0),
		(1, 5, 'Hino Cinco', char(160), 0)
	`); err != nil {
		t.Fatal(err)
	}
	return conn
}

func metaValida() etl.MetaManifesto {
	return etl.MetaManifesto{
		Versao:           1,
		BatchID:          "batch-001",
		SnapshotSHA256:   strings.Repeat("a", 64),
		CSVSHA256:        strings.Repeat("b", 64),
		ProvenanceSHA256: strings.Repeat("c", 64),
		TemplateSHA256:   strings.Repeat("d", 64),
	}
}

func TestHashFinal(t *testing.T) {
	// Shared test vectors from spec:
	// Vector 1: codigo CC, numero 42, titulo Ó Vem, letra "    Ó Vem\n", creditos nil
	h1, err := etl.HashFinal("CC", 42, "Ó Vem", "    Ó Vem\n", nil)
	if err != nil {
		t.Fatalf("HashFinal vetor 1 falhou: %v", err)
	}
	const want1 = "35459d763c853fcd19927927ab8850e0cd5344deb0ae035636507751b175b8ec"
	if h1 != want1 {
		t.Fatalf("vetor 1 hash divergente: got %s, want %s", h1, want1)
	}

	// Vector 2: same, with creditos pointer to ""
	emptyStr := ""
	h2, err := etl.HashFinal("CC", 42, "Ó Vem", "    Ó Vem\n", &emptyStr)
	if err != nil {
		t.Fatalf("HashFinal vetor 2 falhou: %v", err)
	}
	const want2 = "5bb4297d9baea87d0c749281e41b7e103f0dc1f96ab4d2fc6e69a4e4e7115f93"
	if h2 != want2 {
		t.Fatalf("vetor 2 hash divergente: got %s, want %s", h2, want2)
	}

	// Vector 3: with non-empty creditos
	cred := "Autor"
	h3, err := etl.HashFinal("CC", 42, "Ó Vem", "    Ó Vem\n", &cred)
	if err != nil {
		t.Fatalf("HashFinal com créditos falhou: %v", err)
	}
	if h3 == h1 || h3 == h2 {
		t.Fatalf("hash com créditos deveria ser distinto: got %s", h3)
	}

	// Rejection of NUL bytes
	if _, err := etl.HashFinal("C\x00C", 1, "T", "L", nil); err == nil {
		t.Fatal("esperava erro para NUL em codigo")
	}
	if _, err := etl.HashFinal("CC", 1, "T\x00itulo", "L", nil); err == nil {
		t.Fatal("esperava erro para NUL em titulo")
	}
	if _, err := etl.HashFinal("CC", 1, "T", "L\x00etra", nil); err == nil {
		t.Fatal("esperava erro para NUL em letra")
	}
	nulStr := "C\x00red"
	if _, err := etl.HashFinal("CC", 1, "T", "L", &nulStr); err == nil {
		t.Fatal("esperava erro para NUL em creditos")
	}

	// Rejection of invalid number
	if _, err := etl.HashFinal("CC", 0, "T", "L", nil); err == nil {
		t.Fatal("esperava erro para numero 0")
	}
	if _, err := etl.HashFinal("CC", -5, "T", "L", nil); err == nil {
		t.Fatal("esperava erro para numero negativo")
	}
}

func TestManifestoElegibilidade(t *testing.T) {
	conn := bancoManifestoTeste(t)
	ctx := context.Background()

	shaA := strings.Repeat("1", 64)
	shaB := strings.Repeat("2", 64)

	linhas := []etl.Linha{
		{Arquivo: "1.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: "Estrofe Um\nLinha dois", Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB},
		{Arquivo: "2.pptx", Coletanea: "CC", Numero: 2, Titulo: "Hino Dois", Letra: "Estrofe Dois", Status: "OK", NumLinha: 3, FonteID: strings.Repeat("3", 64), FonteSHA256: shaB},
		{Arquivo: "3.pptx", Coletanea: "CC", Numero: 3, Titulo: "Hino Três", Letra: "Estrofe Três", Status: "OK", NumLinha: 4, FonteID: strings.Repeat("4", 64), FonteSHA256: shaB},
		{Arquivo: "4.pptx", Coletanea: "CC", Numero: 4, Titulo: "Hino Quatro", Letra: "Estrofe Quatro\nLinha quatro", Status: "OK", NumLinha: 5, FonteID: strings.Repeat("5", 64), FonteSHA256: shaB},
		{Arquivo: "5.pptx", Coletanea: "CC", Numero: 5, Titulo: "Hino Cinco", Letra: "Estrofe Cinco", Status: "OK", NumLinha: 6, FonteID: strings.Repeat("6", 64), FonteSHA256: shaB},
	}

	m, err := etl.PrepararManifesto(ctx, conn, linhas, metaValida())
	if err != nil {
		t.Fatalf("PrepararManifesto falhou: %v", err)
	}

	// Apenas hinos 1 (NULL) e 4 (espaço ASCII) são elegíveis; 2 (revisado), 3 (já tem letra) e 5 (NBSP) são pulados.
	if len(m.Itens) != 2 {
		t.Fatalf("esperava 2 itens no manifesto, vieram %d: %+v", len(m.Itens), m.Itens)
	}
	if m.Itens[0].Numero != 1 || m.Itens[1].Numero != 4 {
		t.Fatalf("itens inesperados no manifesto: %+v", m.Itens)
	}

	// Chave duplicada no CSV bloqueia todo o manifesto
	linhasDup := append([]etl.Linha(nil), linhas...)
	linhasDup = append(linhasDup, etl.Linha{
		Arquivo: "1b.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: "Outra Letra", Status: "OK", NumLinha: 7, FonteID: strings.Repeat("7", 64), FonteSHA256: shaB,
	})
	if _, err := etl.PrepararManifesto(ctx, conn, linhasDup, metaValida()); err == nil {
		t.Fatal("chave duplicada no CSV deveria bloquear todo o manifesto")
	}

	// Chave ausente no banco bloqueia todo o manifesto
	linhasAusente := []etl.Linha{
		{Arquivo: "999.pptx", Coletanea: "CC", Numero: 999, Titulo: "Fantasma", Letra: "Letra", Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB},
	}
	if _, err := etl.PrepararManifesto(ctx, conn, linhasAusente, metaValida()); err == nil {
		t.Fatal("chave ausente no banco deveria bloquear todo o manifesto")
	}

	// Coletânea inexistente bloqueia todo o manifesto
	linhasColInex := []etl.Linha{
		{Arquivo: "1.pptx", Coletanea: "XX", Numero: 1, Titulo: "Inexistente", Letra: "Letra", Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB},
	}
	if _, err := etl.PrepararManifesto(ctx, conn, linhasColInex, metaValida()); err == nil {
		t.Fatal("coletânea inexistente deveria bloquear todo o manifesto")
	}

	// Título divergente bloqueia todo o manifesto
	linhasTituloDiv := []etl.Linha{
		{Arquivo: "1.pptx", Coletanea: "CC", Numero: 1, Titulo: "Título Divergente", Letra: "Letra Um", Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB},
	}
	if _, err := etl.PrepararManifesto(ctx, conn, linhasTituloDiv, metaValida()); err == nil {
		t.Fatal("título divergente deveria bloquear todo o manifesto")
	}
}

func TestManifestoDeterministico(t *testing.T) {
	conn := bancoManifestoTeste(t)
	ctx := context.Background()

	shaA := strings.Repeat("1", 64)
	shaB := strings.Repeat("2", 64)
	shaC := strings.Repeat("3", 64)
	emptyStr := ""

	l1 := etl.Linha{Arquivo: "1.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: "Estrofe Um\nLinha dois", Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB, Creditos: nil}
	l4 := etl.Linha{Arquivo: "4.pptx", Coletanea: "CC", Numero: 4, Titulo: "Hino Quatro", Letra: "Estrofe Quatro\nLinha quatro", Status: "OK", NumLinha: 3, FonteID: shaC, FonteSHA256: shaB, Creditos: &emptyStr}

	// Ordem 1: 1 depois 4
	m1, err := etl.PrepararManifesto(ctx, conn, []etl.Linha{l1, l4}, metaValida())
	if err != nil {
		t.Fatal(err)
	}

	// Ordem 2: 4 depois 1
	m2, err := etl.PrepararManifesto(ctx, conn, []etl.Linha{l4, l1}, metaValida())
	if err != nil {
		t.Fatal(err)
	}

	// Ambos manifestos canônicos devem ter itens na mesma ordem: CC/1, depois CC/4
	if len(m1.Itens) != 2 || len(m2.Itens) != 2 {
		t.Fatalf("esperava 2 itens, vieram m1=%d, m2=%d", len(m1.Itens), len(m2.Itens))
	}
	if m1.Itens[0].Numero != 1 || m1.Itens[1].Numero != 4 || m2.Itens[0].Numero != 1 || m2.Itens[1].Numero != 4 {
		t.Fatalf("ordem incorreta: m1=%+v, m2=%+v", m1.Itens, m2.Itens)
	}

	// Salvar e ler manifesto: digest idêntico e preservação de nil/vazio
	tmpDir := t.TempDir()
	path1 := filepath.Join(tmpDir, "manifesto1.json")
	path2 := filepath.Join(tmpDir, "manifesto2.json")

	if err := etl.SalvarManifesto(path1, m1); err != nil {
		t.Fatalf("SalvarManifesto falhou: %v", err)
	}
	if err := etl.SalvarManifesto(path2, m2); err != nil {
		t.Fatalf("SalvarManifesto falhou: %v", err)
	}

	lido1, digest1, err := etl.LerManifesto(path1)
	if err != nil {
		t.Fatalf("LerManifesto falhou: %v", err)
	}
	_, digest2, err := etl.LerManifesto(path2)
	if err != nil {
		t.Fatalf("LerManifesto falhou: %v", err)
	}

	if digest1 != digest2 {
		t.Fatalf("digests deveriam ser idênticos: %s != %s", digest1, digest2)
	}

	// Preservação de nil e string vazia nos créditos
	if lido1.Itens[0].Creditos != nil {
		t.Fatalf("item 0 deveria ter Creditos=nil, veio: %v", *lido1.Itens[0].Creditos)
	}
	if lido1.Itens[1].Creditos == nil || *lido1.Itens[1].Creditos != "" {
		t.Fatalf("item 1 deveria ter Creditos=&\"\", veio: %v", lido1.Itens[1].Creditos)
	}

	// Metadata ou hash alterado muda digest
	metaMod := metaValida()
	metaMod.BatchID = "batch-outro"
	mMod, err := etl.PrepararManifesto(ctx, conn, []etl.Linha{l1, l4}, metaMod)
	if err != nil {
		t.Fatal(err)
	}
	pathMod := filepath.Join(tmpDir, "manifesto_mod.json")
	if err := etl.SalvarManifesto(pathMod, mMod); err != nil {
		t.Fatal(err)
	}
	_, digestMod, err := etl.LerManifesto(pathMod)
	if err != nil {
		t.Fatal(err)
	}
	if digestMod == digest1 {
		t.Fatalf("digest deveria mudar com metadata alterado: %s", digestMod)
	}
}

func TestCheckCandidato(t *testing.T) {
	conn := bancoManifestoTeste(t)
	ctx := context.Background()

	shaA := strings.Repeat("1", 64)
	shaB := strings.Repeat("2", 64)

	letraComDoisBlocos := "Estrofe Um\nSegunda linha\n\n    Refrão Um\n    Segunda linha do refrão"
	linhas := []etl.Linha{
		{Arquivo: "1.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: letraComDoisBlocos, Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB},
	}

	m, err := etl.PrepararManifesto(ctx, conn, linhas, metaValida())
	if err != nil {
		t.Fatalf("PrepararManifesto falhou: %v", err)
	}

	// Inspecionar alvo específico CC/1
	out, err := etl.InspecionarManifesto(m, "CC/1")
	if err != nil {
		t.Fatalf("InspecionarManifesto falhou: %v", err)
	}
	if !strings.Contains(out, "2 blocos") || !strings.Contains(out, "[estrofe]") || !strings.Contains(out, "[refrao]") {
		t.Fatalf("inspeção inesperada:\n%s", out)
	}

	// Inspecionar com alvo "all"
	outAll, err := etl.InspecionarManifesto(m, "all")
	if err != nil {
		t.Fatalf("InspecionarManifesto 'all' falhou: %v", err)
	}
	if !strings.Contains(outAll, "CC/1: 2 blocos") {
		t.Fatalf("inspeção 'all' inesperada:\n%s", outAll)
	}

	// Alvo inexistente no manifesto
	if _, err := etl.InspecionarManifesto(m, "CC/999"); !errors.Is(err, repository.ErrHinoNotFound) {
		t.Fatalf("esperava ErrHinoNotFound para alvo inexistente no manifesto, veio: %v", err)
	}

	// Formato de alvo inválido
	if _, err := etl.InspecionarManifesto(m, "invalido"); err == nil {
		t.Fatal("esperava erro para formato de alvo inválido")
	}

	// Sem escrita: banco segue intacto com letra NULL em CC/1
	var letraNoBanco *string
	if err := conn.QueryRow(`SELECT letra FROM hino WHERE numeracao = 1`).Scan(&letraNoBanco); err != nil {
		t.Fatal(err)
	}
	if letraNoBanco != nil {
		t.Fatalf("banco não deveria ter sido alterado, letra = %v", *letraNoBanco)
	}
}

func TestManifestoNaoNormalizaDepois(t *testing.T) {
	conn := bancoManifestoTeste(t)
	ctx := context.Background()

	shaA := strings.Repeat("1", 64)
	shaB := strings.Repeat("2", 64)

	// 1. Letra que não é idempotente com TitularLetra (ex.: caixa baixa em início de verso)
	letraNaoTitulada := "estrofe um minúscula\nsegunda linha"
	linhas1 := []etl.Linha{
		{Arquivo: "1.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: letraNaoTitulada, Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB},
	}
	if _, err := etl.PrepararManifesto(ctx, conn, linhas1, metaValida()); err == nil {
		t.Fatal("esperava erro quando letra não está previamente normalizada com TitularLetra")
	}

	// 2. Letra com quebra CRLF (não normalizada)
	letraCRLF := "Estrofe Um\r\nSegunda Linha"
	linhas2 := []etl.Linha{
		{Arquivo: "1.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: letraCRLF, Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB},
	}
	if _, err := etl.PrepararManifesto(ctx, conn, linhas2, metaValida()); err == nil {
		t.Fatal("esperava erro para quebra CRLF")
	}

	// 3. Linha com mais de 64 runas (excluído recuo de refrão)
	// Estrofe: 65 runas -> falha
	letra65Runas := "A" + strings.Repeat("a", 64)
	linhas3 := []etl.Linha{
		{Arquivo: "1.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: letra65Runas, Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB},
	}
	if _, err := etl.PrepararManifesto(ctx, conn, linhas3, metaValida()); err == nil {
		t.Fatal("esperava erro para linha com 65 runas")
	}

	// Estrofe: exatamente 64 runas -> sucesso
	letra64Runas := "A" + strings.Repeat("a", 63)
	linhas4 := []etl.Linha{
		{Arquivo: "1.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: letra64Runas, Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB},
	}
	if _, err := etl.PrepararManifesto(ctx, conn, linhas4, metaValida()); err != nil {
		t.Fatalf("linha com 64 runas deveria passar: %v", err)
	}

	// Refrão: 4 espaços + 64 runas -> sucesso (recuo ignorado no limite)
	letraRefrao64 := "    " + "B" + strings.Repeat("b", 63)
	linhas5 := []etl.Linha{
		{Arquivo: "1.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: letraRefrao64, Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB},
	}
	if _, err := etl.PrepararManifesto(ctx, conn, linhas5, metaValida()); err != nil {
		t.Fatalf("refrão com 4 espaços + 64 runas deveria passar: %v", err)
	}

	// Refrão: 4 espaços + 65 runas -> falha
	letraRefrao65 := "    " + "B" + strings.Repeat("b", 64)
	linhas6 := []etl.Linha{
		{Arquivo: "1.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: letraRefrao65, Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB},
	}
	if _, err := etl.PrepararManifesto(ctx, conn, linhas6, metaValida()); err == nil {
		t.Fatal("esperava erro para refrão com 65 runas úteis")
	}

	// 4. Bloco com mais de 10 linhas
	linhas11 := make([]string, 11)
	for i := range linhas11 {
		linhas11[i] = fmt.Sprintf("Verso %d", i+1)
	}
	letra11Linhas := strings.Join(linhas11, "\n")
	linhas7 := []etl.Linha{
		{Arquivo: "1.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: letra11Linhas, Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB},
	}
	if _, err := etl.PrepararManifesto(ctx, conn, linhas7, metaValida()); err == nil {
		t.Fatal("esperava erro para bloco com 11 linhas")
	}

	// Bloco com exatamente 10 linhas -> sucesso
	letra10Linhas := strings.Join(linhas11[:10], "\n")
	linhas8 := []etl.Linha{
		{Arquivo: "1.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: letra10Linhas, Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB},
	}
	if _, err := etl.PrepararManifesto(ctx, conn, linhas8, metaValida()); err != nil {
		t.Fatalf("bloco com 10 linhas deveria passar: %v", err)
	}
}

func TestValidarRecibos(t *testing.T) {
	shaA := strings.Repeat("1", 64)
	shaB := strings.Repeat("2", 64)
	letraValida := "Estrofe Um\nLinha dois"
	hFinal, err := etl.HashFinal("CC", 1, "Hino Um", letraValida, nil)
	if err != nil {
		t.Fatal(err)
	}

	reciboOK := map[string]interface{}{
		"version":       1,
		"source_id":     shaA,
		"source_sha256": shaB,
		"state":         "OK",
		"codigo":        "CC",
		"numero":        1,
		"titulo":        "Hino Um",
		"letra":         letraValida,
		"creditos":      nil,
		"final_sha256":  hFinal,
	}

	reciboBytes, err := json.Marshal(reciboOK)
	if err != nil {
		t.Fatal(err)
	}

	provFile := filepath.Join(t.TempDir(), "provenance.jsonl")
	if err := os.WriteFile(provFile, append(reciboBytes, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	linhas := []etl.Linha{
		{Arquivo: "1.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: letraValida, Status: "OK", NumLinha: 2, FonteID: shaA, FonteSHA256: shaB, Creditos: nil},
		{Arquivo: "2.pptx", Coletanea: "CC", Numero: 2, Titulo: "Hino Dois", Letra: "L", Status: "PENDENTE", NumLinha: 3},
	}

	// 1. Sucesso com recibo válido correspondente
	if err := etl.ValidarRecibos(linhas, provFile); err != nil {
		t.Fatalf("ValidarRecibos falhou: %v", err)
	}

	// 2. Modificar título após recibo falha
	linhasModTitulo := append([]etl.Linha(nil), linhas...)
	linhasModTitulo[0].Titulo = "Outro Título"
	if err := etl.ValidarRecibos(linhasModTitulo, provFile); err == nil {
		t.Fatal("esperava erro ao modificar título após recibo")
	}

	// 3. Modificar créditos após recibo falha
	cred := "Novo Autor"
	linhasModCred := append([]etl.Linha(nil), linhas...)
	linhasModCred[0].Creditos = &cred
	if err := etl.ValidarRecibos(linhasModCred, provFile); err == nil {
		t.Fatal("esperava erro ao modificar créditos após recibo")
	}

	// 4. Modificar chave (número) após recibo falha
	linhasModNum := append([]etl.Linha(nil), linhas...)
	linhasModNum[0].Numero = 99
	if err := etl.ValidarRecibos(linhasModNum, provFile); err == nil {
		t.Fatal("esperava erro ao modificar número após recibo")
	}

	// 5. Modificar fonte_sha256 após recibo falha
	linhasModSHA := append([]etl.Linha(nil), linhas...)
	linhasModSHA[0].FonteSHA256 = strings.Repeat("f", 64)
	if err := etl.ValidarRecibos(linhasModSHA, provFile); err == nil {
		t.Fatal("esperava erro ao modificar fonte_sha256 após recibo")
	}

	// 6. Recibo com state diferente de OK falha para linha OK
	reciboDoubt := map[string]interface{}{
		"version":       1,
		"source_id":     shaA,
		"source_sha256": shaB,
		"state":         "doubt",
		"codigo":        "CC",
		"numero":        1,
		"titulo":        "Hino Um",
		"letra":         letraValida,
		"creditos":      nil,
		"final_sha256":  nil,
	}
	bytesDoubt, _ := json.Marshal(reciboDoubt)
	provDoubt := filepath.Join(t.TempDir(), "provenance_doubt.jsonl")
	_ = os.WriteFile(provDoubt, append(bytesDoubt, '\n'), 0o644)
	if err := etl.ValidarRecibos(linhas, provDoubt); err == nil {
		t.Fatal("esperava erro para recibo com state='doubt'")
	}

	// 7. Reutilização duplicada do mesmo recibo falha
	linhasDupRec := append([]etl.Linha(nil), linhas...)
	linhasDupRec = append(linhasDupRec, etl.Linha{
		Arquivo: "1b.pptx", Coletanea: "CC", Numero: 1, Titulo: "Hino Um", Letra: letraValida, Status: "OK", NumLinha: 4, FonteID: shaA, FonteSHA256: shaB,
	})
	if err := etl.ValidarRecibos(linhasDupRec, provFile); err == nil {
		t.Fatal("esperava erro para recibo reutilizado em mais de uma linha")
	}
}

func TestSalvarManifesto_NaoSobrescreve(t *testing.T) {
	p := filepath.Join(t.TempDir(), "manifesto.json")
	if err := os.WriteFile(p, []byte("existente"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &etl.Manifesto{Meta: metaValida(), Itens: []etl.ItemManifesto{}}
	err := etl.SalvarManifesto(p, m)
	if err == nil || !errors.Is(err, os.ErrExist) {
		t.Fatalf("esperava os.ErrExist ao tentar sobrescrever, veio: %v", err)
	}
}
