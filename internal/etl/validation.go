package etl

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/domain"
	"github.com/adjoli/louvores-go/internal/models"
	"github.com/adjoli/louvores-go/internal/processors"
	"github.com/adjoli/louvores-go/internal/repository"
	"github.com/adjoli/louvores-go/internal/services"
)

// ValidarManifesto executa a validação isolada dos slides de um manifesto em uma
// cópia descartável e consistente do snapshot SQLite (via VACUUM INTO), garantindo
// que os textos candidatos produzem pacotes PPTX íntegros sem tocar no banco original
// e sem relaxar a trava de produção (revisado=0).
func ValidarManifesto(ctx context.Context, snapshotPath, templatePath string, m *Manifesto, manifestSHA256 string) ([]Validacao, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("manifesto nulo")
	}
	if !ehSHA256Hex(manifestSHA256) {
		return nil, fmt.Errorf("manifest_sha256 %q inválido, esperado SHA-256 hex de 64 caracteres", manifestSHA256)
	}
	if strings.TrimSpace(snapshotPath) == "" {
		return nil, errors.New("caminho do snapshot vazio")
	}
	if _, err := os.Stat(snapshotPath); err != nil {
		return nil, fmt.Errorf("snapshot não encontrado: %w", err)
	}

	// 1. Gera os artefatos SQL a partir do manifesto
	artefatos, err := GerarSQL(m, manifestSHA256)
	if err != nil {
		return nil, fmt.Errorf("gerar SQL para validação: %w", err)
	}

	// 2. Prepara arquivo temporário único em os.TempDir()
	tmpFile, err := os.CreateTemp("", "louvores-validar-*.db")
	if err != nil {
		return nil, fmt.Errorf("criar arquivo temporário: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	_ = os.Remove(tmpPath) // VACUUM INTO exige que o arquivo destino ainda não exista

	defer func() {
		_ = os.Remove(tmpPath)
		_ = os.Remove(tmpPath + "-wal")
		_ = os.Remove(tmpPath + "-shm")
	}()

	// 3. Clone consistente do snapshot via VACUUM INTO a partir de conexão read-only
	if err := clonarSnapshot(ctx, snapshotPath, tmpPath); err != nil {
		return nil, fmt.Errorf("clonar snapshot: %w", err)
	}

	// 4. Abre a cópia temporária e aplica migração
	tempDB, err := database.Open(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("abrir cópia temporária: %w", err)
	}
	defer tempDB.Close()

	if err := database.Migrate(tempDB); err != nil {
		return nil, fmt.Errorf("migrar cópia temporária: %w", err)
	}

	// 5. Lê estado pré-existente dos candidatos na cópia antes do import
	type preState struct {
		exists   bool
		codigo   string
		numero   int
		titulo   string
		letra    *string
		creditos *string
	}
	preStates := make(map[int64]preState)
	for _, item := range m.Itens {
		var ps preState
		row := tempDB.QueryRowContext(ctx, `
			SELECT c.codigo, h.numeracao, h.titulo, h.letra, h.creditos
			FROM hino h
			JOIN coletanea c ON c.id = h.coletanea_id
			WHERE h.id = ?
		`, item.HinoID)
		if err := row.Scan(&ps.codigo, &ps.numero, &ps.titulo, &ps.letra, &ps.creditos); err == nil {
			ps.exists = true
			preStates[item.HinoID] = ps
		}
	}

	// 6. Aplica import.sql na cópia temporária
	if err := execScript(ctx, tempDB, artefatos.Import); err != nil {
		return nil, fmt.Errorf("aplicar import.sql na cópia: %w", err)
	}

	// 7. Confere conteúdo final de cada candidato e coleta IDs com conteúdo correto
	candidateErrors := make(map[string]error)
	var validHinoIDs []int64

	for _, item := range m.Itens {
		chave := fmt.Sprintf("%s/%d", item.Codigo, item.Numero)
		var hID int64
		var cod string
		var num int
		var tit string
		var letra *string
		var cred *string
		var rev int

		err := tempDB.QueryRowContext(ctx, `
			SELECT h.id, c.codigo, h.numeracao, h.titulo, h.letra, h.creditos, h.revisado
			FROM hino h
			JOIN coletanea c ON c.id = h.coletanea_id
			WHERE h.id = ?
		`, item.HinoID).Scan(&hID, &cod, &num, &tit, &letra, &cred, &rev)

		if err != nil {
			candidateErrors[chave] = fmt.Errorf("hino %s: destino ausente no banco pós-import", chave)
			continue
		}
		if cod != item.Codigo || num != item.Numero || tit != item.TituloEsperado {
			candidateErrors[chave] = fmt.Errorf("hino %s: destino incompatível (esperado %s/%d %q, encontrado %s/%d %q)",
				chave, item.Codigo, item.Numero, item.TituloEsperado, cod, num, tit)
			continue
		}
		if letra == nil || *letra != item.Letra {
			candidateErrors[chave] = fmt.Errorf("hino %s: conflito de letra (conteúdo divergente no banco)", chave)
			continue
		}

		ps := preStates[item.HinoID]
		var expectedCred *string
		if ps.exists && !models.SemConteudo(ps.creditos) {
			// Crédito vivo preservado
			expectedCred = ps.creditos
		} else if item.Creditos != nil {
			// Proposta aplicada
			expectedCred = item.Creditos
		} else {
			// Sem proposta e sem crédito anterior
			expectedCred = ps.creditos
		}

		if !creditosIguais(cred, expectedCred) {
			candidateErrors[chave] = fmt.Errorf("hino %s: crédito divergente do esperado", chave)
			continue
		}

		validHinoIDs = append(validHinoIDs, item.HinoID)
	}

	// 8. Só IDs com conteúdo correto ganham revisado=1 na cópia
	for _, id := range validHinoIDs {
		if _, err := tempDB.ExecContext(ctx, "UPDATE hino SET revisado = 1 WHERE id = ?", id); err != nil {
			return nil, fmt.Errorf("marcar revisado=1 na cópia para hino %d: %w", id, err)
		}
	}

	// 9. Gera e valida slides para cada candidato do manifesto
	hinoRepo := repository.NewSQLiteHinoRepository(tempDB)
	coletRepo := repository.NewSQLiteColetaneaRepository(tempDB)
	svc := services.NewHinoService(hinoRepo, coletRepo, templatePath)

	resultados := make([]Validacao, 0, len(m.Itens))
	for _, item := range m.Itens {
		chave := fmt.Sprintf("%s/%d", item.Codigo, item.Numero)
		v := Validacao{Chave: chave}

		if cErr, ok := candidateErrors[chave]; ok {
			v.Erro = cErr
			resultados = append(resultados, v)
			continue
		}

		data, err := svc.GerarSlides(ctx, item.Codigo, item.Numero, templatePath)
		if err != nil {
			v.Erro = err
			resultados = append(resultados, v)
			continue
		}

		seq := processors.ProcessarHino(item.Letra)
		if err := validarPacote(data, seq); err != nil {
			v.Erro = err
			v.Slides, _ = contarSlides(data)
			resultados = append(resultados, v)
			continue
		}

		v.Slides = 1 + len(seq.Partes)
		v.Erro = nil
		resultados = append(resultados, v)
	}

	return resultados, nil
}

func contarSlides(data []byte) (int, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, f := range z.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			n++
		}
	}
	return n, nil
}

