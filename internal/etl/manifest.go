package etl

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/adjoli/louvores-go/internal/models"
	"github.com/adjoli/louvores-go/internal/processors"
	"github.com/adjoli/louvores-go/internal/repository"
	"github.com/adjoli/louvores-go/internal/services"
)

var blocoRegex = regexp.MustCompile(`\n\s*\n`)

// MetaManifesto traz os metadados imutáveis e digests das entradas de importação.
type MetaManifesto struct {
	Versao           int    `json:"versao"`
	BatchID          string `json:"batch_id"`
	SnapshotSHA256   string `json:"snapshot_sha256"`
	CSVSHA256        string `json:"csv_sha256"`
	ProvenanceSHA256 string `json:"provenance_sha256"`
	TemplateSHA256   string `json:"template_sha256"`
}

// ItemManifesto representa um hino elegível congelado no manifesto.
type ItemManifesto struct {
	HinoID         int64   `json:"hino_id"`
	ColetaneaID    int64   `json:"coletanea_id"`
	Codigo         string  `json:"codigo"`
	Numero         int     `json:"numero"`
	TituloEsperado string  `json:"titulo_esperado"`
	Arquivo        string  `json:"arquivo"`
	FonteID        string  `json:"fonte_id"`
	FonteSHA256    string  `json:"fonte_sha256"`
	Letra          string  `json:"letra"`
	Creditos       *string `json:"creditos"`
}

// Manifesto agrega os metadados e os itens elegíveis ordenados canonicamente.
type Manifesto struct {
	Meta  MetaManifesto   `json:"meta"`
	Itens []ItemManifesto `json:"itens"`
}

// HashFinal calcula o digest SHA-256 canônico conforme a especificação:
// SHA256(UTF8(codigo) || NUL || ASCII(numero_decimal) || NUL || UTF8(titulo_destino) || NUL || UTF8(letra_final) || NUL || credito_tipado)
// onde credito_tipado é byte 'N' se creditos == nil, ou byte 'S' seguido dos bytes UTF-8 se creditos != nil.
func HashFinal(codigo string, numero int, titulo, letra string, creditos *string) (string, error) {
	if numero <= 0 {
		return "", fmt.Errorf("número %d inválido", numero)
	}
	if strings.ContainsRune(codigo, 0) ||
		strings.ContainsRune(titulo, 0) ||
		strings.ContainsRune(letra, 0) ||
		(creditos != nil && strings.ContainsRune(*creditos, 0)) {
		return "", errors.New("campo contém byte NUL")
	}
	var buf bytes.Buffer
	buf.WriteString(codigo)
	buf.WriteByte(0)
	buf.WriteString(strconv.Itoa(numero))
	buf.WriteByte(0)
	buf.WriteString(titulo)
	buf.WriteByte(0)
	buf.WriteString(letra)
	buf.WriteByte(0)
	if creditos == nil {
		buf.WriteByte('N')
	} else {
		buf.WriteByte('S')
		buf.WriteString(*creditos)
	}
	h := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(h[:]), nil
}

type reciboProvenance struct {
	Version      int     `json:"version"`
	SourceID     string  `json:"source_id"`
	SourceSHA256 string  `json:"source_sha256"`
	State        string  `json:"state"`
	Codigo       string  `json:"codigo"`
	Numero       int     `json:"numero"`
	Titulo       string  `json:"titulo"`
	Letra        string  `json:"letra"`
	Creditos     *string `json:"creditos"`
	FinalSHA256  *string `json:"final_sha256"`
}

