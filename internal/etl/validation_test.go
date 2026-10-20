package etl

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/domain"
	"github.com/adjoli/louvores-go/internal/ppt"
	"github.com/adjoli/louvores-go/internal/repository"
	"github.com/adjoli/louvores-go/internal/services"
)

func pptGenerateHelper(titulo, subtitulo, tituloSlides string, seq domain.SequenciaHino, templatePath string) ([]byte, error) {
	return ppt.GenerateSlides(titulo, subtitulo, tituloSlides, seq, templatePath)
}

func templateTeste(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "data", "templates", "default.pptx")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("template padrão ausente: %v", err)
	}
	return p
}

func hashArquivo(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler arquivo para hash: %v", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func criarBancoSnapshot(t *testing.T, comWAL bool) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "snapshot.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("abrir banco snapshot: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrar banco snapshot: %v", err)
	}

	if comWAL {
		if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
			t.Fatalf("ativar WAL: %v", err)
		}
	}

	_, err = db.Exec(`
		INSERT INTO coletanea (id, codigo, titulo) VALUES (1, 'CC', 'Cantor Cristão');
		INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado)
		VALUES
			(1, 1, 1, 'Hino Um', NULL, NULL, 0),
			(2, 1, 2, 'Hino Dois', NULL, NULL, 0);
	`)
	if err != nil {
		t.Fatalf("popular dados no snapshot: %v", err)
	}

	if comWAL {
		// Mantém db aberto para que o arquivo WAL permaneça ativo durante o teste
		return dbPath, func() {
			_ = db.Close()
		}
	}

	if err := db.Close(); err != nil {
		t.Fatalf("fechar banco snapshot: %v", err)
	}
	return dbPath, func() {}
}

func repackZip(t *testing.T, data []byte, mutator func(parts map[string][]byte)) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("abrir zip para mutação: %v", err)
	}
	parts := make(map[string][]byte)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("abrir entry %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("ler entry %s: %v", f.Name, err)
		}
		parts[f.Name] = b
	}

	mutator(parts)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("criar entry %s: %v", name, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("escrever entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("fechar zip mutado: %v", err)
	}
	return buf.Bytes()
}

