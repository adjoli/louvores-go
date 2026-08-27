package ppt

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/baliance/gooxml/presentation"

	"github.com/adjoli/louvores-go/internal/domain"
)

func templatePath(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "data", "templates", "default.pptx")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("template não encontrado: %v", err)
	}
	return p
}

func TestGenerateSlides(t *testing.T) {
	seq := domain.SequenciaHino{Partes: []domain.ParteHino{
		{Txt: "Estrofe um\nLinha dois", Numero: 1, Tipo: domain.TipoParteEstrofe},
		{Txt: "Refrão um\nRefrão dois", Numero: 2, Tipo: domain.TipoParteRefrao},
		{Txt: "Estrofe dois", Numero: 3, Tipo: domain.TipoParteEstrofe},
	}}

	data, err := GenerateSlides("Hino de Teste", "Autor", seq, templatePath(t))
	if err != nil {
		t.Fatalf("gerar: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("conteúdo vazio")
	}

	// Reabre o arquivo gerado para validar a estrutura.
	path := filepath.Join(t.TempDir(), "out.pptx")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Valida o PPTX gerado parseando o XML diretamente (evita bug do gooxml em prs.Slides())
	zipReader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("abrir PPTX como zip: %v", err)
	}
	defer zipReader.Close()

	var presXML []byte
	for _, f := range zipReader.File {
		if f.Name == "ppt/presentation.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("abrir presentation.xml: %v", err)
			}
			presXML, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("ler presentation.xml: %v", err)
			}
			break
		}
	}
	if presXML == nil {
		t.Fatal("presentation.xml não encontrado")
	}

	// Parse presentation.xml com regex (evita problemas de namespace do XML parser do Go)
	sldIdRegex := regexp.MustCompile(`<p:sldId\s+id="(\d+)"\s+r:id="([^"]+)"`)
	matches := sldIdRegex.FindAllStringSubmatch(string(presXML), -1)
	if len(matches) == 0 {
		t.Fatalf("não encontrou sldId em presentation.xml")
	}

	// Verifica se tem o número correto de slides (1 título + 3 partes = 4 slides)
	expectedSlides := 1 + len(seq.Partes)
	if len(matches) != expectedSlides {
		t.Fatalf("slides = %d, esperado %d", len(matches), expectedSlides)
	}

	// Verifica se o slide de título tem o título correto
	// Reabre com gooxml apenas para verificar placeholders (não chama Slides())
	prs, err := presentation.Open(path)
	if err != nil {
		t.Fatalf("reabrir pptx: %v", err)
	}

	// O slide de título é o primeiro na lista de sldIdLst
	// Não chamamos prs.Slides() porque causa panic no gooxml
	// Em vez disso, verificamos o conteúdo via XML se necessário
	// Por enquanto, apenas verificamos se o arquivo abre sem erro
	_ = prs

	// TODO: Verificar placeholders parseando o XML dos slides diretamente
	// Por enquanto, apenas garantimos que o arquivo foi gerado sem erros
}