// ValidarRecibos confere que cada linha OK do CSV tem um recibo único e
// válido em provenancePath, correspondente em source_id/hash, state OK,
// destino/título e final_sha256 idêntico ao HashFinal da linha.
func ValidarRecibos(linhas []Linha, provenancePath string) error {
	f, err := os.Open(provenancePath)
	if err != nil {
		return fmt.Errorf("abrir provenance: %w", err)
	}
	defer f.Close()

	recibos := make(map[string]reciboProvenance)
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		txt := strings.TrimSpace(scanner.Text())
		if txt == "" {
			continue
		}
		var r reciboProvenance
		if err := json.Unmarshal([]byte(txt), &r); err != nil {
			return fmt.Errorf("provenance linha %d: json inválido: %w", lineNum, err)
		}
		if r.SourceID == "" {
			return fmt.Errorf("provenance linha %d: source_id vazio", lineNum)
		}
		if _, dup := recibos[r.SourceID]; dup {
			return fmt.Errorf("provenance linha %d: source_id %q duplicado", lineNum, r.SourceID)
		}
		recibos[r.SourceID] = r
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("ler provenance: %w", err)
	}

	usados := make(map[string]int)
	for _, l := range linhas {
		if !strings.EqualFold(strings.TrimSpace(l.Status), "OK") {
			continue
		}
		if l.FonteID == "" {
			return fmt.Errorf("linha %d: fonte_id vazio para status OK", l.NumLinha)
		}
		if l.FonteSHA256 == "" {
			return fmt.Errorf("linha %d: fonte_sha256 vazio para status OK", l.NumLinha)
		}
		if prevLinha, dup := usados[l.FonteID]; dup {
			return fmt.Errorf("linha %d: fonte_id %q já utilizado na linha %d", l.NumLinha, l.FonteID, prevLinha)
		}
		usados[l.FonteID] = l.NumLinha

		rec, ok := recibos[l.FonteID]
		if !ok {
			return fmt.Errorf("linha %d: recibo não encontrado para fonte_id %q", l.NumLinha, l.FonteID)
		}
		if !strings.EqualFold(strings.TrimSpace(rec.State), "OK") {
			return fmt.Errorf("linha %d: recibo com state %q, esperado 'OK'", l.NumLinha, rec.State)
		}
		if rec.SourceSHA256 != l.FonteSHA256 {
			return fmt.Errorf("linha %d: fonte_sha256 divergente (csv=%s, recibo=%s)", l.NumLinha, l.FonteSHA256, rec.SourceSHA256)
		}
		if !strings.EqualFold(strings.TrimSpace(rec.Codigo), strings.TrimSpace(l.Coletanea)) {
			return fmt.Errorf("linha %d: código divergente (csv=%s, recibo=%s)", l.NumLinha, l.Coletanea, rec.Codigo)
		}
		if rec.Numero != l.Numero {
			return fmt.Errorf("linha %d: número divergente (csv=%d, recibo=%d)", l.NumLinha, l.Numero, rec.Numero)
		}
		if rec.Titulo != l.Titulo {
			return fmt.Errorf("linha %d: título divergente (csv=%q, recibo=%q)", l.NumLinha, l.Titulo, rec.Titulo)
		}
		hFinal, err := HashFinal(l.Coletanea, l.Numero, l.Titulo, l.Letra, l.Creditos)
		if err != nil {
			return fmt.Errorf("linha %d: erro ao calcular hash final: %w", l.NumLinha, err)
		}
		if rec.FinalSHA256 == nil || *rec.FinalSHA256 != hFinal {
			recHash := "<nil>"
			if rec.FinalSHA256 != nil {
				recHash = *rec.FinalSHA256
			}
			return fmt.Errorf("linha %d: final_sha256 divergente (calculado=%s, recibo=%s)", l.NumLinha, hFinal, recHash)
		}
	}
	return nil
}