// TestValidarSnapshotIntacto compara hash/tabelas/bytes antes/depois;
// clone consistente de SQLite com WAL; temp removido em sucesso/falha/cancelamento.
func TestValidarSnapshotIntacto(t *testing.T) {
	tpl := templateTeste(t)
	dbPath, closeDB := criarBancoSnapshot(t, true)
	defer closeDB()

	// Garante que o arquivo WAL existe antes da validação
	walPath := dbPath + "-wal"
	if _, err := os.Stat(walPath); os.IsNotExist(err) {
		t.Fatalf("esperava arquivo WAL em %s", walPath)
	}

	hashAntes := hashArquivo(t, dbPath)
	bytesAntes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("ler bytes antes: %v", err)
	}

	m := &Manifesto{
		Meta: MetaManifesto{
			Versao:           1,
			BatchID:          "batch-snapshot-intacto",
			SnapshotSHA256:   strings.Repeat("a", 64),
			CSVSHA256:        strings.Repeat("b", 64),
			ProvenanceSHA256: strings.Repeat("c", 64),
			TemplateSHA256:   strings.Repeat("d", 64),
		},
		Itens: []ItemManifesto{
			{
				HinoID:         1,
				ColetaneaID:    1,
				Codigo:         "CC",
				Numero:         1,
				TituloEsperado: "Hino Um",
				Arquivo:        "1.pptx",
				FonteID:        strings.Repeat("1", 64),
				FonteSHA256:    strings.Repeat("2", 64),
				Letra:          "Estrofe Um\nPrimeira linha\n\n    Refrão Um\n    Segunda linha",
			},
		},
	}
	manifestSHA := strings.Repeat("e", 64)

	// 1. Execução com sucesso: snapshot permanece 100% idêntico
	res, err := ValidarManifesto(context.Background(), dbPath, tpl, m, manifestSHA)
	if err != nil {
		t.Fatalf("ValidarManifesto falhou: %v", err)
	}
	if len(res) != 1 || res[0].Erro != nil || res[0].Slides < 2 {
		t.Fatalf("resultado inesperado: %+v", res)
	}

	hashDepois := hashArquivo(t, dbPath)
	if hashAntes != hashDepois {
		t.Errorf("hash do snapshot foi modificado! antes=%s depois=%s", hashAntes, hashDepois)
	}
	bytesDepois, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("ler bytes depois: %v", err)
	}
	if !bytes.Equal(bytesAntes, bytesDepois) {
		t.Errorf("bytes do snapshot divergiram após validação")
	}

	// Confere que tabelas e linhas do snapshot continuam intactas
	dbCheck, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("reabrir snapshot: %v", err)
	}
	var countHino, revisados int
	if err := dbCheck.QueryRow("SELECT COUNT(*), SUM(revisado) FROM hino").Scan(&countHino, &revisados); err != nil {
		t.Fatalf("consultar snapshot: %v", err)
	}
	if countHino != 2 || revisados != 0 {
		t.Errorf("snapshot modificado: total=%d revisados=%d", countHino, revisados)
	}
	dbCheck.Close()

	// 2. Limpeza de temp em caso de cancelamento
	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel()

	tempEntriesAntes, _ := os.ReadDir(os.TempDir())
	_, errCancel := ValidarManifesto(ctxCancel, dbPath, tpl, m, manifestSHA)
	if errCancel == nil {
		t.Error("esperava erro em contexto cancelado")
	}
	tempEntriesDepois, _ := os.ReadDir(os.TempDir())
	for _, entry := range tempEntriesDepois {
		if strings.HasPrefix(entry.Name(), "louvores-validar-") {
			encontrado := false
			for _, ant := range tempEntriesAntes {
				if ant.Name() == entry.Name() {
					encontrado = true
					break
				}
			}
			if !encontrado {
				t.Errorf("arquivo temporário vazou após cancelamento: %s", entry.Name())
			}
		}
	}

	// 3. Limpeza de temp em caso de falha de validação/SQL
	mInvalido := &Manifesto{
		Meta: MetaManifesto{
			Versao:  1,
			BatchID: "batch-invalido",
		},
		Itens: []ItemManifesto{
			{
				HinoID:         999, // Hino inexistente
				ColetaneaID:    1,
				Codigo:         "CC",
				Numero:         999,
				TituloEsperado: "Inexistente",
				Letra:          "Letra",
			},
		},
	}
	tempEntriesAntesFalha, _ := os.ReadDir(os.TempDir())
	resFalha, errFalha := ValidarManifesto(context.Background(), dbPath, tpl, mInvalido, manifestSHA)
	_ = errFalha
	_ = resFalha
	tempEntriesDepoisFalha, _ := os.ReadDir(os.TempDir())
	for _, entry := range tempEntriesDepoisFalha {
		if strings.HasPrefix(entry.Name(), "louvores-validar-") {
			encontrado := false
			for _, ant := range tempEntriesAntesFalha {
				if ant.Name() == entry.Name() {
					encontrado = true
					break
				}
			}
			if !encontrado {
				t.Errorf("arquivo temporário vazou após falha: %s", entry.Name())
			}
		}
	}
}

