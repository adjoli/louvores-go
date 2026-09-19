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
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/pterm/pterm"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/etl"
)

func main() {
	csvPath := flag.String("csv", "", "planilha de revisão")
	dbPath := flag.String("db", "", "SQLite local de destino")
	dryRun := flag.Bool("dry-run", true, "mostra o overview sem escrever")
	yes := flag.Bool("yes", false, "confirma a gravação no modo não-interativo")
	force := flag.Bool("force", false, "sobrescreve letras já existentes")
	initDB := flag.Bool("init", false, "permite criar o SQLite se o arquivo não existir")
	lote := flag.Int("batch", 100, "tamanho do lote transacional")
	check := flag.String("check", "", "exibe blocos parseados de CODIGO/NUM (ex. CC/36)")
	flag.Parse()

	csv, db := *csvPath, *dbPath
	interativo := csv == ""
	importarEscolhido := false
	if interativo {
		var err error
		var acao string
		acao, csv, db, err = menu()
		if err != nil {
			log.Fatal(err)
		}
		importarEscolhido = strings.HasPrefix(acao, "Importar")
	}
	if err := etl.RecusarBancoProd(db); err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(db); err != nil {
		if os.IsNotExist(err) && *initDB {
			fmt.Println("banco inexistente — será criado (-init)")
		} else {
			log.Fatalf("banco %q inacessível (use -init para criar): %v", db, err)
		}
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

	plano, err := etl.Planejar(conn, linhas, *force)
	if err != nil {
		log.Fatal(err)
	}
	for _, d := range plano.Detalhes {
		fmt.Fprintln(os.Stderr, d)
	}
	fmt.Print(etl.Relatorio(plano))
	if plano.Erros > 0 {
		log.Fatal("importação bloqueada: corrija os erros acima")
	}

	// -check funciona também em dry-run (conferir antes de gravar).
	mostrarCheck := *check != ""
	if mostrarCheck {
		out, err := etl.InspecionarBlocos(conn, *check)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(out)
	}

	seco := *dryRun || (interativo && !importarEscolhido)
	if seco {
		fmt.Println("DRY-RUN: nada escrito.")
		return
	}
	if interativo {
		ok, err := pterm.DefaultInteractiveConfirm.
			WithDefaultValue(false).
			Show(fmt.Sprintf("Gravar %d hinos? (backup será criado antes)", len(plano.Prontos)))
		if err != nil || !ok {
			fmt.Println("cancelado.")
			return
		}
	} else if !*yes {
		log.Fatal("modo não-interativo exige -yes para gravar")
	}

	backup, err := etl.BackupArquivo(db)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("backup em", backup)

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
		msg := fmt.Sprintf("%v (backup em %s)", err, backup)
		if !*force {
			msg += " — re-rodar retoma (gravados viram pulados)"
		}
		log.Fatal(msg)
	}
	fmt.Printf("GRAVADO: %d hinos em lotes de %d (backup em %s)\n", n, *lote, backup)

	if *check != "" {
		out, err := etl.InspecionarBlocos(conn, *check)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(out)
	}
}

func menu() (string, string, string, error) {
	acao, err := pterm.DefaultInteractiveSelect.
		WithOptions([]string{"Importar CSV (dry-run + gravar)", "Validar CSV (só dry-run)"}).
		Show()
	if err != nil {
		return "", "", "", err
	}
	csv, err := pterm.DefaultInteractiveTextInput.
		WithDefaultText("caminho do CSV").
		Show()
	if err != nil {
		return "", "", "", err
	}
	db, err := pterm.DefaultInteractiveTextInput.
		WithDefaultText("caminho do SQLite local").
		Show()
	if err != nil {
		return "", "", "", err
	}
	return acao, strings.TrimSpace(csv), strings.TrimSpace(db), nil
}
