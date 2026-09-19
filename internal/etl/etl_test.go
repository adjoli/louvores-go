package etl

import (
	"context"
	"database/sql"
	"errors"
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
	if linhas[0].Coletanea != "CC" || linhas[0].Numero != 1 || linhas[0].NumLinha != 2 {
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
	plano, err := Planejar(context.Background(), conn, linhas, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plano.Prontos) != 2 || plano.Pulados != 1 || plano.Erros != 1 {
		t.Fatalf("plano = %+v", plano)
	}
	novos, mods := 0, 0
	for _, it := range plano.Prontos {
		if it.Destino.TemLetra {
			mods++
		} else {
			novos++
		}
	}
	if novos != 1 || mods != 1 {
		t.Fatalf("novos=%d modificados=%d, esperado 1 e 1", novos, mods)
	}
	if plano.Pulados != 1 || plano.PuladosPorCol["CC"] != 1 {
		t.Fatalf("pulados=%d porCol=%v", plano.Pulados, plano.PuladosPorCol)
	}
}

func TestImportar_EscreveTitleCaseERevisado(t *testing.T) {
	conn := bancoTeste(t)
	p := csvTemp(t, cabecalho+`a,um.pptx,CC,1,T1,"OH! MARAVILHA MAIUSCULA",OK,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	plano, err := Planejar(context.Background(), conn, linhas, true)
	if err != nil {
		t.Fatal(err)
	}
	n, err := Importar(context.Background(), conn, plano, 100, nil)
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
	plano, err := Planejar(context.Background(), conn, linhas, true)
	if err != nil {
		t.Fatal(err)
	}
	// Sabota o segundo alvo depois do plano: some com o hino.
	if _, err := conn.Exec(`DELETE FROM hino WHERE numeracao = 2`); err != nil {
		t.Fatal(err)
	}
	if _, err := Importar(context.Background(), conn, plano, 1, nil); err == nil || !strings.Contains(err.Error(), "lote 2-2") {
		t.Fatalf("esperava erro no lote 2-2, veio %v", err)
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
	plano, err := Planejar(context.Background(), conn, linhas, true)
	if err != nil {
		t.Fatal(err)
	}
	rel := Relatorio(plano)
	if !strings.Contains(rel, "CC: novos=1 modificados=1") {
		t.Fatalf("relatório inesperado:\n%s", rel)
	}
	if !strings.Contains(rel, "TOTAL prontos=2 (novos=1 modificados=1)") {
		t.Fatalf("falta linha TOTAL em:\n%s", rel)
	}
}

func TestPlanejar_PulaQuemJaTemLetra(t *testing.T) {
	conn := bancoTeste(t)
	p := csvTemp(t, cabecalho+`a,dois.pptx,CC,2,T2,"Nova",OK,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	plano, err := Planejar(context.Background(), conn, linhas, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plano.Prontos) != 0 || plano.JaTem != 1 || plano.Pulados != 1 || plano.PuladosPorCol["CC"] != 1 {
		t.Fatalf("plano = %+v", plano)
	}
}

func TestPlanejar_ForceSobrescreveEIdempotente(t *testing.T) {
	conn := bancoTeste(t)
	p := csvTemp(t, cabecalho+`a,um.pptx,CC,1,T1,"Letra Um",OK,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	plano, err := Planejar(context.Background(), conn, linhas, false)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := Importar(context.Background(), conn, plano, 100, nil); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	// Re-planejar: agora tem letra → 0 prontos (idempotente).
	plano2, err := Planejar(context.Background(), conn, linhas, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plano2.Prontos) != 0 {
		t.Fatalf("reexecução deveria ser vazia: %+v", plano2)
	}
	// Com --force, volta a ser modificável.
	plano3, err := Planejar(context.Background(), conn, linhas, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plano3.Prontos) != 1 || !plano3.Prontos[0].Destino.TemLetra {
		t.Fatalf("force deveria liberar: %+v", plano3)
	}
}

func TestPlanejar_ChaveDuplicadaCSV(t *testing.T) {
	conn := bancoTeste(t)
	p := csvTemp(t, cabecalho+
		`a,um.pptx,CC,1,T1,"L1",OK,`+"\n"+
		`a,outro.pptx,CC,1,T1,"L2",OK,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	plano, err := Planejar(context.Background(), conn, linhas, false)
	if err != nil {
		t.Fatal(err)
	}
	if plano.Erros != 1 || len(plano.Prontos) != 1 {
		t.Fatalf("plano = %+v", plano)
	}
}

func TestPlanejar_ChaveDuplicadaBanco(t *testing.T) {
	conn := bancoTeste(t)
	if _, err := conn.Exec(`INSERT INTO hino (coletanea_id, numeracao, titulo) VALUES (1, 1, 'Dup')`); err != nil {
		t.Fatal(err)
	}
	p := csvTemp(t, cabecalho+`a,um.pptx,CC,1,T1,"L1",OK,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	plano, err := Planejar(context.Background(), conn, linhas, false)
	if err != nil {
		t.Fatal(err)
	}
	if plano.Erros != 1 {
		t.Fatalf("plano = %+v", plano)
	}
}

func TestLerCSV_LinhaCurtaENumeroInvalido(t *testing.T) {
	p := csvTemp(t, cabecalho+
		"a,curto.pptx,CC\n"+
		`a,ruim.pptx,CC,abc,T,L,OK,`+"\n"+
		`a,zero.pptx,CC,0,T,L,OK,`+"\n")
	if _, err := LerCSV(p); err == nil {
		t.Fatal("esperava erros de parse")
	}
}

func TestLerCSV_BOM(t *testing.T) {
	p := csvTemp(t, "\ufeff"+cabecalho+`a,um.pptx,CC,1,T1,"L1",OK,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatalf("BOM deveria ser tolerado: %v", err)
	}
	if len(linhas) != 1 {
		t.Fatalf("linhas = %d", len(linhas))
	}
}

func TestImportar_LoteInvalido(t *testing.T) {
	conn := bancoTeste(t)
	if _, err := Importar(context.Background(), conn, &Plano{}, 0, nil); err == nil {
		t.Fatal("lote 0 deveria errar")
	}
}

func TestBackupArquivo(t *testing.T) {
	orig := filepath.Join(t.TempDir(), "banco.db")
	if err := os.WriteFile(orig, []byte("dados"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest, err := BackupArquivo(orig)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "dados" {
		t.Fatalf("backup inválido: %v %q", err, got)
	}
	if _, err := BackupArquivo(filepath.Join(t.TempDir(), "falta.db")); err == nil {
		t.Fatal("arquivo inexistente deveria errar")
	}
}

func TestRecusarBancoProd_RemotoEVazio(t *testing.T) {
	for _, p := range []string{"", "libsql://x.turso.io", "https://x/db"} {
		if err := RecusarBancoProd(p); err == nil {
			t.Fatalf("%q deveria ser recusado", p)
		}
	}
}

func TestInspecionarBlocos(t *testing.T) {
	conn := bancoTeste(t)
	if _, err := conn.Exec(`UPDATE hino SET letra = 'Estrofe Um' || char(10) || char(10) || '  Refrao Um', revisado = 1 WHERE numeracao = 1`); err != nil {
		t.Fatal(err)
	}
	out, err := InspecionarBlocos(context.Background(), conn, "CC/1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2 blocos") || !strings.Contains(out, "[refrao]") {
		t.Fatalf("saída inesperada:\n%s", out)
	}
	if _, err := InspecionarBlocos(context.Background(), conn, "sem-barra"); err == nil {
		t.Fatal("formato inválido deveria errar")
	}
}

func TestLerCSV_LinhaLongaEDupHeader(t *testing.T) {
	p := csvTemp(t, cabecalho+`a,um.pptx,CC,1,T1,"L1",OK,,EXTRA`+"\n")
	if _, err := LerCSV(p); err == nil {
		t.Fatal("linha longa deveria errar")
	}
	p2 := csvTemp(t, "arquivo,arquivo,hinario,numero,titulo,letra,status\n")
	if _, err := LerCSV(p2); err == nil {
		t.Fatal("cabeçalho duplicado deveria errar")
	}
}

func TestImportar_PlanoNulo(t *testing.T) {
	conn := bancoTeste(t)
	if _, err := Importar(context.Background(), conn, nil, 10, nil); err == nil {
		t.Fatal("plano nulo deveria errar")
	}
}

func TestInspecionarBlocos_NumeroInvalido(t *testing.T) {
	conn := bancoTeste(t)
	if _, err := InspecionarBlocos(context.Background(), conn, "CC/0"); err == nil {
		t.Fatal("número 0 deveria errar")
	}
	if _, err := InspecionarBlocos(context.Background(), conn, "CC/abc"); !errors.Is(err, ErrLinhaInvalida) {
		t.Fatalf("esperava ErrLinhaInvalida, veio %v", err)
	}
}

func TestLerCSV_SoCabecalho(t *testing.T) {
	p := csvTemp(t, cabecalho)
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(linhas) != 0 {
		t.Fatalf("linhas = %d, esperado 0", len(linhas))
	}
}

func TestLerCSV_NumeroZeroRejeitado(t *testing.T) {
	p := csvTemp(t, cabecalho+`a,z.pptx,CC,0,T,L,OK,`+"\n")
	if _, err := LerCSV(p); err == nil {
		t.Fatal("numero 0 deveria errar")
	}
}

func TestPlanejar_NumeroZeroConstruido(t *testing.T) {
	conn := bancoTeste(t)
	plano, err := Planejar(context.Background(), conn, []Linha{{Arquivo: "x", Coletanea: "CC", Numero: 0, Letra: "L", Status: "OK", NumLinha: 2}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if plano.Erros != 1 || !strings.Contains(plano.Detalhes[0], "incompletos") {
		t.Fatalf("plano = %+v", plano)
	}
}

func TestPlanejar_LetraVaziaEColetaneaInexistente(t *testing.T) {
	conn := bancoTeste(t)
	plano, err := Planejar(context.Background(), conn, []Linha{
		{Arquivo: "x", Coletanea: "CC", Numero: 1, Letra: "   ", Status: "OK", NumLinha: 2},
		{Arquivo: "y", Coletanea: "XX", Numero: 1, Letra: "L", Status: "OK", NumLinha: 3},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if plano.Erros != 2 {
		t.Fatalf("plano = %+v", plano)
	}
}

func TestImportar_ProgContaLotes(t *testing.T) {
	conn := bancoTeste(t)
	p := csvTemp(t, cabecalho+
		`a,um.pptx,CC,1,T1,"L1",OK,`+"\n"+
		`a,dois.pptx,CC,2,T2,"L2",OK,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	plano, err := Planejar(context.Background(), conn, linhas, true)
	if err != nil {
		t.Fatal(err)
	}
	chamadas := 0
	if _, err := Importar(context.Background(), conn, plano, 1, func(feitos, total int) { chamadas++ }); err != nil {
		t.Fatal(err)
	}
	if chamadas != 2 {
		t.Fatalf("prog chamado %dx, esperado 2", chamadas)
	}
}

func TestRelatorio_TotalEPuladosPorCol(t *testing.T) {
	plano := &Plano{
		Prontos:       []Item{{Linha: Linha{Coletanea: "CC"}, Destino: Destino{TemLetra: true}}},
		Pulados:       2,
		PuladosPorCol: map[string]int{"CC": 1, "": 1},
	}
	rel := Relatorio(plano)
	for _, want := range []string{"TOTAL prontos=1 (novos=0 modificados=1)", "pulados CC: 1", "(sem coletânea)"} {
		if !strings.Contains(rel, want) {
			t.Fatalf("falta %q em:\n%s", want, rel)
		}
	}
}

func TestInspecionarBlocos_ErroSentinelaECorte(t *testing.T) {
	conn := bancoTeste(t)
	if _, err := InspecionarBlocos(context.Background(), conn, "CC/abc"); err == nil {
		t.Fatal("deveria errar")
	}
	if _, err := InspecionarBlocos(context.Background(), conn, "CC/1"); err != nil {
		t.Fatal(err)
	}
	longa := strings.Repeat("á", 51)
	if _, err := conn.Exec(`UPDATE hino SET letra = ? WHERE numeracao = 1`, longa+"\n  refrão"); err != nil {
		t.Fatal(err)
	}
	out, err := InspecionarBlocos(context.Background(), conn, "CC/1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, strings.Repeat("á", 50)+"...") {
		t.Fatalf("corte de 50 runes não aplicado:\n%s", out)
	}
}

func TestLerCSV_ArquivoInexistenteEMalformado(t *testing.T) {
	if _, err := LerCSV(filepath.Join(t.TempDir(), "falta.csv")); err == nil {
		t.Fatal("arquivo inexistente deveria errar")
	}
	p := csvTemp(t, cabecalho+"a,um.pptx,CC,1,T1,\"aspas quebradas,OK,\n")
	if _, err := LerCSV(p); err == nil {
		t.Fatal("csv malformado deveria errar")
	}
}

func TestImportar_BancoFechado(t *testing.T) {
	conn := bancoTeste(t)
	conn.Close()
	if _, err := Importar(context.Background(), conn, &Plano{Prontos: []Item{{Linha: Linha{NumLinha: 2}, Destino: Destino{HinoID: 1}}}}, 10, nil); err == nil {
		t.Fatal("banco fechado deveria errar")
	}
}

func TestBackupArquivo_Diretorio(t *testing.T) {
	if _, err := BackupArquivo(t.TempDir()); err == nil {
		t.Fatal("diretório deveria errar")
	}
}

func TestRelatorio_PlanoNulo(t *testing.T) {
	if got := Relatorio(nil); !strings.Contains(got, "nulo") {
		t.Fatalf("esperava aviso, veio %q", got)
	}
}

func TestInspecionarBlocos_NaoEncontrados(t *testing.T) {
	conn := bancoTeste(t)
	if _, err := InspecionarBlocos(context.Background(), conn, "XX/1"); err == nil {
		t.Fatal("coletânea inexistente deveria errar")
	}
	if _, err := InspecionarBlocos(context.Background(), conn, "CC/999"); err == nil {
		t.Fatal("hino inexistente deveria errar")
	}
}

func TestLerCSV_Vazio(t *testing.T) {
	p := csvTemp(t, "")
	if _, err := LerCSV(p); err == nil {
		t.Fatal("csv vazio deveria errar")
	}
}

func TestPlanejar_BancoFechado(t *testing.T) {
	conn := bancoTeste(t)
	conn.Close()
	linhas := []Linha{{Arquivo: "x", Coletanea: "CC", Numero: 1, Letra: "L", Status: "OK", NumLinha: 2}}
	if _, err := Planejar(context.Background(), conn, linhas, false); err == nil {
		t.Fatal("banco fechado deveria errar")
	}
}

func TestLerCSV_NumeroLinhaNoErro(t *testing.T) {
	p := csvTemp(t, cabecalho+
		`a,ok.pptx,CC,1,T,L,OK,`+"\n"+
		`a,ruim.pptx,CC,abc,T,L,OK,`+"\n")
	_, err := LerCSV(p)
	if err == nil || !strings.Contains(err.Error(), "linha 3") {
		t.Fatalf("erro deveria citar a linha 3, veio %v", err)
	}
}

func TestRelatorio_PuladosOrdemAlfabetica(t *testing.T) {
	plano := &Plano{Pulados: 3, PuladosPorCol: map[string]int{"VM": 1, "CC": 2}}
	rel := Relatorio(plano)
	icc := strings.Index(rel, "pulados CC: 2")
	ivm := strings.Index(rel, "pulados VM: 1")
	if icc < 0 || ivm < 0 || icc > ivm {
		t.Fatalf("ordem alfabética esperada em:\n%s", rel)
	}
}

func TestValidarSlides_PacoteIntegro(t *testing.T) {
	conn := bancoTeste(t)
	tpl := filepath.Join("..", "..", "data", "templates", "default.pptx")
	if _, err := os.Stat(tpl); err != nil {
		t.Skip("template ausente")
	}
	p := csvTemp(t, cabecalho+
		`a,um.pptx,CC,1,T1,"Estrofe Um`+"\n"+`Segunda Linha",OK,`+"\n"+
		`a,dois.pptx,CC,2,T2,"Outra`+"\n"+`  Refrao",OK,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	plano, err := Planejar(context.Background(), conn, linhas, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Importar(context.Background(), conn, plano, 100, nil); err != nil {
		t.Fatal(err)
	}
	res := ValidarSlides(context.Background(), conn, tpl, plano.Prontos)
	if len(res) != 2 {
		t.Fatalf("res = %+v", res)
	}
	for _, v := range res {
		if v.Erro != nil || v.Slides < 2 {
			t.Fatalf("hino %s inválido: %+v", v.Chave, v)
		}
	}
}

func TestValidarSlides_TemplateAusente(t *testing.T) {
	conn := bancoTeste(t)
	res := ValidarSlides(context.Background(), conn, filepath.Join(t.TempDir(), "falta.pptx"),
		[]Item{{Linha: Linha{Coletanea: "CC", Numero: 1}, Destino: Destino{HinoID: 1}}})
	if len(res) != 1 || res[0].Erro == nil {
		t.Fatalf("res = %+v", res)
	}
}

func TestImportar_RollbackParcialDentroDoLote(t *testing.T) {
	conn := bancoTeste(t)
	p := csvTemp(t, cabecalho+
		`a,um.pptx,CC,1,T1,"L1",OK,`+"\n"+
		`a,dois.pptx,CC,2,T2,"L2",OK,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	plano, err := Planejar(context.Background(), conn, linhas, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`DELETE FROM hino WHERE numeracao = 2`); err != nil {
		t.Fatal(err)
	}
	if _, err := Importar(context.Background(), conn, plano, 2, nil); err == nil {
		t.Fatal("esperava erro no lote")
	}
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM hino WHERE numeracao = 1 AND letra = 'L1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("lote inteiro deveria ter revertido, incluindo CC/1")
	}
}

func TestValidarSlides_NaoRevisadoEFalhaMista(t *testing.T) {
	conn := bancoTeste(t)
	tpl := filepath.Join("..", "..", "data", "templates", "default.pptx")
	if _, err := os.Stat(tpl); err != nil {
		t.Skip("template ausente")
	}
	if _, err := conn.Exec(`UPDATE hino SET letra = 'Estrofe', revisado = 0 WHERE numeracao = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE hino SET letra = 'Outra', revisado = 1 WHERE numeracao = 2`); err != nil {
		t.Fatal(err)
	}
	res := ValidarSlides(context.Background(), conn, tpl, []Item{
		{Linha: Linha{Coletanea: "CC", Numero: 2}, Destino: Destino{HinoID: 2}},
		{Linha: Linha{Coletanea: "CC", Numero: 1}, Destino: Destino{HinoID: 1}},
	})
	if len(res) != 2 {
		t.Fatalf("res = %+v", res)
	}
	if res[0].Erro != nil {
		t.Fatalf("CC/2 deveria validar: %v", res[0].Erro)
	}
	if !strings.Contains(res[1].Erro.Error(), "revisado") || res[1].Chave != "CC/1" || res[1].Slides != 0 {
		t.Fatalf("res[1] = %+v", res[1])
	}
}

func TestPlanejar_NormalizaStatusEColetanea(t *testing.T) {
	conn := bancoTeste(t)
	p := csvTemp(t, cabecalho+`a,um.pptx,cc,1,T1,"L1", Ok ,`+"\n")
	linhas, err := LerCSV(p)
	if err != nil {
		t.Fatal(err)
	}
	plano, err := Planejar(context.Background(), conn, linhas, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plano.Prontos) != 1 || plano.Prontos[0].Linha.Coletanea != "CC" {
		t.Fatalf("plano = %+v", plano)
	}
}
