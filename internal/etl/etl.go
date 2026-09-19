// Package etl concentra a lógica testável da importação em lote da
// planilha de revisão: leitura do CSV, planejamento (dry-run, sem
// escrita) e gravação em lotes transacionais.
//
// A interface com o operador (TUI pterm) vive em cmd/etl e não é
// testada aqui: pterm exige terminal. Tudo que toca banco ou CSV está
// neste pacote, coberto por testes com SQLite :memory:.
package etl

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adjoli/louvores-go/internal/processors"
	"github.com/adjoli/louvores-go/internal/repository"
	"github.com/adjoli/louvores-go/internal/services"
)

// Erros sentinela do CSV (erros como valores, casa com o repo).
var (
	ErrCSVVazio         = errors.New("csv vazio")
	ErrColunaAusente    = errors.New("coluna ausente")
	ErrCabecalhoDuplic  = errors.New("cabeçalho duplicado")
	ErrLinhaInvalida    = errors.New("linha inválida")
	ErrTamanhoLote      = errors.New("tamanho de lote inválido")
	ErrBancoRecusado    = errors.New("banco recusado")
	ErrChaveDuplicada   = errors.New("chave duplicada")
	ErrDestinoAmbiguo   = errors.New("destino ambíguo no banco")
	ErrDestinoAusente   = errors.New("destino ausente no banco")
	ErrColetaneaAusente = errors.New("coletânea inexistente")
)

// Linha é uma linha da planilha de revisão.
type Linha struct {
	Arquivo  string
	Colet    string
	Numero   int
	Titulo   string
	Letra    string
	Status   string
	NumLinha int
}

// Destino é o alvo já resolvido no banco.
type Destino struct {
	HinoID   int64
	TemLetra bool
}

// Item junta linha + destino resolvido.
type Item struct {
	L Linha
	D Destino
}

// Plano é o resultado do planejamento (dry-run): nada foi escrito.
type Plano struct {
	Prontos       []Item
	Pulados       int
	PuladosPorCol map[string]int
	JaTem         int
	Erros         int
	Detalhes      []string
}

func ctx() context.Context { return context.Background() }

// LerCSV lê a planilha de revisão (cabeçalho pasta,arquivo,hinario,numero,
// titulo,letra,status,observacao — tolera os cabeçalhos com " original" e
// BOM inicial). Linhas curtas ou com número inválido viram erro
// contextualizado em vez de panic/silêncio.
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
	var out []Linha
	var elist []error
	for i, r := range recs[1:] {
		if len(r) != len(recs[0]) {
			elist = append(elist, fmt.Errorf("linha %d: %d colunas, esperado %d: %w", i+2, len(r), len(recs[0]), ErrLinhaInvalida))
			continue
		}
		num, err := strconv.Atoi(strings.TrimSpace(r[idx["numero"]]))
		if err != nil || num <= 0 {
			elist = append(elist, fmt.Errorf("linha %d: número %q inválido: %w", i+2, r[idx["numero"]], ErrLinhaInvalida))
			continue
		}
		out = append(out, Linha{
			Arquivo:  r[idx["arquivo"]],
			Colet:    strings.ToUpper(strings.TrimSpace(r[idx["hinario"]])),
			Numero:   num,
			Titulo:   r[idx["titulo"]],
			Letra:    r[idx["letra"]],
			Status:   r[idx["status"]],
			NumLinha: i + 2,
		})
	}
	if len(elist) > 0 {
		return out, errors.Join(elist...)
	}
	return out, nil
}