func clonarSnapshot(ctx context.Context, snapshotPath, destPath string) error {
	origConn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", snapshotPath))
	if err != nil {
		return fmt.Errorf("abrir snapshot read-only: %w", err)
	}
	defer origConn.Close()

	if _, err := origConn.ExecContext(ctx, "VACUUM INTO ?", destPath); err != nil {
		return fmt.Errorf("VACUUM INTO: %w", err)
	}
	return nil
}

func creditosIguais(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil {
		return models.SemConteudo(b)
	}
	if b == nil {
		return models.SemConteudo(a)
	}
	return *a == *b
}

func splitStatements(sqlText string) []string {
	var statements []string
	var current strings.Builder
	inString := false
	runes := []rune(sqlText)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if inString {
			current.WriteRune(r)
			if r == '\'' {
				if i+1 < len(runes) && runes[i+1] == '\'' {
					current.WriteRune('\'')
					i++
				} else {
					inString = false
				}
			}
		} else {
			if r == '\'' {
				inString = true
				current.WriteRune(r)
			} else if r == ';' {
				current.WriteRune(';')
				stmt := strings.TrimSpace(current.String())
				if stmt != "" && stmt != ";" {
					statements = append(statements, stmt)
				}
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		}
	}
	stmt := strings.TrimSpace(current.String())
	if stmt != "" && stmt != ";" {
		statements = append(statements, stmt)
	}
	return statements
}

