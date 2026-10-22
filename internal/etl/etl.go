// Package etl concentra a lógica testável da importação assistida de
// hinos: leitura do CSV, congelamento do manifesto, exportação de SQL
// puro e validação isolada de slides.
//
// A interface com o operador vive em cmd/etl (stdlib, sem TUI).
package etl

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adjoli/louvores-go/internal/models"
	"github.com/adjoli/louvores-go/internal/repository"
)

// Erros sentinela do CSV e do snapshot.
var (
	ErrCSVVazio        = errors.New("csv vazio")
	ErrColunaAusente   = errors.New("coluna ausente")
	ErrCabecalhoDuplic = errors.New("cabeçalho duplicado")
	ErrLinhaInvalida   = errors.New("linha inválida")
	ErrBancoRecusado   = errors.New("banco recusado")
	ErrChaveDuplicada  = errors.New("chave duplicada")
	ErrDestinoAmbiguo  = repository.ErrDestinoAmbiguo
)

// Linha é uma linha da planilha de revisão ou corpus sanitizado.
type Linha struct {
	Arquivo     string
	Coletanea   string
	Numero      int
	Titulo      string
	Letra       string
	Status      string
	NumLinha    int
	Creditos    *string
	FonteID     string
	FonteSHA256 string
}

// Validacao é o resultado da geração de slides de um hino importado.
type Validacao struct {
	Chave  string
	Slides int
	Erro   error
}

// LerCSV lê a planilha de revisão (cabeçalho pasta,arquivo,hinario,numero,
// titulo,letra,status,observacao — tolera os cabeçalhos com " original" e
// BOM inicial). Linhas com número inválido viram erro contextualizado em
// vez de panic/silêncio.
func LerCSV(path string) ([]Linha, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) < 1 {
		return nil, ErrCSVVazio
	}
	idx := map[string]int{}
	for i, h := range recs[0] {
		h = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(h)), "\ufeff")
		h = strings.TrimSuffix(h, " original")
		if _, dup := idx[h]; dup {
			return nil, fmt.Errorf("cabeçalho duplicado %q: %w", h, ErrCabecalhoDuplic)
		}
		idx[h] = i
	}
	need := []string{"arquivo", "hinario", "numero", "titulo", "letra", "status"}
	for _, n := range need {
		if _, ok := idx[n]; !ok {
			return nil, fmt.Errorf("coluna %q ausente (cabeçalho: %v): %w", n, recs[0], ErrColunaAusente)
		}
	}
	credIdx, hasCred := idx["creditos"]
	if !hasCred {
		credIdx, hasCred = idx["créditos"]
	}
	srcIDIdx, hasSrcID := idx["source_id"]
	srcSHAIdx, hasSrcSHA := idx["source_sha256"]

	var out []Linha
	var elist []error
	for i, r := range recs[1:] {
		status := r[idx["status"]]
		isOK := strings.EqualFold(strings.TrimSpace(status), "OK")

		num, err := strconv.Atoi(strings.TrimSpace(r[idx["numero"]]))
		if isOK {
			if err != nil || num <= 0 {
				elist = append(elist, fmt.Errorf("linha %d: número %q inválido: %w", i+2, r[idx["numero"]], ErrLinhaInvalida))
				continue
			}
		} else {
			if err != nil || num <= 0 {
				num = 0
			}
		}

		var creditos *string
		if hasCred {
			c := r[credIdx]
			if !models.SemConteudo(&c) {
				creditos = &c
			}
		}

		var srcID string
		if hasSrcID {
			srcID = strings.TrimSpace(r[srcIDIdx])
		}

		var srcSHA string
		if hasSrcSHA {
			srcSHA = strings.TrimSpace(r[srcSHAIdx])
		}

		out = append(out, Linha{
			Arquivo:     r[idx["arquivo"]],
			Coletanea:   strings.ToUpper(strings.TrimSpace(r[idx["hinario"]])),
			Numero:      num,
			Titulo:      r[idx["titulo"]],
			Letra:       r[idx["letra"]],
			Status:      status,
			NumLinha:    i + 2,
			Creditos:    creditos,
			FonteID:     srcID,
			FonteSHA256: srcSHA,
		})
	}
	if len(elist) > 0 {
		return out, errors.Join(elist...)
	}
	return out, nil
}

