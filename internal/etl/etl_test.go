package etl

import (
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCSVCreditosPreservados(t *testing.T) {
	for _, credit := range []string{"\u00a0", " Autor Humano ", "", " \t\n"} {
		t.Run(credit, func(t *testing.T) {
			var data strings.Builder
			w := csv.NewWriter(&data)
			if err := w.Write([]string{"arquivo", "hinario", "numero", "titulo", "letra", "status", "creditos"}); err != nil {
				t.Fatal(err)
			}
			if err := w.Write([]string{"hino.pptx", "CC", "1", "Hino", "Ó Vem", "OK", credit}); err != nil {
				t.Fatal(err)
			}
			w.Flush()
			linhas, err := LerCSV(csvTemp(t, data.String()))
			if err != nil {
				t.Fatal(err)
			}
			if credit == "" || credit == " \t\n" {
				if linhas[0].Creditos != nil {
					t.Fatal("vazio ASCII deveria representar ausência de proposta")
				}
			} else if linhas[0].Creditos == nil || *linhas[0].Creditos != credit {
				t.Fatalf("crédito presente modificado: esperado %q, obtido %v", credit, linhas[0].Creditos)
			}
		})
	}
}

func TestSnapshotRecusaWALOculto(t *testing.T) {
	real, closeDB := criarBancoSnapshot(t, true)
	defer closeDB()
	for _, kind := range []string{"symlink", "hardlink", "copy"} {
		t.Run(kind, func(t *testing.T) {
			alias := filepath.Join(t.TempDir(), "alias.db")
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(real, alias)
			case "hardlink":
				err = os.Link(real, alias)
			case "copy":
				var data []byte
				data, err = os.ReadFile(real)
				if err == nil {
					err = os.WriteFile(alias, data, 0o600)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			conn, err := AbrirSnapshot(alias)
			if err == nil {
				conn.Close()
				t.Fatal("snapshot WAL sem sidecars no alias foi aceito")
			}
		})
	}
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

	// Banco inexistente é recusado
	if err := RecusarBancoProd(filepath.Join(t.TempDir(), "inexistente.db")); err == nil {
		t.Fatal("deveria recusar banco inexistente")
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

func TestRecusarBancoProd_RemotoEVazio(t *testing.T) {
	for _, p := range []string{"", "libsql://x.turso.io", "https://x/db", "http://x/db", "ws://x/db", "wss://x/db", "file:x.db"} {
		if err := RecusarBancoProd(p); err == nil {
			t.Fatalf("%q deveria ser recusado", p)
		}
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

func TestLerCSV_ArquivoInexistenteEMalformado(t *testing.T) {
	if _, err := LerCSV(filepath.Join(t.TempDir(), "falta.csv")); err == nil {
		t.Fatal("arquivo inexistente deveria errar")
	}
	p := csvTemp(t, cabecalho+"a,um.pptx,CC,1,T1,\"aspas quebradas,OK,\n")
	if _, err := LerCSV(p); err == nil {
		t.Fatal("csv malformado deveria errar")
	}
}

func TestLerCSV_Vazio(t *testing.T) {
	p := csvTemp(t, "")
	if _, err := LerCSV(p); err == nil {
		t.Fatal("csv vazio deveria errar")
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

func TestCSVCreditosENumero(t *testing.T) {
	csvContent := "pasta,arquivo,hinario,numero,titulo,letra,status,observacao,creditos,source_id,source_sha256\n" +
		"p,1.pptx,CC,1,T1,L1,OK,,Autor Um,src-1,sha-1\n" +
		"p,2.pptx,CC,,T2,L2,PENDENTE,,Autor Dois,src-2,sha-2\n" +
		"p,3.pptx,CC,abc,T3,L3,IGNORAR,,  ,src-3,sha-3\n" +
		"p,4.pptx,CC,4,T4,L4,OK,,,src-4,sha-4\n"

	linhas, err := LerCSV(csvTemp(t, csvContent))
	if err != nil {
		t.Fatalf("esperava sucesso para PENDENTE/IGNORAR sem número, veio: %v", err)
	}
	if len(linhas) != 4 {
		t.Fatalf("linhas = %d, esperado 4", len(linhas))
	}
	if linhas[0].Numero != 1 || linhas[0].Creditos == nil || *linhas[0].Creditos != "Autor Um" || linhas[0].FonteID != "src-1" || linhas[0].FonteSHA256 != "sha-1" {
		t.Fatalf("linha 1 inesperada: %+v", linhas[0])
	}
	if linhas[1].Numero != 0 || linhas[1].Status != "PENDENTE" {
		t.Fatalf("linha 2 inesperada: %+v", linhas[1])
	}
	if linhas[2].Numero != 0 || linhas[2].Status != "IGNORAR" || linhas[2].Creditos != nil {
		t.Fatalf("linha 3 com espaço em créditos deveria ter Creditos=nil: %+v", linhas[2])
	}
	if linhas[3].Numero != 4 || linhas[3].Creditos != nil {
		t.Fatalf("linha 4 com créditos vazios deveria ter Creditos=nil: %+v", linhas[3])
	}

	// OK sem número falha com indicação da linha
	csvErro := "pasta,arquivo,hinario,numero,titulo,letra,status,observacao\n" +
		"p,ok.pptx,CC,1,T1,L1,OK,\n" +
		"p,ruim.pptx,CC,,T2,L2,OK,\n"
	_, err = LerCSV(csvTemp(t, csvErro))
	if err == nil || !strings.Contains(err.Error(), "linha 3") || !errors.Is(err, ErrLinhaInvalida) {
		t.Fatalf("OK sem número deveria falhar citando linha 3 e ErrLinhaInvalida, veio: %v", err)
	}

	// Coluna creditos ausente resulta em Creditos == nil
	csvSemCred := "pasta,arquivo,hinario,numero,titulo,letra,status\n" +
		"p,1.pptx,CC,1,T1,L1,OK\n"
	linhasSemCred, err := LerCSV(csvTemp(t, csvSemCred))
	if err != nil {
		t.Fatalf("csv sem créditos falhou: %v", err)
	}
	if len(linhasSemCred) != 1 || linhasSemCred[0].Creditos != nil {
		t.Fatalf("creditos deveria ser nil quando coluna ausente, veio: %+v", linhasSemCred[0])
	}
}