func execScript(ctx context.Context, db *sql.DB, script string) error {
	stmts := splitStatements(script)
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			_, _ = db.Exec("ROLLBACK;")
			return fmt.Errorf("executar instrução SQL: %w\nSQL: %s", err, stmt)
		}
	}
	return nil
}

// -------------------------------------------------------------------------
// Validação OOXML
// -------------------------------------------------------------------------

const (
	relTypeSlide            = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide"
	relTypeSlideLayout      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout"
	contentTypePresentation = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
	contentTypeSlide        = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"
	contentTypeSlideLayout  = "application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"
)

type xmlTypes struct {
	Overrides []xmlOverride `xml:"Override"`
}

type xmlOverride struct {
	PartName    string `xml:"PartName,attr"`
	ContentType string `xml:"ContentType,attr"`
}

type xmlRelationships struct {
	Relationships []xmlRelationship `xml:"Relationship"`
}

type xmlRelationship struct {
	ID     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
}

type xmlPresentation struct {
	SldIdLst struct {
		SldIds []xmlSldId `xml:"sldId"`
	} `xml:"sldIdLst"`
}

type xmlSldId struct {
	ID  string
	RID string
}

func (s *xmlSldId) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for _, a := range start.Attr {
		if a.Name.Local == "id" && a.Name.Space == "" {
			s.ID = a.Value
		} else if a.Name.Local == "id" && (a.Name.Space == "http://schemas.openxmlformats.org/officeDocument/2006/relationships" || a.Name.Space == "r") {
			s.RID = a.Value
		}
	}
	return d.Skip()
}

type xmlParagraph struct {
	Text string
}

func (p *xmlParagraph) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var sb strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "br" {
				sb.WriteByte('\n')
			}
		case xml.CharData:
			sb.Write(t)
		case xml.EndElement:
			depth--
		}
	}
	p.Text = sb.String()
	return nil
}

type xmlShape struct {
	NvSpPr struct {
		NvPr struct {
			Ph struct {
				Type string `xml:"type,attr"`
				Sz   string `xml:"sz,attr"`
				Idx  string `xml:"idx,attr"`
			} `xml:"ph"`
		} `xml:"nvPr"`
	} `xml:"nvSpPr"`
	TxBody struct {
		Paragraphs []xmlParagraph `xml:"p"`
	} `xml:"txBody"`
}

type xmlSlide struct {
	CSld struct {
		SpTree struct {
			Shapes []xmlShape `xml:"sp"`
		} `xml:"spTree"`
	} `xml:"cSld"`
}

func resolvePartURI(sourcePart string, relTarget string) string {
	dir := path.Dir(sourcePart)
	if !strings.HasPrefix(dir, "/") {
		dir = "/" + dir
	}
	return path.Clean(path.Join(dir, relTarget))
}

func zipEntryName(partURI string) string {
	return strings.TrimPrefix(partURI, "/")
}

// validarXMLCompleto consome até EOF: Unmarshal sozinho pode ignorar conteúdo
// após a raiz, e Token permite múltiplas raízes e texto fora delas.
func validarXMLCompleto(data []byte) error {
	d := xml.NewDecoder(bytes.NewReader(data))
	depth, roots := 0, 0
	for {
		tok, err := d.Token()
		if err == io.EOF {
			if roots != 1 {
				return errors.New("XML deve conter exatamente um elemento raiz")
			}
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots > 1 {
					return errors.New("XML contém múltiplos elementos raiz")
				}
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && len(bytes.Trim(t, " \t\r\n")) != 0 {
				return errors.New("XML contém texto fora do elemento raiz")
			}
		}
	}
}

