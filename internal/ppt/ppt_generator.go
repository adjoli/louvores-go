package ppt

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/adjoli/louvores-go/internal/domain"
)

// Caminhos das partes OOXML manipuladas pelo gerador.
const (
	presFile         = "ppt/presentation.xml"
	contentTypes     = "[Content_Types].xml"
	presentationRels = "ppt/_rels/presentation.xml.rels"
	slidesPrefix     = "ppt/slides/slide"
	slidesRelsPrefix = "ppt/slides/_rels/slide"
	titleSlidePart   = "ppt/slides/slide1.xml"
)

// contentTypesSlide é o ContentType de uma parte de slide.
const contentTypesSlide = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"

// relTypeSlide é o Type das relações de slide.
const relTypeSlide = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide"

// relTypeSlideLayout é o Type das relações slide→layout.
const relTypeSlideLayout = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout"

type slideLayoutInfo struct {
	index   int
	name    string
	content []byte
}

type layoutPlaceholderInfo struct {
	titleIdx  int
	bodyIdx   int
	footerIdx int
}

type generatedSlide struct {
	fileName string // ex.: ppt/slides/slide2.xml
	content  []byte
	rels     []byte // conteúdo do .rels (slide → layout)
}

// GenerateSlides constrói a apresentação preservando todas as partes do
// template e apenas acrescentando/registrando os slides novos de forma
// consistente (Content_Types, relações e sldIdLst).
func GenerateSlides(titulo, creditos string, seq domain.SequenciaHino, templatePath string) ([]byte, error) {
	zr, err := zip.OpenReader(templatePath)
	if err != nil {
		return nil, fmt.Errorf("abrir template: %w", err)
	}
	defer zr.Close()

	// 1. Lê todos os arquivos do template, mantendo o conteúdo original.
	files := make(map[string][]byte)
	var layouts []slideLayoutInfo

	for _, f := range zr.File {
		content, err := readZipEntry(f)
		if err != nil {
			return nil, fmt.Errorf("ler %s: %w", f.Name, err)
		}
		files[f.Name] = content

		if idx := parseLayoutIndex(f.Name); idx >= 0 {
			layouts = append(layouts, slideLayoutInfo{index: idx, name: f.Name, content: content})
		}
	}

	// 2. Detecta os índices dos placeholders (body e rodapé) de cada layout.
	placeholders := make(map[int]layoutPlaceholderInfo)
	for _, l := range layouts {
		info, err := parseSlideLayoutPlaceholders(l.content)
		if err != nil {
			return nil, fmt.Errorf("parse layout %d: %w", l.index, err)
		}
		placeholders[l.index] = info
	}

	// 3. Aloca identificadores sem colidir com os existentes.
	nextSlideNum, err := nextSlideNumber(files)
	if err != nil {
		return nil, err
	}
	nextRelID, err := nextRelID(files[presentationRels])
	if err != nil {
		return nil, err
	}
	nextSldID, err := nextSldID(files[presFile])
	if err != nil {
		return nil, err
	}

	// 4. Slide de título: reutiliza o slide1.xml do template, injetando o texto.
	title, err := buildTitleSlide(files[titleSlidePart], titulo, creditos)
	if err != nil {
		return nil, fmt.Errorf("build título: %w", err)
	}
	files[titleSlidePart] = title

	// 5. Slides de conteúdo.
	for i, parte := range seq.Partes {
		layoutIdx := LayoutRefrao
		if parte.Tipo == domain.TipoParteEstrofe {
			layoutIdx = LayoutEstrofe
		}
		ph := placeholders[layoutIdx]
		slideNum := nextSlideNum + i
		slide := buildContentSlide(parte, titulo, len(seq.Partes), slideNum, layoutIdx, ph)

		files[slide.fileName] = slide.content
		files[slideRelsName(slideNum)] = slide.rels

		// Registra a relação na presentation.xml.rels.
		files[presentationRels] = appendRel(files[presentationRels],
			fmt.Sprintf(`<Relationship Id="rId%d" Type="%s" Target="slides/slide%d.xml"/>`,
				nextRelID+i, relTypeSlide, slideNum))

		// Registra o Override no [Content_Types].xml.
		files[contentTypes] = appendOverride(files[contentTypes],
			fmt.Sprintf(`<Override PartName="/ppt/slides/slide%d.xml" ContentType="%s"/>`,
				slideNum, contentTypesSlide))

		// Registra o sldId no presentation.xml.
		files[presFile] = appendSldID(files[presFile], nextSldID+i, nextRelID+i)
	}

	// 6. Grava o pacote completo.
	return writeZip(files)
}