// PrepararManifesto valida cada candidato elegível contra o banco e gera o
// manifesto ordenado canonicamente. Candidatos já revisados ou que já possuem
// conteúdo na letra são pulados. Chaves duplicadas, ausentes, ambíguas ou
// títulos divergentes barram o manifesto inteiro.
func PrepararManifesto(ctx context.Context, db *sql.DB, linhas []Linha, meta MetaManifesto) (*Manifesto, error) {
	if meta.Versao == 0 {
		meta.Versao = 1
	}
	if meta.Versao != 1 {
		return nil, fmt.Errorf("versão do manifesto %d inválida, esperado 1", meta.Versao)
	}
	if strings.TrimSpace(meta.BatchID) == "" {
		return nil, errors.New("batch_id vazio no metadata do manifesto")
	}
	if !ehSHA256Hex(meta.SnapshotSHA256) ||
		!ehSHA256Hex(meta.CSVSHA256) ||
		!ehSHA256Hex(meta.ProvenanceSHA256) ||
		!ehSHA256Hex(meta.TemplateSHA256) {
		return nil, errors.New("hash inválido no metadata do manifesto")
	}

	repo := repository.NewImportRepository(db)
	itens := make([]ItemManifesto, 0)
	vistas := make(map[string]int)
	fontesVistas := make(map[string]int)
	hinosVistos := make(map[int64]int)

	for _, l := range linhas {
		if !strings.EqualFold(strings.TrimSpace(l.Status), "OK") {
			continue
		}
		cod := strings.ToUpper(strings.TrimSpace(l.Coletanea))
		chave := fmt.Sprintf("%s/%d", cod, l.Numero)
		if prev, dup := vistas[chave]; dup {
			return nil, fmt.Errorf("chave %s duplicada no CSV (linhas %d e %d): %w", chave, prev, l.NumLinha, ErrChaveDuplicada)
		}
		vistas[chave] = l.NumLinha

		if !ehSHA256Hex(l.FonteID) {
			return nil, fmt.Errorf("linha %d: fonte_id %q inválido (esperado SHA256 hex)", l.NumLinha, l.FonteID)
		}
		if !ehSHA256Hex(l.FonteSHA256) {
			return nil, fmt.Errorf("linha %d: fonte_sha256 %q inválido (esperado SHA256 hex)", l.NumLinha, l.FonteSHA256)
		}
		if prev, dup := fontesVistas[l.FonteID]; dup {
			return nil, fmt.Errorf("linha %d: fonte_id %q duplicado (linha %d)", l.NumLinha, l.FonteID, prev)
		}
		fontesVistas[l.FonteID] = l.NumLinha

		destino, err := repo.LerDestino(ctx, cod, l.Numero)
		if err != nil {
			return nil, fmt.Errorf("linha %d: destino %s: %w", l.NumLinha, chave, err)
		}
		if l.Titulo != destino.Titulo {
			return nil, fmt.Errorf("linha %d: título divergente para %s: csv=%q db=%q", l.NumLinha, chave, l.Titulo, destino.Titulo)
		}

		// Candidatos revisados ou com letra não vazia são pulados
		if destino.Revisado || !models.SemConteudo(destino.Letra) {
			continue
		}

		// A letra deve já estar normalizada por TitularLetra (idempotente)
		titulada := services.TitularLetra(l.Letra)
		if titulada != l.Letra {
			return nil, fmt.Errorf("linha %d: letra de %s não está normalizada com TitularLetra", l.NumLinha, chave)
		}

		// Validação de limites conservadores (64 runas/linha, 10 linhas/bloco)
		if err := validarLimitesLetra(l.Letra); err != nil {
			return nil, fmt.Errorf("linha %d: limites de letra para %s: %w", l.NumLinha, chave, err)
		}

		// Valida que o hash final pode ser calculado
		if _, err := HashFinal(cod, l.Numero, l.Titulo, l.Letra, l.Creditos); err != nil {
			return nil, fmt.Errorf("linha %d: hash final para %s: %w", l.NumLinha, chave, err)
		}

		if prev, dup := hinosVistos[destino.ID]; dup {
			return nil, fmt.Errorf("linha %d: hino_id %d duplicado no manifesto (linha %d)", l.NumLinha, destino.ID, prev)
		}
		hinosVistos[destino.ID] = l.NumLinha

		itens = append(itens, ItemManifesto{
			HinoID:         destino.ID,
			ColetaneaID:    destino.ColetaneaID,
			Codigo:         cod,
			Numero:         l.Numero,
			TituloEsperado: destino.Titulo,
			Arquivo:        l.Arquivo,
			FonteID:        l.FonteID,
			FonteSHA256:    l.FonteSHA256,
			Letra:          l.Letra,
			Creditos:       l.Creditos,
		})
	}

	sort.Slice(itens, func(i, j int) bool {
		if itens[i].Codigo != itens[j].Codigo {
			return itens[i].Codigo < itens[j].Codigo
		}
		if itens[i].Numero != itens[j].Numero {
			return itens[i].Numero < itens[j].Numero
		}
		return itens[i].HinoID < itens[j].HinoID
	})

	return &Manifesto{
		Meta:  meta,
		Itens: itens,
	}, nil
}