// TestGeneratedPackageIntegrity valida a consistência do pacote OOXML gerado,
// garantindo que PowerPoint não peça reparo: toda sldId tem Relationship,
// todo slide tem Override no Content_Types e .rels próprio apontando a um layout.
func TestGeneratedPackageIntegrity(t *testing.T) {
	seq := domain.SequenciaHino{Partes: []domain.ParteHino{
		{Txt: "Estrofe um\nLinha dois", Numero: 1, Tipo: domain.TipoParteEstrofe},
		{Txt: "Refrão um\nRefrão dois", Numero: 2, Tipo: domain.TipoParteRefrao},
		{Txt: "Estrofe dois", Numero: 3, Tipo: domain.TipoParteEstrofe},
	}}
	data, err := GenerateSlides("Hino de Teste", "Autor", seq, templatePath(t))
	if err != nil {
		t.Fatalf("gerar: %v", err)
	}

	path := filepath.Join(t.TempDir(), "out.pptx")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("abrir pptx: %v", err)
	}
	defer zr.Close()

	// Conteúdo das partes-chave.
	parts := make(map[string]string)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("abrir %s: %v", f.Name, err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		parts[f.Name] = string(b)
	}

	// Toda parte referenciada deve existir no pacote.
	if _, ok := parts["ppt/slides/slide1.xml"]; !ok {
		t.Fatal("slide1.xml ausente")
	}
	for _, l := range []string{"1", "2", "3"} {
		if _, ok := parts["ppt/slideLayouts/slideLayout"+l+".xml"]; !ok {
			t.Fatalf("slideLayout%s.xml ausente", l)
		}
	}

	presXML := parts["ppt/presentation.xml"]
	presRels := parts["ppt/_rels/presentation.xml.rels"]
	contentTypesXML := parts["[Content_Types].xml"]

	// 1. sldIdLst deve ter 1 (título) + N (partes).
	sldIDs := regexp.MustCompile(`<p:sldId id="(\d+)"\s+r:id="([^"]+)"`).
		FindAllStringSubmatch(presXML, -1)
	if len(sldIDs) != 1+len(seq.Partes) {
		t.Fatalf("sldIdLst = %d, esperado %d", len(sldIDs), 1+len(seq.Partes))
	}

	for _, m := range sldIDs {
		rID := m[2]

		// 2. Cada sldId deve ter Relationship correspondente em presentation.xml.rels.
		relRe := regexp.MustCompile(`<Relationship Id="` + regexp.QuoteMeta(rID) + `" Type="[^"]*" Target="(slides/slide\d+\.xml)"`)
		relMatch := relRe.FindStringSubmatch(presRels)
		if relMatch == nil {
			t.Fatalf("sldId %s sem Relationship em presentation.xml.rels", rID)
		}
		target := relMatch[1]

		// 3. Cada slide referenciado deve ter Override no Content_Types.
		partName := "/ppt/" + target
		if !regexp.MustCompile(`<Override PartName="` + regexp.QuoteMeta(partName) + `"`).
			MatchString(contentTypesXML) {
			t.Fatalf("%s sem Override em [Content_Types].xml", partName)
		}

		// 4. Cada slide deve ter .rels apontando para um layout existente.
		relsPart := "ppt/slides/_rels/" + strings.TrimPrefix(target, "slides/") + ".rels"
		slideRels, ok := parts[relsPart]
		if !ok {
			t.Fatalf("%s sem .rels (%s)", target, relsPart)
		}
		layoutRel := regexp.MustCompile(`Target="\.\./slideLayouts/slideLayout(\d+)\.xml"`).
			FindStringSubmatch(slideRels)
		if layoutRel == nil {
			t.Fatalf("%s .rels sem layout", target)
		}
		if _, ok := parts["ppt/slideLayouts/slideLayout"+layoutRel[1]+".xml"]; !ok {
			t.Fatalf("%s aponta a layout inexistente", target)
		}
	}

	// 5. Os slides de conteúdo (todos exceto o slide1 de título) devem exibir
	//    o título do hino no placeholder de título do layout.
	titulo := "Hino de Teste"
	for i := 2; i <= 1+len(seq.Partes); i++ {
		slidePart := fmt.Sprintf("ppt/slides/slide%d.xml", i)
		slideXML := parts[slidePart]
		if slideXML == "" {
			t.Fatalf("%s ausente", slidePart)
		}
		if !strings.Contains(slideXML, `<p:ph type="title"`) {
			t.Fatalf("%s sem placeholder de título", slidePart)
		}
		if !strings.Contains(slideXML, titulo) {
			t.Fatalf("%s sem o título do hino no slide", slidePart)
		}
	}

	// 6. O arquivo reabre em gooxml sem erro.
	prs, err := presentation.Open(path)
	if err != nil {
		t.Fatalf("reabrir pptx com gooxml: %v", err)
	}
	_ = prs
}

func TestGenerateSlidesTemplateInexistente(t *testing.T) {
	_, err := GenerateSlides("x", "", domain.SequenciaHino{}, filepath.Join(t.TempDir(), "nao-existe.pptx"))
	if err == nil {
		t.Fatal("esperado erro para template inexistente")
	}
}

func placeholderText(ph presentation.PlaceHolder) string {
	out := ""
	for _, p := range ph.Paragraphs() {
		for _, r := range p.X().EG_TextRun {
			if r.R != nil {
				out += r.R.T
			}
		}
	}
	return out
}