// validarPacote valida a consistência completa do pacote OOXML gerado a partir
// da sequência de partes de um hino, garantindo integridade estrutural byte-a-byte,
// relações, partes, layouts, correspondência de texto/ordem e rodapé.
func validarPacote(data []byte, seq domain.SequenciaHino) error {
	if len(data) == 0 {
		return errors.New("pacote vazio")
	}

	// 1. Abre pacote ZIP e lê todas as entradas conferindo integridade (e CRC)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("abrir zip: %w", err)
	}

	files := make(map[string][]byte)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("abrir entrada zip %s: %w", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return fmt.Errorf("ler entrada zip %s (falha de CRC ou descompressão): %w", f.Name, err)
		}
		if strings.HasSuffix(f.Name, ".xml") || strings.HasSuffix(f.Name, ".rels") {
			if err := validarXMLCompleto(b); err != nil {
				return fmt.Errorf("parse %s: %w", f.Name, err)
			}
		}
		files[f.Name] = b
	}

	// 2. Partes estruturais obrigatórias
	required := []string{
		"[Content_Types].xml",
		"ppt/presentation.xml",
		"ppt/_rels/presentation.xml.rels",
		"ppt/slides/slide1.xml",
		"ppt/slides/_rels/slide1.xml.rels",
	}
	for _, req := range required {
		if _, ok := files[req]; !ok {
			return fmt.Errorf("arquivo obrigatório %s ausente no pacote", req)
		}
	}

	// 3. Valida [Content_Types].xml
	var typesDoc xmlTypes
	if err := xml.Unmarshal(files["[Content_Types].xml"], &typesDoc); err != nil {
		return fmt.Errorf("parse [Content_Types].xml: %w", err)
	}
	overrides := make(map[string]string)
	for _, ov := range typesDoc.Overrides {
		overrides[ov.PartName] = ov.ContentType
	}
	if overrides["/ppt/presentation.xml"] != contentTypePresentation {
		return errors.New("presentation.xml sem Override válido em [Content_Types].xml")
	}
	if overrides["/ppt/slides/slide1.xml"] != contentTypeSlide {
		return errors.New("slide1.xml sem Override válido em [Content_Types].xml")
	}

	// 4. Valida ppt/presentation.xml
	var presDoc xmlPresentation
	if err := xml.Unmarshal(files["ppt/presentation.xml"], &presDoc); err != nil {
		return fmt.Errorf("parse ppt/presentation.xml: %w", err)
	}
	sldIds := presDoc.SldIdLst.SldIds
	expectedSlides := 1 + len(seq.Partes)
	if len(sldIds) != expectedSlides {
		return fmt.Errorf("contagem de slides em presentation.xml = %d, esperado %d", len(sldIds), expectedSlides)
	}
	if len(sldIds) < 2 {
		return fmt.Errorf("pacote com %d slide(s), esperado título + conteúdo", len(sldIds))
	}

	// 5. Valida ppt/_rels/presentation.xml.rels
	var presRelsDoc xmlRelationships
	if err := xml.Unmarshal(files["ppt/_rels/presentation.xml.rels"], &presRelsDoc); err != nil {
		return fmt.Errorf("parse ppt/_rels/presentation.xml.rels: %w", err)
	}
	presRels := make(map[string]xmlRelationship)
	for _, rel := range presRelsDoc.Relationships {
		presRels[rel.ID] = rel
	}

	for idx, s := range sldIds {
		if s.RID == "" {
			return fmt.Errorf("slide %d com r:id vazio", idx+1)
		}
		rel, ok := presRels[s.RID]
		if !ok {
			return fmt.Errorf("sldId r:id %q sem Relationship correspondente em presentation.xml.rels", s.RID)
		}
		if rel.Type != relTypeSlide {
			return fmt.Errorf("slide %d com tipo de Relationship inválido: %q", idx+1, rel.Type)
		}
		targetURI := resolvePartURI("/ppt/presentation.xml", rel.Target)
		entryName := zipEntryName(targetURI)
		if _, ok := files[entryName]; !ok {
			return fmt.Errorf("slide %s referenciado por presentation.xml.rels ausente no pacote", entryName)
		}
		if overrides[targetURI] != contentTypeSlide {
			return fmt.Errorf("slide %s sem Override válido em [Content_Types].xml", targetURI)
		}
	}

	// 6. Valida slide 1 (slide de título)
	firstRel := presRels[sldIds[0].RID]
	firstTarget := zipEntryName(resolvePartURI("/ppt/presentation.xml", firstRel.Target))
	if firstTarget != "ppt/slides/slide1.xml" {
		return fmt.Errorf("primeiro slide deve ser ppt/slides/slide1.xml, obtido %s", firstTarget)
	}
	var slide1Rels xmlRelationships
	if err := xml.Unmarshal(files["ppt/slides/_rels/slide1.xml.rels"], &slide1Rels); err != nil {
		return fmt.Errorf("parse ppt/slides/_rels/slide1.xml.rels: %w", err)
	}
	var layoutTituloFound bool
	for _, r := range slide1Rels.Relationships {
		layoutURI := resolvePartURI("/ppt/slides/slide1.xml", r.Target)
		if r.Type == relTypeSlideLayout && layoutURI == "/ppt/slideLayouts/slideLayout1.xml" {
			if _, ok := files[zipEntryName(layoutURI)]; ok && overrides[layoutURI] == contentTypeSlideLayout {
				layoutTituloFound = true
				break
			}
		}
	}
	if !layoutTituloFound {
		return errors.New("slide de título sem relação válida para slideLayout1.xml")
	}

	// 7. Valida cada slide de conteúdo
	for i, parte := range seq.Partes {
		slideNum := i + 2
		rel := presRels[sldIds[1+i].RID]
		slidePart := zipEntryName(resolvePartURI("/ppt/presentation.xml", rel.Target))
		slideRelsPart := zipEntryName(resolvePartURI(slidePart, "_rels/"+path.Base(slidePart)+".rels"))

		relsData, ok := files[slideRelsPart]
		if !ok {
			return fmt.Errorf("slide %s sem .rels (%s)", slidePart, slideRelsPart)
		}
		var sRels xmlRelationships
		if err := xml.Unmarshal(relsData, &sRels); err != nil {
			return fmt.Errorf("parse %s: %w", slideRelsPart, err)
		}

		expectedLayoutIdx := 2
		if parte.Tipo == domain.TipoParteRefrao {
			expectedLayoutIdx = 3
		}
		expectedLayoutName := fmt.Sprintf("slideLayout%d.xml", expectedLayoutIdx)

		var layoutFound bool
		for _, r := range sRels.Relationships {
			layoutURI := resolvePartURI(slidePart, r.Target)
			if r.Type == relTypeSlideLayout && layoutURI == "/ppt/slideLayouts/"+expectedLayoutName {
				if _, ok := files[zipEntryName(layoutURI)]; ok && overrides[layoutURI] == contentTypeSlideLayout {
					layoutFound = true
					break
				}
			}
		}
		if !layoutFound {
			return fmt.Errorf("slide %d (%s) sem relação válida para %s", slideNum, parte.Tipo, expectedLayoutName)
		}

		// Valida XML do slide de conteúdo
		var sDoc xmlSlide
		if err := xml.Unmarshal(files[slidePart], &sDoc); err != nil {
			return fmt.Errorf("parse %s: %w", slidePart, err)
		}

		var titleShape, bodyShape, footerShape *xmlShape
		for idx := range sDoc.CSld.SpTree.Shapes {
			sp := &sDoc.CSld.SpTree.Shapes[idx]
			phType := sp.NvSpPr.NvPr.Ph.Type
			phSz := sp.NvSpPr.NvPr.Ph.Sz
			phIdx := sp.NvSpPr.NvPr.Ph.Idx

			if phType == "title" {
				titleShape = sp
			} else if phType == "body" && (phSz == "quarter" || phIdx == "10") {
				footerShape = sp
			} else if phType == "body" {
				bodyShape = sp
			}
		}

		if titleShape == nil {
			return fmt.Errorf("slide %d sem placeholder de título", slideNum)
		}
		if len(titleShape.TxBody.Paragraphs) == 0 || strings.TrimSpace(titleShape.TxBody.Paragraphs[0].Text) == "" {
			return fmt.Errorf("slide %d com placeholder de título vazio", slideNum)
		}

		if bodyShape == nil {
			return fmt.Errorf("slide %d sem placeholder de corpo", slideNum)
		}
		var bodyLines []string
		for _, p := range bodyShape.TxBody.Paragraphs {
			bodyLines = append(bodyLines, p.Text)
		}
		actualBody := strings.Join(bodyLines, "\n")
		if actualBody != parte.Txt {
			return fmt.Errorf("slide %d: texto do corpo divergente da parte (esperado %q, obtido %q)", slideNum, parte.Txt, actualBody)
		}

		if footerShape == nil {
			return fmt.Errorf("slide %d sem placeholder de rodapé", slideNum)
		}
		footerText := ""
		if len(footerShape.TxBody.Paragraphs) > 0 {
			footerText = footerShape.TxBody.Paragraphs[0].Text
		}
		expectedFooter := parte.Rodape(len(seq.Partes))
		if footerText != expectedFooter {
			return fmt.Errorf("slide %d: rodapé divergente (esperado %q, obtido %q)", slideNum, expectedFooter, footerText)
		}
	}

	return nil
}
