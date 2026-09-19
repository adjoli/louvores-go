// Protótipo ETL (ticket 06) — CLI de importação de CSV revisado.
//
// Escopo propositalmente mínimo: lê a planilha de revisão (formato
// corpus/revisao.csv), mostra dry-run obrigatório (novos vs modificados vs
// pulados, global e por coletânea) e, com -dry-run=false, grava letra +
// revisado=1 em transação. Somente SQLite local; NUNCA Turso/prod.
//
// titularLetra é uma cópia temporária de services.titularLetra (não
// exportada): o protótipo precisa gravar o texto idêntico ao da edição
// web. Ao promover para o 06, unificar num único lugar.
package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/processors"
	"github.com/adjoli/louvores-go/internal/repository"
)

type linha struct {
	arquivo  string
	colet    string
	numero   int
	titulo   string
	letra    string
	status   string
	numLinha int
}

type destino struct {
	coletaneaID int64
	hinoID      int64
	temLetra    bool
}

type item struct {
	l linha
	d destino
}

func main() {
	csvPath := flag.String("csv", "", "planilha de revisão (formato corpus/revisao.csv)")
	dbPath := flag.String("db", "", "SQLite local de destino (NUNCA data/hinos.db nem Turso)")
	dryRun := flag.Bool("dry-run", true, "mostra o que seria feito sem escrever")
	check := flag.String("check", "", "após importar, exibe os blocos parseados de CODIGO/NUM (ex. CC/36)")
	flag.Parse()

	if *csvPath == "" || *dbPath == "" {
		log.Fatal("uso: etl -csv <planilha> -db <sqlite-local> [-dry-run=false] [-check CC/36]")
	}
	abs, err := filepath.Abs(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	if strings.HasSuffix(abs, "data/hinos.db") {
		log.Fatal("recusado: este CLI nunca escreve em data/hinos.db (use uma cópia)")
	}

	linhas, err := lerCSV(*csvPath)
	if err != nil {
		log.Fatal(err)
	}

	conn, err := database.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	if err := database.Migrate(conn); err != nil {
		log.Fatal(err)
	}
	hinos := repository.NewSQLiteHinoRepository(conn)
	colets := repository.NewSQLiteColetaneaRepository(conn)

	var prontos []item
	var pulados, erros int
	for _, l := range linhas {
		if !strings.EqualFold(strings.TrimSpace(l.status), "OK") {
			pulados++
			continue
		}
		if l.colet == "" || l.numero == 0 || strings.TrimSpace(l.letra) == "" {
			fmt.Printf("linha %d (%s): ERRO — hinario/numero/letra incompletos\n", l.numLinha, l.arquivo)
			erros++
			continue
		}
		d, err := resolver(conn, hinos, colets, l)
		if err != nil {
			fmt.Printf("linha %d (%s): ERRO — %v\n", l.numLinha, l.arquivo, err)
			erros++
			continue
		}
		prontos = append(prontos, item{l, d})
	}

	relatorio(prontos)
	fmt.Printf("pulados (status != OK): %d | erros: %d\n", pulados, erros)
	if erros > 0 {
		log.Fatal("importação bloqueada: corrija os erros acima")
	}
	if *dryRun {
		fmt.Println("DRY-RUN: nada escrito. Rode com -dry-run=false para gravar.")
		return
	}

	tx, err := conn.Begin()
	if err != nil {
		log.Fatal(err)
	}
	letraNova := 0
	for _, it := range prontos {
		letra := titularLetra(it.l.letra)
		res, err := tx.Exec("UPDATE hino SET letra = ?, revisado = 1 WHERE id = ?", letra, it.d.hinoID)
		if err != nil {
			tx.Rollback()
			log.Fatalf("linha %d: %v", it.l.numLinha, err)
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			tx.Rollback()
			log.Fatalf("linha %d: RowsAffected=%d, rollback", it.l.numLinha, n)
		}
		letraNova++
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("GRAVADO: %d hinos (transação única)\n", letraNova)

	if *check != "" {
		partes := strings.SplitN(*check, "/", 2)
		if len(partes) != 2 {
			log.Fatal("-check no formato CODIGO/NUM")
		}
		num, _ := strconv.Atoi(partes[1])
		col, err := colets.FindByCodigo(ctx(), strings.ToUpper(partes[0]))
		if err != nil {
			log.Fatal(err)
		}
		h, err := hinos.FindByNumero(ctx(), col.ID, num)
		if err != nil {
			log.Fatal(err)
		}
		letra := ""
		if h.Letra != nil {
			letra = *h.Letra
		}
		seq := processors.ProcessarHino(letra)
		fmt.Printf("%s: %d blocos\n", *check, len(seq.Partes))
		for _, p := range seq.Partes {
			primeira := strings.SplitN(p.Txt, "\n", 2)[0]
			fmt.Printf("  [%s] %s...\n", p.Tipo, trunc(primeira, 50))
		}
	}
}

func ctx() context.Context { return context.Background() }

func relatorio(prontos []item) {
	porCol := map[string][2]int{}
	for _, it := range prontos {
		c := porCol[it.l.colet]
		if it.d.temLetra {
			c[1]++
		} else {
			c[0]++
		}
		porCol[it.l.colet] = c
	}
	cols := make([]string, 0, len(porCol))
	for k := range porCol {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	novos, mods := 0, 0
	for _, k := range cols {
		fmt.Printf("  %s: novos=%d modificados=%d\n", k, porCol[k][0], porCol[k][1])
		novos += porCol[k][0]
		mods += porCol[k][1]
	}
	fmt.Printf("TOTAL prontos=%d (novos=%d modificados=%d)\n", novos+mods, novos, mods)
}

func resolver(conn *sql.DB, hinos *repository.SQLiteHinoRepository, colets *repository.SQLiteColetaneaRepository, l linha) (destino, error) {
	col, err := colets.FindByCodigo(ctx(), l.colet)
	if err != nil {
		return destino{}, fmt.Errorf("coletânea %q: %w", l.colet, err)
	}
	h, err := hinos.FindByNumero(ctx(), col.ID, l.numero)
	if err != nil {
		return destino{}, fmt.Errorf("%s/%d: %w", l.colet, l.numero, err)
	}
	return destino{col.ID, h.ID, h.Letra != nil && strings.TrimSpace(*h.Letra) != ""}, nil
}

func lerCSV(path string) ([]linha, error) {
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
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	need := []string{"arquivo", "hinario", "numero", "titulo", "letra", "status"}
	for _, n := range need {
		if _, ok := idx[n]; !ok {
			return nil, fmt.Errorf("coluna %q ausente (cabeçalho: %v)", n, recs[0])
		}
	}
	var out []linha
	for i, r := range recs[1:] {
		num, _ := strconv.Atoi(strings.TrimSpace(r[idx["numero"]]))
		out = append(out, linha{
			arquivo:  r[idx["arquivo"]],
			colet:    strings.ToUpper(strings.TrimSpace(r[idx["hinario"]])),
			numero:   num,
			titulo:   r[idx["titulo"]],
			letra:    r[idx["letra"]],
			status:   r[idx["status"]],
			numLinha: i + 2,
		})
	}
	return out, nil
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// titularLetra — CÓPIA de services.titularLetra para o protótipo.
// (Não exportada no serviço; unificar ao promover.)
func titularLetra(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if trimmed == strings.ToLower(trimmed) || trimmed == strings.ToUpper(trimmed) {
			lines[i] = titularLinha(line)
		}
	}
	return strings.Join(lines, "\n")
}

func titularLinha(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]

	words := strings.Fields(trimmed)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return indent + strings.Join(words, " ")
}