// SalvarManifesto grava o manifesto em JSON indentado.
// Retorna os.ErrExist se o arquivo já existir (não sobrescreve).
func SalvarManifesto(path string, m *Manifesto) error {
	if m == nil {
		return errors.New("manifesto nulo")
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Close()
}

// LerManifesto lê o manifesto em path e retorna o struct parseado junto
// com o digest SHA-256 exato dos bytes do arquivo.
func LerManifesto(path string) (*Manifesto, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	h := sha256.Sum256(data)
	digest := hex.EncodeToString(h[:])
	var m Manifesto
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, "", err
	}
	return &m, digest, nil
}

// InspecionarManifesto formata a estrutura de blocos e tipos de estrofe/refrão
// de itens do manifesto para conferência visual. Aceita "all" ou a chave "CODIGO/NUM".
func InspecionarManifesto(m *Manifesto, alvo string) (string, error) {
	if m == nil {
		return "", errors.New("manifesto nulo")
	}
	if alvo == "all" {
		var sb strings.Builder
		for _, it := range m.Itens {
			chave := fmt.Sprintf("%s/%d", it.Codigo, it.Numero)
			seq := processors.ProcessarHino(it.Letra)
			fmt.Fprintf(&sb, "%s: %d blocos\n", chave, len(seq.Partes))
			for _, p := range seq.Partes {
				primeira := strings.SplitN(p.Txt, "\n", 2)[0]
				if r := []rune(primeira); len(r) > 50 {
					primeira = string(r[:50])
				}
				fmt.Fprintf(&sb, "  [%s] %s...\n", p.Tipo, primeira)
			}
		}
		return sb.String(), nil
	}

	partes := strings.SplitN(alvo, "/", 2)
	if len(partes) != 2 {
		return "", fmt.Errorf("alvo no formato CODIGO/NUM ou 'all'")
	}
	num, err := strconv.Atoi(strings.TrimSpace(partes[1]))
	if err != nil || num <= 0 {
		return "", fmt.Errorf("número %q inválido: %w", partes[1], ErrLinhaInvalida)
	}
	cod := strings.ToUpper(strings.TrimSpace(partes[0]))
	for _, it := range m.Itens {
		if strings.ToUpper(it.Codigo) == cod && it.Numero == num {
			chave := fmt.Sprintf("%s/%d", it.Codigo, it.Numero)
			seq := processors.ProcessarHino(it.Letra)
			var sb strings.Builder
			fmt.Fprintf(&sb, "%s: %d blocos\n", chave, len(seq.Partes))
			for _, p := range seq.Partes {
				primeira := strings.SplitN(p.Txt, "\n", 2)[0]
				if r := []rune(primeira); len(r) > 50 {
					primeira = string(r[:50])
				}
				fmt.Fprintf(&sb, "  [%s] %s...\n", p.Tipo, primeira)
			}
			return sb.String(), nil
		}
	}
	return "", fmt.Errorf("hino %s não encontrado no manifesto: %w", alvo, repository.ErrHinoNotFound)
}

func validarLimitesLetra(letra string) error {
	letra = strings.ReplaceAll(letra, "\r\n", "\n")
	letra = strings.ReplaceAll(letra, "\r", "\n")
	trimmed := strings.Trim(letra, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return errors.New("letra vazia")
	}
	blocos := blocoRegex.Split(trimmed, -1)
	if len(blocos) == 0 {
		return errors.New("nenhum bloco encontrado")
	}
	for i, bloco := range blocos {
		linhasRaw := strings.Split(bloco, "\n")
		var linhas []string
		for _, linha := range linhasRaw {
			if strings.TrimSpace(linha) != "" {
				linhas = append(linhas, linha)
			}
		}
		if len(linhas) == 0 {
			continue
		}
		if len(linhas) > 10 {
			return fmt.Errorf("bloco %d com %d linhas excede o limite de 10", i+1, len(linhas))
		}
		for j, linha := range linhas {
			conteudoLinha := strings.TrimLeft(linha, " \t")
			runas := []rune(conteudoLinha)
			if len(runas) > 64 {
				return fmt.Errorf("bloco %d linha %d com %d caracteres excede o limite de 64", i+1, j+1, len(runas))
			}
		}
	}
	return nil
}

func ehSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