// Planejar resolve cada linha OK contra o banco e classifica em
// novos (sem letra) vs modificados (com letra, só com sobrescrever).
// Não escreve nada; problemas entram em Erros com detalhe por linha.
// Chaves duplicadas no CSV ou no banco barram o plano.
func Planejar(conn *sql.DB, linhas []Linha, sobrescrever bool) (*Plano, error) {
	hinos := repository.NewSQLiteHinoRepository(conn)
	colets := repository.NewSQLiteColetaneaRepository(conn)
	plano := &Plano{PuladosPorCol: map[string]int{}}
	vistas := map[string]int{}
	for _, l := range linhas {
		if !strings.EqualFold(strings.TrimSpace(l.Status), "OK") {
			plano.Pulados++
			plano.PuladosPorCol[l.Colet]++
			continue
		}
		if l.Colet == "" || l.Numero <= 0 || strings.TrimSpace(l.Letra) == "" {
			plano.Erros++
			plano.Detalhes = append(plano.Detalhes,
				fmt.Sprintf("linha %d (%s): hinario/numero/letra incompletos", l.NumLinha, l.Arquivo))
			continue
		}
		chave := l.Colet + "/" + strconv.Itoa(l.Numero)
		if visto, dup := vistas[chave]; dup {
			plano.Erros++
			plano.Detalhes = append(plano.Detalhes,
				fmt.Sprintf("linha %d (%s): chave %s duplicada no CSV (linhas %d e %d): %v", l.NumLinha, l.Arquivo, chave, visto, l.NumLinha, ErrChaveDuplicada))
			continue
		}
		vistas[chave] = l.NumLinha
		col, err := colets.FindByCodigo(ctx(), l.Colet)
		if err != nil {
			if errors.Is(err, repository.ErrColetaneaNotFound) {
				plano.Erros++
				plano.Detalhes = append(plano.Detalhes,
					fmt.Sprintf("linha %d (%s): coletânea %q inexistente: %v", l.NumLinha, l.Arquivo, l.Colet, ErrColetaneaAusente))
				continue
			}
			return nil, err
		}
		n, err := hinos.ContarPorNumero(ctx(), col.ID, l.Numero)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			plano.Erros++
			plano.Detalhes = append(plano.Detalhes,
				fmt.Sprintf("linha %d (%s): %s sem hino no banco: %v", l.NumLinha, l.Arquivo, chave, ErrDestinoAusente))
			continue
		}
		if n > 1 {
			plano.Erros++
			plano.Detalhes = append(plano.Detalhes,
				fmt.Sprintf("linha %d (%s): %s com %d hinos no banco (preflight): %v", l.NumLinha, l.Arquivo, chave, n, ErrDestinoAmbiguo))
			continue
		}
		h, err := hinos.FindByNumero(ctx(), col.ID, l.Numero)
		if err != nil {
			return nil, err
		}
		tem := h.Letra != nil && strings.TrimSpace(*h.Letra) != ""
		if tem && !sobrescrever {
			plano.Pulados++
			plano.PuladosPorCol[l.Colet]++
			plano.JaTem++
			plano.Detalhes = append(plano.Detalhes,
				fmt.Sprintf("linha %d (%s): %s já tem letra (use --force)", l.NumLinha, l.Arquivo, chave))
			continue
		}
		plano.Prontos = append(plano.Prontos, Item{
			L: l,
			D: Destino{h.ID, tem},
		})
	}
	return plano, nil
}

// Importar grava o plano em lotes transacionais de tamanhoLote (cada lote
// é uma transação: falha reverte só o lote; como o padrão pula quem já
// tem letra, re-rodar retoma de onde parou). prog recebe (feitos, total)
// para a barra de progresso; pode ser nil. A letra passa por
// services.TitularLetra — texto idêntico ao da edição web.
func Importar(conn *sql.DB, plano *Plano, tamanhoLote int, prog func(feitos, total int)) (int, error) {
	if plano == nil {
		return 0, fmt.Errorf("plano nulo")
	}
	if tamanhoLote < 1 {
		return 0, fmt.Errorf("tamanho de lote %d inválido (use >= 1): %w", tamanhoLote, ErrTamanhoLote)
	}
	total := len(plano.Prontos)
	gravados := 0
	for ini := 0; ini < total; ini += tamanhoLote {
		fim := ini + tamanhoLote
		if fim > total {
			fim = total
		}
		if err := importarLote(conn, plano.Prontos[ini:fim]); err != nil {
			return gravados, fmt.Errorf("lote %d-%d: %w", ini+1, fim, err)
		}
		gravados = fim
		if prog != nil {
			prog(gravados, total)
		}
	}
	return gravados, nil
}

