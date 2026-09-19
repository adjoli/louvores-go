// Command etl — CLI do operador para importar a planilha de revisão.
//
// Camada fina sobre internal/etl (toda lógica testável está lá): menu em
// texto puro (stdlib, sem dependência de TUI), input de paths, dry-run
// obrigatório com overview, confirmação e progresso por lote. Somente
// SQLite local; NUNCA Turso/prod (trava em internal/etl.RecusarBancoProd).
//
// Sem flags (-csv) abre o menu interativo. Com flags roda direto — útil
// para testes e scripts (nesse modo a gravação exige -yes explícito).
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

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
	validar := flag.Bool("validar", false, "após gravar, gera o PPTX de cada hino e confere o pacote")
	templatePath := flag.String("template", "data/templates/default.pptx", "template PPTX da validação")
	flag.Parse()

	ctx := context.Background()
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
		importarEscolhido = acao == "importar"
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

	plano, err := etl.Planejar(ctx, conn, linhas, *force)
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
	if *check != "" {
		out, err := etl.InspecionarBlocos(ctx, conn, *check)
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
		if !confirma(fmt.Sprintf("Gravar %d hinos? (backup será criado antes) [s/N] ", len(plano.Prontos))) {
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

	if len(plano.Prontos) == 0 {
		fmt.Println("nada a gravar.")
		return
	}

	n, err := etl.Importar(ctx, conn, plano, *lote, func(feitos, total int) {
		fmt.Printf("  lote: %d/%d\n", feitos, total)
	})
	if err != nil {
		msg := fmt.Sprintf("%v (backup em %s)", err, backup)
		if !*force {
			msg += " — re-rodar retoma (gravados viram pulados)"
		}
		log.Fatal(msg)
	}
	fmt.Printf("GRAVADO: %d hinos em lotes de %d (backup em %s)\n", n, *lote, backup)

	if *validar {
		falhas := 0
		for _, v := range etl.ValidarSlides(ctx, conn, *templatePath, plano.Prontos) {
			if v.Erro != nil {
				fmt.Printf("  FALHA %s: %v\n", v.Chave, v.Erro)
				falhas++
			} else {
				fmt.Printf("  OK %s: %d slides\n", v.Chave, v.Slides)
			}
		}
		if falhas > 0 {
			log.Fatalf("validação: %d hino(s) com pacote inválido", falhas)
		}
		fmt.Println("validação: todos os pacotes íntegros")
	}
}

// menu exibe as ações numeradas e lê a escolha + paths do stdin.
func menu() (acao, csv, db string, err error) {
	in := bufio.NewReader(os.Stdin)
	fmt.Println("1) Importar CSV (dry-run + gravar)")
	fmt.Println("2) Validar CSV (só dry-run)")
	fmt.Print("escolha [1-2]: ")
	op, err := in.ReadString('\n')
	if err != nil {
		return "", "", "", err
	}
	if strings.TrimSpace(op) == "1" {
		acao = "importar"
	} else {
		acao = "validar"
	}
	fmt.Print("caminho do CSV: ")
	csv, err = in.ReadString('\n')
	if err != nil {
		return "", "", "", err
	}
	fmt.Print("caminho do SQLite local: ")
	db, err = in.ReadString('\n')
	if err != nil {
		return "", "", "", err
	}
	return acao, strings.TrimSpace(csv), strings.TrimSpace(db), nil
}

// confirma lê s/N do stdin (default N).
func confirma(prompt string) bool {
	fmt.Print(prompt)
	in := bufio.NewReader(os.Stdin)
	resp, err := in.ReadString('\n')
	if err != nil {
		return false
	}
	resp = strings.ToLower(strings.TrimSpace(resp))
	return resp == "s" || resp == "sim" || resp == "y" || resp == "yes"
}