// TestValidarNaoRevisado: candidato passa na cópia e original continua revisado=0;
// chamada normal do serviço ao original continua ErrHinoNotReviewed.
func TestValidarNaoRevisado(t *testing.T) {
	tpl := templateTeste(t)
	dbPath, closeDB := criarBancoSnapshot(t, false)
	defer closeDB()

	m := &Manifesto{
		Meta: MetaManifesto{
			Versao:           1,
			BatchID:          "batch-nao-revisado",
			SnapshotSHA256:   strings.Repeat("a", 64),
			CSVSHA256:        strings.Repeat("b", 64),
			ProvenanceSHA256: strings.Repeat("c", 64),
			TemplateSHA256:   strings.Repeat("d", 64),
		},
		Itens: []ItemManifesto{
			{
				HinoID:         1,
				ColetaneaID:    1,
				Codigo:         "CC",
				Numero:         1,
				TituloEsperado: "Hino Um",
				Arquivo:        "1.pptx",
				FonteID:        strings.Repeat("1", 64),
				FonteSHA256:    strings.Repeat("2", 64),
				Letra:          "Primeira estrofe\nLinha um\n\nSegunda estrofe\nLinha dois",
			},
		},
	}
	manifestSHA := strings.Repeat("e", 64)

	res, err := ValidarManifesto(context.Background(), dbPath, tpl, m, manifestSHA)
	if err != nil {
		t.Fatalf("ValidarManifesto: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("tamanho esperado 1, obtido %d", len(res))
	}
	if res[0].Erro != nil {
		t.Fatalf("candidato deveria passar na cópia, mas falhou: %v", res[0].Erro)
	}
	if res[0].Slides != 3 { // 1 título + 2 estrofes = 3 slides
		t.Fatalf("slides esperados 3, obtidos %d", res[0].Slides)
	}

	// Abre o banco original para verificar que continua revisado=0 e sem letra
	origDB, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("abrir banco original: %v", err)
	}
	defer origDB.Close()

	var revisado int
	var letra *string
	if err := origDB.QueryRow("SELECT revisado, letra FROM hino WHERE id = 1").Scan(&revisado, &letra); err != nil {
		t.Fatalf("consultar hino 1: %v", err)
	}
	if revisado != 0 {
		t.Errorf("hino original deveria continuar revisado=0, obtido %d", revisado)
	}
	if letra != nil {
		t.Errorf("hino original deveria continuar com letra NULL, obtido %v", *letra)
	}

	// Chamada normal do serviço ao banco original DEVE falhar com ErrHinoNotReviewed
	hinoRepo := repository.NewSQLiteHinoRepository(origDB)
	coletaneaRepo := repository.NewSQLiteColetaneaRepository(origDB)
	svc := services.NewHinoService(hinoRepo, coletaneaRepo, tpl)

	_, errOriginal := svc.GerarSlides(context.Background(), "CC", 1, tpl)
	if !errors.Is(errOriginal, services.ErrHinoNotReviewed) {
		t.Errorf("esperado ErrHinoNotReviewed na chamada ao banco original, obtido: %v", errOriginal)
	}
}

