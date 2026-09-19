package etl

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
)

func bancoTeste(t *testing.T) *sql.DB {
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
	// CC/1 sem letra, CC/2 com letra e revisado.
	if _, err := conn.Exec(`INSERT INTO hino (coletanea_id, numeracao, titulo, letra, revisado)
		VALUES (1, 1, 'Sem Letra', NULL, 0), (1, 2, 'Com Letra', 'Letra Antiga', 1)`); err != nil {
		t.Fatal(err)
	}
	return conn
}

func csvTemp(t *testing.T, conteudo string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "revisao.csv")
	if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const cabecalho = "pasta,arquivo,hinario,numero,titulo,letra,status,observacao\n"

func TestLerCSV_OKePulados(t *testing.T) {
	p := csvTemp(t, cabecalho+
		`a,um.pptx,CC,1,T1,"Linha Um",OK,`+"\n"+
		`a,dois.pptx,CC,2,T2,"Linha Dois",PENDENTE,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(linhas) != 2 {
		t.Fatalf("linhas = %d, esperado 2", len(linhas))
	}
	if linhas[0].Colet != "CC" || linhas[0].Numero != 1 || linhas[0].NumLinha != 2 {
		t.Fatalf("linha 1 mal parseada: %+v", linhas[0])
	}
}

func TestLerCSV_ColunaAusente(t *testing.T) {
	p := csvTemp(t, "arquivo,hinario,numero\num.pptx,CC,1\n")
	if _, err := LerCSV(p); err == nil || !strings.Contains(err.Error(), "ausente") {
		t.Fatalf("esperava erro de coluna ausente, veio %v", err)
	}
}

func TestPlanejar_ClassificaENovosModificadosErros(t *testing.T) {
	conn := bancoTeste(t)
	p := csvTemp(t, cabecalho+
		`a,um.pptx,CC,1,T1,"Nova Letra",OK,`+"\n"+
		`a,dois.pptx,CC,2,T2,"Outra Letra",OK,`+"\n"+
		`a,nove.pptx,CC,9,T9,"Fantasma",OK,`+"\n"+
		`a,skip.pptx,CC,1,T1,"X",PENDENTE,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	plano, err := Planejar(conn, linhas)
	if err != nil {
		t.Fatal(err)
	}
	if len(plano.Prontos) != 2 || plano.Pulados != 1 || plano.Erros != 1 {
		t.Fatalf("plano = %+v", plano)
	}
	novos, mods := 0, 0
	for _, it := range plano.Prontos {
		if it.D.TemLetra {
			mods++
		} else {
			novos++
		}
	}
	if novos != 1 || mods != 1 {
		t.Fatalf("novos=%d modificados=%d, esperado 1 e 1", novos, mods)
	}
}

func TestImportar_EscreveTitleCaseERevisado(t *testing.T) {
	conn := bancoTeste(t)
	p := csvTemp(t, cabecalho+`a,um.pptx,CC,1,T1,"OH! MARAVILHA MAIUSCULA",OK,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	plano, err := Planejar(conn, linhas)
	if err != nil {
		t.Fatal(err)
	}
	n, err := Importar(conn, plano, 100, nil)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	var letra string
	var rev bool
	if err := conn.QueryRow(`SELECT letra, revisado FROM hino WHERE numeracao = 1`).Scan(&letra, &rev); err != nil {
		t.Fatal(err)
	}
	if letra != "Oh! Maravilha Maiuscula" || !rev {
		t.Fatalf("letra=%q revisado=%v", letra, rev)
	}
}

func TestImportar_RollbackDoLoteComFalha(t *testing.T) {
	conn := bancoTeste(t)
	p := csvTemp(t, cabecalho+
		`a,um.pptx,CC,1,T1,"Letra Um",OK,`+"\n"+
		`a,dois.pptx,CC,2,T2,"Letra Dois",OK,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	plano, err := Planejar(conn, linhas)
	if err != nil {
		t.Fatal(err)
	}
	// Sabota o segundo alvo depois do plano: some com o hino.
	if _, err := conn.Exec(`DELETE FROM hino WHERE numeracao = 2`); err != nil {
		t.Fatal(err)
	}
	if _, err := Importar(conn, plano, 1, nil); err == nil {
		t.Fatal("esperava erro no lote 2")
	}
	// Lote 1 (CC/1) commitado, lote 2 revertido: CC/2 segue inexistente.
	var letra string
	if err := conn.QueryRow(`SELECT letra FROM hino WHERE numeracao = 1`).Scan(&letra); err != nil {
		t.Fatal(err)
	}
	if letra != "Letra Um" {
		t.Fatalf("lote 1 deveria estar commitado, letra=%q", letra)
	}
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM hino WHERE numeracao = 2`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("lote 2 deveria ter revertido")
	}
}

func TestRecusarBancoProd(t *testing.T) {
	if err := RecusarBancoProd("data/hinos.db"); err == nil {
		t.Fatal("deveria recusar data/hinos.db")
	}
	// Caminho absoluto também é barrado (é o caso real do CLI).
	abs, err := filepath.Abs("data/hinos.db")
	if err != nil {
		t.Fatal(err)
	}
	if err := RecusarBancoProd(abs); err == nil {
		t.Fatalf("deveria recusar %s", abs)
	}
	if err := RecusarBancoProd("/x/data/hinos-test.db"); err != nil {
		t.Fatalf("clone deveria passar: %v", err)
	}
}

func TestRelatorio_PorColetanea(t *testing.T) {
	conn := bancoTeste(t)
	p := csvTemp(t, cabecalho+
		`a,um.pptx,CC,1,T1,"L1",OK,`+"\n"+
		`a,dois.pptx,CC,2,T2,"L2",OK,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	plano, err := Planejar(conn, linhas)
	if err != nil {
		t.Fatal(err)
	}
	rel := Relatorio(plano.Prontos)
	if !strings.Contains(rel, "CC: novos=1 modificados=1") {
		t.Fatalf("relatório inesperado:\n%s", rel)
	}
}