// buildTitleSlide injeta o título e os créditos nos placeholders do slide de
// título do template, preservando o restante do XML intocado.
func buildTitleSlide(slide []byte, titulo, creditos string) ([]byte, error) {
	if slide == nil {
		return nil, fmt.Errorf("slide de título (slide1.xml) ausente no template")
	}
	out := string(slide)

	ctrAnchor := `<p:ph type="ctrTitle"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr dirty="0"/></a:p>`
	if !strings.Contains(out, ctrAnchor) {
		return nil, fmt.Errorf("placeholder ctrTitle não encontrado no slide de título")
	}
	out = strings.Replace(out, ctrAnchor,
		`<p:ph type="ctrTitle"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/>`+paragraph(titulo), 1)

	if creditos != "" {
		subAnchor := `<p:ph type="subTitle" idx="1"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr dirty="0"/></a:p>`
		if !strings.Contains(out, subAnchor) {
			return nil, fmt.Errorf("placeholder subTitle não encontrado no slide de título")
		}
		out = strings.Replace(out, subAnchor,
			`<p:ph type="subTitle" idx="1"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/>`+paragraph(creditos), 1)
	}

	return []byte(out), nil
}

// buildContentSlide monta um slide de conteúdo (estrofe/refrão) que herda a
// formatação do layout referenciado, preenchendo título, body e rodapé.
func buildContentSlide(parte domain.ParteHino, titulo string, total, slideNum, layoutIdx int, ph layoutPlaceholderInfo) generatedSlide {
	title := fmt.Sprintf(`<p:ph type="title" idx="%d"/>`, ph.titleIdx)
	body := fmt.Sprintf(`<p:ph type="body" idx="%d"/>`, ph.bodyIdx)
	footer := fmt.Sprintf(`<p:ph type="body" sz="quarter" idx="%d"/>`, ph.footerIdx)

	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
		`xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Título"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr>` + title + `</p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/>` +
		paragraph(titulo) +
		`</p:txBody></p:sp>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Conteúdo"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr>` + body + `</p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/>` +
		paragraphs(parte.Txt) +
		`</p:txBody></p:sp>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="4" name="Rodapé"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr>` + footer + `</p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/>` +
		paragraph(parte.Rodape(total)) +
		`</p:txBody></p:sp>` +
		`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`

	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="` + relTypeSlideLayout + `" Target="../slideLayouts/slideLayout` + itoa(layoutIdx) + `.xml"/>` +
		`</Relationships>`

	return generatedSlide{
		fileName: slideName(slideNum),
		content:  []byte(xml),
		rels:     []byte(rels),
	}
}

// paragraph gera um <a:p> único, convertendo quebras de linha em <a:br/>.
func paragraph(text string) string {
	lines := strings.Split(text, "\n")
	var b strings.Builder
	b.WriteString("<a:p>")
	for i, ln := range lines {
		if i > 0 {
			b.WriteString("<a:br/>")
		}
		if ln != "" {
			b.WriteString(`<a:r><a:t xml:space="preserve">`)
			escapeXMLText(&b, ln)
			b.WriteString(`</a:t></a:r>`)
		}
	}
	b.WriteString("</a:p>")
	return b.String()
}

// paragraphs gera um <a:p> por linha (usado no body das estrofes/refrões).
func paragraphs(text string) string {
	lines := strings.Split(text, "\n")
	var b strings.Builder
	for _, ln := range lines {
		b.WriteString("<a:p>")
		if ln != "" {
			b.WriteString(`<a:r><a:t xml:space="preserve">`)
			escapeXMLText(&b, ln)
			b.WriteString(`</a:t></a:r>`)
		}
		b.WriteString("</a:p>")
	}
	return b.String()
}

func escapeXMLText(w io.Writer, s string) {
	_ = xml.EscapeText(w, []byte(s))
}

func parseLayoutIndex(name string) int {
	if !strings.HasPrefix(name, "ppt/slideLayouts/slideLayout") || !strings.HasSuffix(name, ".xml") {
		return -1
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(name, "ppt/slideLayouts/slideLayout"), ".xml")
	n, err := strconv.Atoi(mid)
	if err != nil {
		return -1
	}
	return n
}

// parseSlideLayoutPlaceholders extrai os índices dos placeholders de título,
// body e rodapé de um layout usando parsing XML de verdade (não regex).
func parseSlideLayoutPlaceholders(data []byte) (layoutPlaceholderInfo, error) {
	var info layoutPlaceholderInfo
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return info, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "ph" {
			continue
		}
		phType, phIdx, sz := "", "", ""
		for _, a := range se.Attr {
			switch a.Name.Local {
			case "type":
				phType = a.Value
			case "idx":
				phIdx = a.Value
			case "sz":
				sz = a.Value
			}
		}
		switch {
		case phType == "body" && sz == "quarter":
			info.footerIdx, _ = strconv.Atoi(phIdx)
		case phType == "body":
			info.bodyIdx, _ = strconv.Atoi(phIdx)
		case phType == "title":
			// Placeholder de título sem índice explícito equivale a idx=0.
			info.titleIdx, _ = strconv.Atoi(phIdx)
		}
	}
	return info, nil
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func slideName(n int) string     { return slidesPrefix + itoa(n) + ".xml" }
func slideRelsName(n int) string { return slidesRelsPrefix + itoa(n) + ".xml.rels" }

// nextSlideNumber determina o próximo número de slide (slideN.xml) existente.
func nextSlideNumber(files map[string][]byte) (int, error) {
	max := 0
	for name := range files {
		if !strings.HasPrefix(name, slidesPrefix) || !strings.HasSuffix(name, ".xml") {
			continue
		}
		mid := strings.TrimSuffix(strings.TrimPrefix(name, slidesPrefix), ".xml")
		n, err := strconv.Atoi(mid)
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return max + 1, nil
}

var relIDRe = regexp.MustCompile(`Id="rId(\d+)"`)

// nextRelID determina o próximo rId livre na presentation.xml.rels.
func nextRelID(rels []byte) (int, error) {
	max := 0
	for _, m := range relIDRe.FindAllStringSubmatch(string(rels), -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n > max {
			max = n
		}
	}
	return max + 1, nil
}

var sldIDRe = regexp.MustCompile(`<p:sldId id="(\d+)"`)

// nextSldID determina o próximo id de slide livre no presentation.xml.
func nextSldID(pres []byte) (int, error) {
	max := 0
	for _, m := range sldIDRe.FindAllStringSubmatch(string(pres), -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n > max {
			max = n
		}
	}
	if max == 0 {
		max = 256
	}
	return max + 1, nil
}

// appendRel insere uma <Relationship> antes do fechamento do <Relationships>.
func appendRel(rels []byte, rel string) []byte {
	return insertBefore(rels, "</Relationships>", rel)
}

// appendOverride insere um <Override> antes do fechamento do <Types>.
func appendOverride(ct []byte, override string) []byte {
	return insertBefore(ct, "</Types>", override)
}

// appendSldID insere um <p:sldId> antes do fechamento do <p:sldIdLst>.
func appendSldID(pres []byte, id, relID int) []byte {
	return insertBefore(pres, "</p:sldIdLst>",
		fmt.Sprintf(`<p:sldId id="%d" r:id="rId%d"/>`, id, relID))
}

// insertBefore insere element logo antes de anchor (última ocorrência).
func insertBefore(src []byte, anchor, element string) []byte {
	s := string(src)
	idx := strings.LastIndex(s, anchor)
	if idx < 0 {
		return src
	}
	return []byte(s[:idx] + element + s[idx:])
}

// writeZip grava o pacote completo preservando todos os arquivos do template.
func writeZip(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(files[name]); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func itoa(n int) string { return strconv.Itoa(n) }