// TestValidarOOXML: corromper XML, CRC, relationship, Override, layout, sldId,
// ordem/texto de body, rodapé ou número de slides → falha identificada pela chave.
// Pacote bom título+N slides passa; template ausente falha.
func TestValidarOOXML(t *testing.T) {
	tpl := templateTeste(t)
	seq := domain.SequenciaHino{
		Partes: []domain.ParteHino{
			{Txt: "Primeira estrofe\nLinha dois", Numero: 1, Tipo: domain.TipoParteEstrofe},
			{Txt: "Refrão da canção\nLinha de coro", Numero: 2, Tipo: domain.TipoParteRefrao},
		},
	}

	// Gera pacote válido base
	hinoRepo := repository.NewSQLiteHinoRepository(nil)
	coletRepo := repository.NewSQLiteColetaneaRepository(nil)
	_ = hinoRepo
	_ = coletRepo

	// Gera slides diretamente pelo gerador do app
	validoBytes, err := services.NewHinoService(nil, nil, tpl).TemplatePath(), error(nil)
	_ = validoBytes
	validoPPTX, err := func() ([]byte, error) {
		// Mock temporário para gerar slides usando ppt.GenerateSlides
		return pptGenerateHelper("Hino Um", "Cantor Cristão - 1", "1CC - Hino Um", seq, tpl)
	}()
	if err != nil {
		t.Fatalf("gerar PPTX válido: %v", err)
	}

	// 1. Pacote bom passa
	if err := validarPacote(validoPPTX, seq); err != nil {
		t.Fatalf("pacote válido deveria passar: %v", err)
	}

	t.Run("XMLCompleto", func(t *testing.T) {
		partes := []string{
			"[Content_Types].xml",
			"ppt/presentation.xml",
			"ppt/_rels/presentation.xml.rels",
			"ppt/slides/slide1.xml",
			"ppt/slides/_rels/slide1.xml.rels",
			"ppt/slides/slide2.xml",
			"ppt/slides/_rels/slide2.xml.rels",
			"ppt/slideLayouts/slideLayout1.xml",
			"ppt/slideLayouts/slideLayout2.xml",
			"ppt/slideLayouts/slideLayout3.xml",
		}
		for _, parte := range partes {
			for _, caso := range []struct {
				nome      string
				prefixo   string
				sufixo    string
				substitui bool
			}{
				{nome: "Malformado", prefixo: "<malformed", substitui: true},
				{nome: "Vazio", substitui: true},
				{nome: "TrailingMalformado", sufixo: "<malformed"},
				{nome: "SegundaRaiz", sufixo: "<extra/>"},
				{nome: "TextoAposRaiz", sufixo: "lixo"},
				{nome: "TextoAntesRaiz", prefixo: "lixo"},
				{nome: "NBSPAposRaiz", sufixo: "\u00a0"},
			} {
				t.Run(parte+"/"+caso.nome, func(t *testing.T) {
					corrompido := repackZip(t, validoPPTX, func(parts map[string][]byte) {
						original, ok := parts[parte]
						if !ok {
							t.Fatalf("parte de teste ausente: %s", parte)
						}
						if caso.substitui {
							original = nil
						}
						parts[parte] = []byte(caso.prefixo + string(original) + caso.sufixo)
					})
					if err := validarPacote(corrompido, seq); err == nil {
						t.Errorf("esperava erro para XML inválido em %s", parte)
					} else if !strings.Contains(err.Error(), parte) {
						t.Errorf("erro deveria identificar %s: %v", parte, err)
					}
				})
			}
		}
	})

	t.Run("XMLComWhitespaceEComentarioAposRaiz", func(t *testing.T) {
		pacote := repackZip(t, validoPPTX, func(parts map[string][]byte) {
			parts["ppt/slides/slide1.xml"] = append(parts["ppt/slides/slide1.xml"], []byte(" \t\r\n<!-- comentário válido -->")...)
		})
		if err := validarPacote(pacote, seq); err != nil {
			t.Fatalf("XML com whitespace e comentário após a raiz deveria passar: %v", err)
		}
	})

	t.Run("TemplateComXMLTrailingMalformado", func(t *testing.T) {
		template, err := os.ReadFile(tpl)
		if err != nil {
			t.Fatalf("ler template: %v", err)
		}
		corrompido := repackZip(t, template, func(parts map[string][]byte) {
			parts["ppt/slides/slide1.xml"] = append(parts["ppt/slides/slide1.xml"], []byte("<malformed")...)
		})
		templatePath := filepath.Join(t.TempDir(), "corrompido.pptx")
		if err := os.WriteFile(templatePath, corrompido, 0o600); err != nil {
			t.Fatalf("gravar cópia do template: %v", err)
		}
		pacote, err := pptGenerateHelper("Hino Um", "Cantor Cristão - 1", "1CC - Hino Um", seq, templatePath)
		if err != nil {
			t.Fatalf("gerador deveria preservar o XML do título: %v", err)
		}
		if err := validarPacote(pacote, seq); err == nil {
			t.Error("esperava rejeição do XML inválido preservado pelo gerador")
		}
	})

	t.Run("TipoRelationshipIncorreto", func(t *testing.T) {
		for _, caso := range []struct {
			parte string
			tipo  string
		}{
			{"ppt/_rels/presentation.xml.rels", "slide"},
			{"ppt/slides/_rels/slide1.xml.rels", "slideLayout"},
			{"ppt/slides/_rels/slide2.xml.rels", "slideLayout"},
			{"ppt/slides/_rels/slide3.xml.rels", "slideLayout"},
		} {
			t.Run(caso.parte, func(t *testing.T) {
				corrompido := repackZip(t, validoPPTX, func(parts map[string][]byte) {
					original := parts[caso.parte]
					tipo := `Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/` + caso.tipo + `"`
					parts[caso.parte] = bytes.ReplaceAll(original, []byte(tipo), []byte(`Type="http://example.invalid/`+caso.tipo+`"`))
					if bytes.Equal(original, parts[caso.parte]) {
						t.Fatal("mutação não alterou o tipo da relationship")
					}
				})
				if err := validarPacote(corrompido, seq); err == nil {
					t.Error("esperava erro para tipo incorreto de relationship")
				}
			})
		}
	})

	t.Run("TipoOverrideIncorreto", func(t *testing.T) {
		for _, caso := range []struct {
			parte string
			tipo  string
		}{
			{"/ppt/presentation.xml", "presentation.main"},
			{"/ppt/slides/slide1.xml", "slide"},
			{"/ppt/slides/slide2.xml", "slide"},
			{"/ppt/slides/slide3.xml", "slide"},
			{"/ppt/slideLayouts/slideLayout1.xml", "slideLayout"},
			{"/ppt/slideLayouts/slideLayout2.xml", "slideLayout"},
			{"/ppt/slideLayouts/slideLayout3.xml", "slideLayout"},
		} {
			t.Run(caso.parte, func(t *testing.T) {
				corrompido := repackZip(t, validoPPTX, func(parts map[string][]byte) {
					original := parts["[Content_Types].xml"]
					override := `PartName="` + caso.parte + `" ContentType="application/vnd.openxmlformats-officedocument.presentationml.` + caso.tipo + `+xml"`
					parts["[Content_Types].xml"] = bytes.ReplaceAll(original, []byte(override), []byte(`PartName="`+caso.parte+`" ContentType="text/plain"`))
					if bytes.Equal(original, parts["[Content_Types].xml"]) {
						t.Fatal("mutação não alterou o tipo do Override")
					}
				})
				if err := validarPacote(corrompido, seq); err == nil {
					t.Error("esperava erro para ContentType incorreto de Override")
				}
			})
		}
	})

	// 2. XML corrompido em presentation.xml
	t.Run("XMLCorrompido", func(t *testing.T) {
		corrompido := repackZip(t, validoPPTX, func(parts map[string][]byte) {
			parts["ppt/presentation.xml"] = []byte("<p:presentation><malformed")
		})
		if err := validarPacote(corrompido, seq); err == nil {
			t.Error("esperava erro para XML corrompido")
		}
	})

	// 3. CRC corrompido
	t.Run("CRCCorrompido", func(t *testing.T) {
		// Altera bytes diretamente no meio do ZIP comprimido para estragar o CRC
		corrompido := make([]byte, len(validoPPTX))
		copy(corrompido, validoPPTX)
		corrompido[len(corrompido)/2] ^= 0xFF
		if err := validarPacote(corrompido, seq); err == nil {
			t.Error("esperava erro para CRC corrompido")
		}
	})

	// 4. Relationship corrompida em presentation.xml.rels
	t.Run("RelationshipCorrompida", func(t *testing.T) {
		corrompido := repackZip(t, validoPPTX, func(parts map[string][]byte) {
			// Remove relação do slide 2
			parts["ppt/_rels/presentation.xml.rels"] = []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/>
</Relationships>`)
		})
		if err := validarPacote(corrompido, seq); err == nil {
			t.Error("esperava erro para relationship ausente")
		}
	})

	// 5. Override ausente em [Content_Types].xml
	t.Run("OverrideAusente", func(t *testing.T) {
		corrompido := repackZip(t, validoPPTX, func(parts map[string][]byte) {
			// Remove Override do slide2.xml
			orig := string(parts["[Content_Types].xml"])
			parts["[Content_Types].xml"] = []byte(strings.ReplaceAll(orig, `<Override PartName="/ppt/slides/slide2.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`, ""))
		})
		if err := validarPacote(corrompido, seq); err == nil {
			t.Error("esperava erro para Override ausente")
		}
	})

	// 6. Layout incorreto no .rels do slide
	t.Run("LayoutIncorreto", func(t *testing.T) {
		corrompido := repackZip(t, validoPPTX, func(parts map[string][]byte) {
			// Slide 3 (refrão) apontando para layout 2 (estrofe) em vez de layout 3
			parts["ppt/slides/_rels/slide3.xml.rels"] = []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout2.xml"/>
</Relationships>`)
		})
		if err := validarPacote(corrompido, seq); err == nil {
			t.Error("esperava erro para layout divergente do tipo de parte")
		}
	})

	// 7. sldId corrompido / apontando para rId inexistente
	t.Run("SldIdInexistente", func(t *testing.T) {
		corrompido := repackZip(t, validoPPTX, func(parts map[string][]byte) {
			orig := string(parts["ppt/presentation.xml"])
			parts["ppt/presentation.xml"] = []byte(strings.Replace(orig, `r:id="rId2"`, `r:id="rId999"`, 1))
		})
		if err := validarPacote(corrompido, seq); err == nil {
			t.Error("esperava erro para sldId com rId inexistente")
		}
	})

	// 8. Ordem/texto do body alterado no slide
	t.Run("TextoBodyAlterado", func(t *testing.T) {
		corrompido := repackZip(t, validoPPTX, func(parts map[string][]byte) {
			orig := string(parts["ppt/slides/slide2.xml"])
			parts["ppt/slides/slide2.xml"] = []byte(strings.Replace(orig, "Primeira estrofe", "Texto Alterado Incorreto", 1))
		})
		if err := validarPacote(corrompido, seq); err == nil {
			t.Error("esperava erro para texto de body divergente")
		}
	})

	// 9. Rodapé alterado no slide
	t.Run("RodapeAlterado", func(t *testing.T) {
		corrompido := repackZip(t, validoPPTX, func(parts map[string][]byte) {
			orig := string(parts["ppt/slides/slide2.xml"])
			parts["ppt/slides/slide2.xml"] = []byte(strings.Replace(orig, "1/2", "99/2", 1))
		})
		if err := validarPacote(corrompido, seq); err == nil {
			t.Error("esperava erro para rodapé divergente")
		}
	})

	// 10. Número de slides incorreto
	t.Run("NumeroSlidesIncorreto", func(t *testing.T) {
		seqTres := domain.SequenciaHino{
			Partes: []domain.ParteHino{
				{Txt: "E1", Numero: 1, Tipo: domain.TipoParteEstrofe},
				{Txt: "R1", Numero: 2, Tipo: domain.TipoParteRefrao},
				{Txt: "E2", Numero: 3, Tipo: domain.TipoParteEstrofe},
			},
		}
		// Passa pacote de 2 partes para uma sequência que espera 3 partes
		if err := validarPacote(validoPPTX, seqTres); err == nil {
			t.Error("esperava erro para contagem de slides incompatível com a sequência")
		}
	})

	// 11. Teste de template ausente via ValidarManifesto atribuído à chave
	t.Run("TemplateAusenteIdentificadoPelaChave", func(t *testing.T) {
		dbPath, closeDB := criarBancoSnapshot(t, false)
		defer closeDB()
		m := &Manifesto{
			Meta: MetaManifesto{
				Versao:  1,
				BatchID: "batch-template-falta",
			},
			Itens: []ItemManifesto{
				{
					HinoID:         1,
					ColetaneaID:    1,
					Codigo:         "CC",
					Numero:         1,
					TituloEsperado: "Hino Um",
					Letra:          "Letra do hino",
				},
			},
		}
		res, err := ValidarManifesto(context.Background(), dbPath, filepath.Join(t.TempDir(), "nao_existe.pptx"), m, strings.Repeat("a", 64))
		if err != nil {
			t.Fatalf("ValidarManifesto retornou erro de topo: %v", err)
		}
		if len(res) != 1 {
			t.Fatalf("esperava 1 resultado, obtido %d", len(res))
		}
		if res[0].Chave != "CC/1" {
			t.Errorf("chave esperada CC/1, obtido %q", res[0].Chave)
		}
		if res[0].Erro == nil {
			t.Error("esperava erro para template ausente")
		}
	})
}

// TestValidarSnapshotPreenchido: manifesto explicitamente validável em cópia sem perder alvos
// quando banco já contém letras do próprio manifesto; conteúdo divergente → conflito, não sobrescrever.
func TestValidarSnapshotPreenchido(t *testing.T) {
	tpl := templateTeste(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "preenchido.db")

	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrar: %v", err)
	}

	letraCC1 := "Primeira estrofe do hino um\nLinha dois\n\n    Refrão do hino um"
	creditoVivoCC1 := "Compositor Original"

	// CC/1: já tem letra idêntica à proposta do manifesto e crédito vivo
	// CC/2: já tem letra DIVERGENTE da proposta do manifesto
	// CC/3: sem letra (vazio)
	_, err = db.Exec(`
		INSERT INTO coletanea (id, codigo, titulo) VALUES (1, 'CC', 'Cantor Cristão');
		INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado)
		VALUES
			(1, 1, 1, 'Hino Um', ?, ?, 0),
			(2, 1, 2, 'Hino Dois', 'Letra Existente e Totalmente Diferente', NULL, 0),
			(3, 1, 3, 'Hino Três', NULL, NULL, 0);
	`, letraCC1, creditoVivoCC1)
	if err != nil {
		t.Fatalf("popular dados: %v", err)
	}
	db.Close()

	propCC1Cred := "Outro Compositor Proposto"
	m := &Manifesto{
		Meta: MetaManifesto{
			Versao:           1,
			BatchID:          "batch-preenchido",
			SnapshotSHA256:   strings.Repeat("a", 64),
			CSVSHA256:        strings.Repeat("b", 64),
			ProvenanceSHA256: strings.Repeat("c", 64),
			TemplateSHA256:   strings.Repeat("d", 64),
		},
		Itens: []ItemManifesto{
			{
				HinoID:         1,
				ColetaneaID:    1,
				Codigo:         "CC",
				Numero:         1,
				TituloEsperado: "Hino Um",
				Arquivo:        "1.pptx",
				FonteID:        strings.Repeat("1", 64),
				FonteSHA256:    strings.Repeat("2", 64),
				Letra:          letraCC1,
				Creditos:       &propCC1Cred, // Proposta diferente, mas crédito vivo deve ser preservado sem falhar
			},
			{
				HinoID:         2,
				ColetaneaID:    1,
				Codigo:         "CC",
				Numero:         2,
				TituloEsperado: "Hino Dois",
				Arquivo:        "2.pptx",
				FonteID:        strings.Repeat("3", 64),
				FonteSHA256:    strings.Repeat("4", 64),
				Letra:          "Letra Proposta Nova Para O Hino Dois",
			},
			{
				HinoID:         3,
				ColetaneaID:    1,
				Codigo:         "CC",
				Numero:         3,
				TituloEsperado: "Hino Três",
				Arquivo:        "3.pptx",
				FonteID:        strings.Repeat("5", 64),
				FonteSHA256:    strings.Repeat("6", 64),
				Letra:          "Letra Para O Hino Tres Sem Conteudo Antes",
			},
		},
	}
	manifestSHA := strings.Repeat("7", 64)

	res, err := ValidarManifesto(context.Background(), dbPath, tpl, m, manifestSHA)
	if err != nil {
		t.Fatalf("ValidarManifesto: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("esperava 3 resultados, obtido %d", len(res))
	}

	// 1. CC/1: Banco já continha letra do próprio manifesto -> Validado com sucesso sem perder alvo!
	if res[0].Chave != "CC/1" || res[0].Erro != nil || res[0].Slides < 2 {
		t.Errorf("CC/1 deveria validar com sucesso: %+v", res[0])
	}

	// 2. CC/2: Banco continha letra divergente -> Conflito! Erro identificado pela chave
	if res[1].Chave != "CC/2" || res[1].Erro == nil {
		t.Errorf("CC/2 com letra divergente deveria ter erro de conflito: %+v", res[1])
	}

	// 3. CC/3: Banco estava sem letra -> Importado e validado com sucesso
	if res[2].Chave != "CC/3" || res[2].Erro != nil || res[2].Slides < 2 {
		t.Errorf("CC/3 deveria validar com sucesso: %+v", res[2])
	}

	// Confere que banco original não teve nenhuma das letras alteradas
	origCheck, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("reabrir db original: %v", err)
	}
	defer origCheck.Close()

	var l1, l2 string
	var l3 *string
	var c1 string
	_ = origCheck.QueryRow("SELECT letra, creditos FROM hino WHERE id = 1").Scan(&l1, &c1)
	_ = origCheck.QueryRow("SELECT letra FROM hino WHERE id = 2").Scan(&l2)
	_ = origCheck.QueryRow("SELECT letra FROM hino WHERE id = 3").Scan(&l3)

	if l1 != letraCC1 || c1 != creditoVivoCC1 {
		t.Errorf("CC/1 original alterado! l1=%q c1=%q", l1, c1)
	}
	if l2 != "Letra Existente e Totalmente Diferente" {
		t.Errorf("CC/2 original alterado! l2=%q", l2)
	}
	if l3 != nil {
		t.Errorf("CC/3 original não deveria ter letra, obtido %v", *l3)
	}
}

// TestValidarLimitesConservadores_Gates64e10 exercita limites máximos de 64 runas por linha e 10 linhas por bloco
func TestValidarLimitesConservadores_Gates64e10(t *testing.T) {
	tpl := templateTeste(t)
	dbPath, closeDB := criarBancoSnapshot(t, false)
	defer closeDB()

	// Bloco com exatamente 10 linhas, cada uma com 64 runas
	var linhas []string
	for i := 1; i <= 10; i++ {
		linhas = append(linhas, fmt.Sprintf("%02d %s", i, strings.Repeat("A", 61)))
	}
	letraLimite := strings.Join(linhas, "\n")

	m := &Manifesto{
		Meta: MetaManifesto{
			Versao:  1,
			BatchID: "batch-limites",
		},
		Itens: []ItemManifesto{
			{
				HinoID:         1,
				ColetaneaID:    1,
				Codigo:         "CC",
				Numero:         1,
				TituloEsperado: "Hino Um",
				Letra:          letraLimite,
			},
		},
	}
	res, err := ValidarManifesto(context.Background(), dbPath, tpl, m, strings.Repeat("a", 64))
	if err != nil {
		t.Fatalf("ValidarManifesto: %v", err)
	}
	if len(res) != 1 || res[0].Erro != nil || res[0].Slides != 2 {
		t.Fatalf("esperava sucesso para limites conservadores de 64/10: %+v", res)
	}
}