// RecusarBancoProd barra produção: caminho do banco oficial, URLs
// remotas, esquemas DSN file:, banco inexistente, aliases, symlinks e hardlinks.
// O CLI só opera snapshots SQLite locais de teste consolidados.
func RecusarBancoProd(dbPath string) error {
	trimmed := strings.TrimSpace(dbPath)
	if trimmed == "" {
		return fmt.Errorf("caminho do banco vazio: %w", ErrBancoRecusado)
	}

	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{"http://", "https://", "ws://", "wss://", "libsql://", "file:"} {
		if strings.HasPrefix(lower, prefix) {
			return fmt.Errorf("recusado: remoto ou DSN inválido (%s) — este CLI é só SQLite local: %w", dbPath, ErrBancoRecusado)
		}
	}

	// Rejeita data/hinos.db direto
	cleanPath := filepath.Clean(dbPath)
	if cleanPath == filepath.Join("data", "hinos.db") {
		return fmt.Errorf("recusado: este CLI nunca escreve em data/hinos.db (use uma cópia): %w", ErrBancoRecusado)
	}

	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return err
	}
	if strings.HasSuffix(filepath.Clean(abs), string(filepath.Separator)+filepath.Join("data", "hinos.db")) {
		return fmt.Errorf("recusado: este CLI nunca escreve em data/hinos.db (use uma cópia): %w", ErrBancoRecusado)
	}

	// Verifica se o arquivo do banco existe
	stat, err := os.Stat(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("banco inexistente %q: %w", dbPath, err)
		}
		return fmt.Errorf("acessar banco %q: %w", dbPath, err)
	}
	if stat.IsDir() {
		return fmt.Errorf("banco %q é um diretório: %w", dbPath, ErrBancoRecusado)
	}

	// Compara com data/hinos.db oficial se existir no sistema de arquivos
	prodRel := filepath.Join("data", "hinos.db")
	if prodStat, err := os.Stat(prodRel); err == nil {
		if os.SameFile(stat, prodStat) {
			return fmt.Errorf("recusado: arquivo é o mesmo que data/hinos.db (hardlink/symlink): %w", ErrBancoRecusado)
		}
		prodAbs, errProdAbs := filepath.Abs(prodRel)
		if errProdAbs == nil {
			prodReal, errProdReal := filepath.EvalSymlinks(prodAbs)
			dbReal, errDbReal := filepath.EvalSymlinks(abs)
			if errDbReal == nil {
				if errProdReal == nil && dbReal == prodReal {
					return fmt.Errorf("recusado: symlink ou alias aponta para data/hinos.db: %w", ErrBancoRecusado)
				}
				if strings.HasSuffix(dbReal, string(filepath.Separator)+filepath.Join("data", "hinos.db")) {
					return fmt.Errorf("recusado: destino do alias aponta para data/hinos.db: %w", ErrBancoRecusado)
				}
			}
		}
	} else {
		dbReal, errDbReal := filepath.EvalSymlinks(abs)
		if errDbReal == nil && strings.HasSuffix(dbReal, string(filepath.Separator)+filepath.Join("data", "hinos.db")) {
			return fmt.Errorf("recusado: destino do alias aponta para data/hinos.db: %w", ErrBancoRecusado)
		}
	}

	return nil
}

// ValidarConsolidacaoSnapshot garante que o snapshot está consolidado e fechado:
// rejeita se o arquivo -wal existir e não estiver vazio, ou se o arquivo -shm existir.
func ValidarConsolidacaoSnapshot(dbPath string) error {
	realPath, err := filepath.EvalSymlinks(dbPath)
	if err != nil {
		return fmt.Errorf("resolver snapshot: %w", err)
	}
	dbPath = realPath
	walPath := dbPath + "-wal"
	if fi, err := os.Stat(walPath); err == nil {
		if fi.Size() > 0 {
			return fmt.Errorf("snapshot não consolidado: arquivo -wal não está vazio (%d bytes)", fi.Size())
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("verificar arquivo -wal %s: %w", walPath, err)
	}

	shmPath := dbPath + "-shm"
	if _, err := os.Stat(shmPath); err == nil {
		return fmt.Errorf("snapshot não consolidado: arquivo -shm existe (%s)", shmPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("verificar arquivo -shm %s: %w", shmPath, err)
	}

	// immutable ignora o WAL e pode reportar journal_mode=delete mesmo com
	// header WAL. Verificar os bytes também protege hardlinks/cópias sem sidecars.
	f, err := os.Open(dbPath)
	if err != nil {
		return err
	}
	defer f.Close()
	var header [20]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return fmt.Errorf("ler header SQLite: %w", err)
	}
	if string(header[:16]) != "SQLite format 3\x00" || header[18] != 1 || header[19] != 1 {
		return errors.New("snapshot não consolidado: header SQLite deve usar journal DELETE, não WAL")
	}
	return nil
}

// AbrirSnapshot abre o snapshot SQLite estritamente em modo somente-leitura e imutável
// (mode=ro&immutable=1), sem criar diretórios ou migrar o schema.
// Valida se o banco não é produção, se o snapshot está consolidado, se o journal_mode
// não é WAL e executa PRAGMA quick_check.
func AbrirSnapshot(dbPath string) (*sql.DB, error) {
	if err := RecusarBancoProd(dbPath); err != nil {
		return nil, err
	}
	if err := ValidarConsolidacaoSnapshot(dbPath); err != nil {
		return nil, err
	}

	realPath, err := filepath.EvalSymlinks(dbPath)
	if err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(realPath)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: absPath, RawQuery: "mode=ro&immutable=1"}
	dsn := u.String()
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir snapshot %q: %w", dbPath, err)
	}
	conn.SetMaxOpenConns(1)

	var jMode string
	if err := conn.QueryRow("PRAGMA journal_mode;").Scan(&jMode); err != nil {
		conn.Close()
		return nil, fmt.Errorf("verificar journal_mode: %w", err)
	}
	if strings.ToLower(jMode) == "wal" {
		conn.Close()
		return nil, fmt.Errorf("snapshot não consolidado: journal_mode é WAL")
	}

	var qCheck string
	if err := conn.QueryRow("PRAGMA quick_check;").Scan(&qCheck); err != nil {
		conn.Close()
		return nil, fmt.Errorf("executar quick_check: %w", err)
	}
	if strings.ToLower(qCheck) != "ok" {
		conn.Close()
		return nil, fmt.Errorf("quick_check falhou: %s", qCheck)
	}

	return conn, nil
}
