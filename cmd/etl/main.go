// Command etl — TUI do operador para importar a planilha de revisão.
//
// Camada fina sobre internal/etl (toda lógica testável está lá): menu de
// seleção, input de paths, dry-run obrigatório com overview, confirmação
// e barra de progresso por lote. Somente SQLite local; NUNCA Turso/prod
// (trava em internal/etl.RecusarBancoProd).
//
// Sem flags (-csv) abre o menu interativo. Com flags roda direto — útil
// para testes e scripts (nesse modo a gravação exige -yes explícito).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/pterm/pterm"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/etl"
	"github.com/adjoli/louvores-go/internal/processors"
	"github.com/adjoli/louvores-go/internal/repository"
)

func main() {
	csvPath := flag.String("csv", "", "planilha de revisão")
	dbPath := flag.String("db", "", "SQLite local de destino")
	dryRun := flag.Bool("dry-run", true, "mostra o overview sem escrever")
	yes := flag.Bool("yes", false, "confirma a gravação no modo não-interativo")
	lote := flag.Int("batch", 100, "tamanho do lote transacional")
	check := flag.String("check", "", "exibe blocos parseados de CODIGO/NUM (ex. CC/36)")
	flag.Parse()

	csv, db := *csvPath, *dbPath
	interativo := csv == ""
	if interativo {
		var err error
		csv, db, err = menu()
		if err != nil {
			log.Fatal(err)
		}
	}
	if err := etl.RecusarBancoProd(db); err != nil {
		log.Fatal(err)
	}

	linhas, err := etl.LerCSV(csv)
	if err != nil {
		log.Fatal(err)
	}
	conn, err := database.Open(db)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	if err := database.Migrate(conn); err != nil {
		log.Fatal(err)
	}

	plano, err := etl.Planejar(conn, linhas)
	if err != nil {
		log.Fatal(err)
	}
	for _, d := range plano.Detalhes {
		fmt.Fprintln(os.Stderr, d)
	}
	fmt.Print(etl.Relatorio(plano.Prontos))
	fmt.Printf("pulados (status != OK): %d | erros: %d\n", plano.Pulados, plano.Erros)
	if plano.Erros > 0 {
		log.Fatal("importação bloqueada: corrija os erros acima")
	}

	if *dryRun {
		fmt.Println("DRY-RUN: nada escrito.")
		return
	}
	if interativo {
		ok, err := pterm.DefaultInteractiveConfirm.
			WithDefaultValue(false).
			Show("Gravar " + strconv.Itoa(len(plano.Prontos)) + " hinos?")
		if err != nil || !ok {
			fmt.Println("cancelado.")
			return
		}
	} else if !*yes {
		log.Fatal("modo não-interativo exige -yes para gravar")
	}

	var prog func(feitos, total int)
	if interativo {
		bar, err := pterm.DefaultProgressbar.
			WithTotal(len(plano.Prontos)).
			WithTitle("Importando").
			Start()
		if err != nil {
			log.Fatal(err)
		}
		defer bar.Stop()
		ultimo := 0
		prog = func(feitos, total int) {
			bar.Add(feitos - ultimo)
			ultimo = feitos
		}
	} else {
		prog = func(feitos, total int) {
			fmt.Printf("  lote: %d/%d\n", feitos, total)
		}
	}
	n, err := etl.Importar(conn, plano, *lote, prog)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("GRAVADO: %d hinos em lotes de %d\n", n, *lote)

	if *check != "" {
		mostrarBlocos(conn, *check)
	}
}

func menu() (string, string, error) {
	acao, err := pterm.DefaultInteractiveSelect.
		WithOptions([]string{"Importar CSV (dry-run + gravar)", "Validar CSV (só dry-run)"}).
		Show()
	if err != nil {
		return "", "", err
	 }
	_ = acao
	csv, err := pterm.DefaultInteractiveTextInput.
		WithDefaultText("caminho do CSV").
		Show()
	if err != nil {
		return "", "", err
	}
	db, err := pterm.DefaultInteractiveTextInput.
		WithDefaultText("caminho do SQLite local").
		Show()
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(csv), strings.TrimSpace(db), nil
}

func mostrarBlocos(conn *sql.DB, alvo string) {
	partes := strings.SplitN(alvo, "/", 2)
	if len(partes) != 2 {
		log.Fatal("-check no formato CODIGO/NUM")
	}
	num, err := strconv.Atoi(partes[1])
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	colets := repository.NewSQLiteColetaneaRepository(conn)
	hinos := repository.NewSQLiteHinoRepository(conn)
	col, err := colets.FindByCodigo(ctx, strings.ToUpper(partes[0]))
	if err != nil {
		log.Fatal(err)
	}
	h, err := hinos.FindByNumero(ctx, col.ID, num)
	if err != nil {
		log.Fatal(err)
	}
	letra := ""
	if h.Letra != nil {
		letra = *h.Letra
	}
	seq := processors.ProcessarHino(letra)
	fmt.Printf("%s: %d blocos\n", alvo, len(seq.Partes))
	for _, p := range seq.Partes {
		primeira := strings.SplitN(p.Txt, "\n", 2)[0]
		if len(primeira) > 50 {
			primeira = primeira[:50]
		}
		fmt.Printf("  [%s] %s...\n", p.Tipo, primeira)
	}
}
