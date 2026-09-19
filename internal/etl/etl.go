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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/adjoli/louvores-go/internal/repository"
	"github.com/adjoli/louvores-go/internal/services"
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
	Prontos  []Item
	Pulados  int
	Erros    int
	Detalhes []string
}

func ctx() context.Context { return context.Background() }

// LerCSV lê a planilha de revisão (cabeçalho pasta,arquivo,hinario,numero,
// titulo,letra,status,observacao — tolera os cabeçalhos com " original").
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
		return nil, fmt.Errorf("csv vazio")
	}
	idx := map[string]int{}
	for i, h := range recs[0] {
		h = strings.ToLower(strings.TrimSpace(h))
		h = strings.TrimSuffix(h, " original")
		idx[h] = i
	}
	need := []string{"arquivo", "hinario", "numero", "titulo", "letra", "status"}
	for _, n := range need {
		if _, ok := idx[n]; !ok {
			return nil, fmt.Errorf("coluna %q ausente (cabeçalho: %v)", n, recs[0])
		}
	}
	var out []Linha
	for i, r := range recs[1:] {
		num, _ := strconv.Atoi(strings.TrimSpace(r[idx["numero"]]))
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
	return out, nil
}

// Planejar resolve cada linha OK contra o banco e classifica em
// novos (sem letra) vs modificados (com letra). Não escreve nada;
// linhas com problema entram em Erros com detalhe por linha.
func Planejar(conn *sql.DB, linhas []Linha) (*Plano, error) {
	hinos := repository.NewSQLiteHinoRepository(conn)
	colets := repository.NewSQLiteColetaneaRepository(conn)
	plano := &Plano{}
	for _, l := range linhas {
		if !strings.EqualFold(strings.TrimSpace(l.Status), "OK") {
			plano.Pulados++
			continue
		}
		if l.Colet == "" || l.Numero == 0 || strings.TrimSpace(l.Letra) == "" {
			plano.Erros++
			plano.Detalhes = append(plano.Detalhes,
				fmt.Sprintf("linha %d (%s): hinario/numero/letra incompletos", l.NumLinha, l.Arquivo))
			continue
		}
		col, err := colets.FindByCodigo(ctx(), l.Colet)
		if err != nil {
			plano.Erros++
			plano.Detalhes = append(plano.Detalhes,
				fmt.Sprintf("linha %d (%s): coletânea %q: %v", l.NumLinha, l.Arquivo, l.Colet, err))
			continue
		}
		h, err := hinos.FindByNumero(ctx(), col.ID, l.Numero)
		if err != nil {
			plano.Erros++
			plano.Detalhes = append(plano.Detalhes,
				fmt.Sprintf("linha %d (%s): %s/%d: %v", l.NumLinha, l.Arquivo, l.Colet, l.Numero, err))
			continue
		}
		plano.Prontos = append(plano.Prontos, Item{
			L: l,
			D: Destino{h.ID, h.Letra != nil && strings.TrimSpace(*h.Letra) != ""},
		})
	}
	return plano, nil
}

// Importar grava o plano em lotes transacionais de tamanhoLote (cada lote
// é uma transação: falha reverte só o lote). prog recebe (feitos, total)
// para a barra de progresso; pode ser nil. A letra passa por
// services.TitularLetra — texto idêntico ao da edição web.
func Importar(conn *sql.DB, plano *Plano, tamanhoLote int, prog func(feitos, total int)) (int, error) {
	if tamanhoLote < 1 {
		tamanhoLote = 100
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

func importarLote(conn *sql.DB, itens []Item) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	for _, it := range itens {
		letra := services.TitularLetra(it.L.Letra)
		res, err := tx.Exec("UPDATE hino SET letra = ?, revisado = 1 WHERE id = ?", letra, it.D.HinoID)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("linha %d: %w", it.L.NumLinha, err)
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			tx.Rollback()
			return fmt.Errorf("linha %d: RowsAffected=%d, rollback", it.L.NumLinha, n)
		}
	}
	return tx.Commit()
}

// RecusarBancoProd barra o caminho do banco de produção. O CLI só opera
// SQLite local de teste; Turso/prod nunca passa por aqui.
func RecusarBancoProd(dbPath string) error {
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return err
	}
	if strings.HasSuffix(abs, string(filepath.Separator)+"data"+string(filepath.Separator)+"hinos.db") ||
		strings.HasSuffix(abs, "data/hinos.db") {
		return fmt.Errorf("recusado: este CLI nunca escreve em data/hinos.db (use uma cópia)")
	}
	return nil
}

// Relatorio resume o plano: novos vs modificados, global e por coletânea.
func Relatorio(prontos []Item) string {
	var sb strings.Builder
	porCol := map[string][2]int{}
	for _, it := range prontos {
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
	return sb.String()
}