func importarLote(conn *sql.DB, itens []Item) (err error) {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	for _, it := range itens {
		letra := services.TitularLetra(it.L.Letra)
		if err = repository.AtualizarLetraTx(ctx(), tx, it.D.HinoID, letra); err != nil {
			return fmt.Errorf("linha %d: %w", it.L.NumLinha, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// BackupArquivo copia o SQLite para <path>.bak-<timestamp>. Usado antes
// de qualquer escrita.
func BackupArquivo(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q é diretório", path)
	}
	dest := fmt.Sprintf("%s.bak-%s", path, time.Now().Format("20060102-150405"))
	orig, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer orig.Close()
	cp, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	defer cp.Close()
	if _, err := io.Copy(cp, orig); err != nil {
		os.Remove(dest)
		return "", err
	}
	return dest, nil
}

// RecusarBancoProd barra produção: caminho do banco oficial, URLs
// libsql/http e path vazio. O CLI só opera SQLite local de teste.
func RecusarBancoProd(dbPath string) error {
	if strings.TrimSpace(dbPath) == "" {
		return fmt.Errorf("caminho do banco vazio: %w", ErrBancoRecusado)
	}
	if strings.HasPrefix(dbPath, "libsql://") || strings.HasPrefix(dbPath, "http://") || strings.HasPrefix(dbPath, "https://") {
		return fmt.Errorf("recusado: remoto (%s) — este CLI é só SQLite local: %w", dbPath, ErrBancoRecusado)
	}
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return err
	}
	if strings.HasSuffix(abs, string(filepath.Separator)+filepath.Join("data", "hinos.db")) {
		return fmt.Errorf("recusado: este CLI nunca escreve em data/hinos.db (use uma cópia): %w", ErrBancoRecusado)
	}
	return nil
}

// Relatorio resume o plano: novos vs modificados por coletânea,
// pulados por coletânea e erros.
func Relatorio(plano *Plano) string {
	if plano == nil {
		return "plano nulo\n"
	}
	var sb strings.Builder
	porCol := map[string][2]int{}
	for _, it := range plano.Prontos {
		c := porCol[it.L.Colet]
		if it.D.TemLetra {
			c[1]++
		} else {
			c[0]++
		}
		porCol[it.L.Colet] = c
	}
	cols := make([]string, 0, len(porCol))
	for k := range porCol {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	novos, mods := 0, 0
	for _, k := range cols {
		fmt.Fprintf(&sb, "  %s: novos=%d modificados=%d\n", k, porCol[k][0], porCol[k][1])
		novos += porCol[k][0]
		mods += porCol[k][1]
	}
	fmt.Fprintf(&sb, "TOTAL prontos=%d (novos=%d modificados=%d)\n", novos+mods, novos, mods)
	type par struct {
		nome string
		n    int
	}
	var pares []par
	for k, n := range plano.PuladosPorCol {
		if k == "" {
			k = "(sem coletânea)"
		}
		pares = append(pares, par{k, n})
	}
	sort.Slice(pares, func(i, j int) bool { return pares[i].nome < pares[j].nome })
	for _, p := range pares {
		fmt.Fprintf(&sb, "  pulados %s: %d\n", p.nome, p.n)
	}
	fmt.Fprintf(&sb, "pulados=%d (já tinham letra=%d) erros=%d\n", plano.Pulados, plano.JaTem, plano.Erros)
	return sb.String()
}

// InspecionarBlocos exibe os blocos parseados de CODIGO/NUM para
// conferência (camada etl; o cmd só imprime).
func InspecionarBlocos(conn *sql.DB, alvo string) (string, error) {
	partes := strings.SplitN(alvo, "/", 2)
	if len(partes) != 2 {
		return "", fmt.Errorf("-check no formato CODIGO/NUM")
	}
	num, err := strconv.Atoi(strings.TrimSpace(partes[1]))
	if err != nil || num <= 0 {
		return "", fmt.Errorf("número %q inválido: %w", partes[1], ErrLinhaInvalida)
	}
	colets := repository.NewSQLiteColetaneaRepository(conn)
	hinos := repository.NewSQLiteHinoRepository(conn)
	col, err := colets.FindByCodigo(ctx(), strings.ToUpper(partes[0]))
	if err != nil {
		return "", err
	}
	h, err := hinos.FindByNumero(ctx(), col.ID, num)
	if err != nil {
		return "", err
	}
	letra := ""
	if h.Letra != nil {
		letra = *h.Letra
	}
	seq := processors.ProcessarHino(letra)
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s: %d blocos\n", alvo, len(seq.Partes))
	for _, p := range seq.Partes {
		primeira := strings.SplitN(p.Txt, "\n", 2)[0]
		if r := []rune(primeira); len(r) > 50 {
			primeira = string(r[:50])
		}
		fmt.Fprintf(&sb, "  [%s] %s...\n", p.Tipo, primeira)
	}
	return sb.String(), nil
}
