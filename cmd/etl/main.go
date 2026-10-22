// Command etl — CLI offline do operador para normalização, preparação de manifesto,
// inspeção, validação isolada de slides e exportação atômica de SQL.
//
// Operação estritamente offline:
//   - Lê snapshots SQLite locais consolidados em modo somente-leitura e imutável.
//   - NUNCA executa migrações (Migrate) no snapshot de entrada.
//   - Recusa bancos de produção (data/hinos.db e aliases/symlinks/hardlinks), DSNs remotos e file:.
//   - Exporta artefatos SQL de forma atômica para diretório ainda inexistente.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/adjoli/louvores-go/internal/etl"
	"github.com/adjoli/louvores-go/internal/services"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}
}

// run executa o comando CLI com os argumentos e streams fornecidos.
func run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}

	fs := flag.NewFlagSet("etl", flag.ContinueOnError)
	fs.SetOutput(errOut)

	normalizarStdin := fs.Bool("normalizar-stdin", false, "normaliza letra recebida em stdin via services.TitularLetra e emite em stdout sem tocar em banco")
	manifestOut := fs.String("manifest-out", "", "caminho para salvar o manifesto congelado preparado a partir do CSV")
	manifestPath := fs.String("manifest", "", "caminho do manifesto congelado a consumir (-check, -validar, -sql-out)")
	csvPath := fs.String("csv", "", "caminho da planilha CSV sanitizada")
	dbPath := fs.String("db", "", "caminho do snapshot SQLite local consolidado (somente-leitura)")
	batchID := fs.String("batch-id", "", "identificador único do lote (obrigatório com -manifest-out)")
	provPath := fs.String("provenance", "", "caminho do arquivo provenance.jsonl com recibos de linhagem")
	templatePath := fs.String("template", "data/templates/default.pptx", "caminho do template PPTX")
	checkArg := fs.String("check", "", "inspeciona blocos e estrofes/refrões do manifesto ('all' ou 'CODIGO/NUM')")
	validarFlag := fs.Bool("validar", false, "valida integridade OOXML dos slides em cópia descartável e isolada do banco")
	sqlOut := fs.String("sql-out", "", "caminho do import.sql para exportação atômica dos scripts (<dir>/import.sql)")

	// Sem flags imprime ajuda e sai com código 0
	if len(args) == 0 {
		fs.SetOutput(out)
		fs.Usage()
		return nil
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	// Modo 1: -normalizar-stdin
	if *normalizarStdin {
		if in == nil {
			return errors.New("stdin nulo para -normalizar-stdin")
		}
		data, err := io.ReadAll(in)
		if err != nil {
			return fmt.Errorf("ler stdin: %w", err)
		}
		norm := services.TitularLetra(string(data))
		if _, err := fmt.Fprint(out, norm); err != nil {
			return fmt.Errorf("escrever stdout: %w", err)
		}
		return nil
	}

	// Validação de exclusividade mútua entre preparação e consumo de manifesto
	if *manifestOut != "" && *manifestPath != "" {
		return errors.New("não é permitido combinar -manifest-out e -manifest na mesma chamada")
	}
	if *manifestOut == "" && *manifestPath == "" {
		return errors.New("especifique -manifest-out para preparar manifesto, -manifest para consumir, ou -normalizar-stdin")
	}

	// Modo 2: Preparação de manifesto (-manifest-out)
	if *manifestOut != "" {
		if *csvPath == "" || *dbPath == "" || *batchID == "" || *provPath == "" {
			return errors.New("modo de preparação (-manifest-out) exige -csv, -db, -batch-id e -provenance")
		}
		if *checkArg != "" || *validarFlag || *sqlOut != "" {
			return errors.New("ações -check, -validar e -sql-out pertencem ao modo -manifest")
		}

		// Recusa sobrescrever arquivo de manifesto existente
		if _, err := os.Lstat(*manifestOut); err == nil {
			return fmt.Errorf("arquivo de manifesto %q já existe: recusa sobrescrever", *manifestOut)
		}

		// Hashes iniciais dos artefatos de entrada
		snapSHAInit, err := hashArquivo(*dbPath)
		if err != nil {
			return fmt.Errorf("hash do snapshot %s: %w", *dbPath, err)
		}
		csvSHA, err := hashArquivo(*csvPath)
		if err != nil {
			return fmt.Errorf("hash do csv %s: %w", *csvPath, err)
		}
		provSHA, err := hashArquivo(*provPath)
		if err != nil {
			return fmt.Errorf("hash do provenance %s: %w", *provPath, err)
		}
		tplSHA, err := hashArquivo(*templatePath)
		if err != nil {
			return fmt.Errorf("hash do template %s: %w", *templatePath, err)
		}

		// Abre snapshot estritamente read-only sem Migrate
		conn, err := etl.AbrirSnapshot(*dbPath)
		if err != nil {
			return fmt.Errorf("abrir snapshot: %w", err)
		}
		defer conn.Close()

		linhas, err := etl.LerCSV(*csvPath)
		if err != nil {
			return fmt.Errorf("ler csv: %w", err)
		}

		if err := etl.ValidarRecibos(linhas, *provPath); err != nil {
			return fmt.Errorf("validar recibos de proveniência: %w", err)
		}

		meta := etl.MetaManifesto{
			Versao:           1,
			BatchID:          *batchID,
			SnapshotSHA256:   snapSHAInit,
			CSVSHA256:        csvSHA,
			ProvenanceSHA256: provSHA,
			TemplateSHA256:   tplSHA,
		}

		m, err := etl.PrepararManifesto(ctx, conn, linhas, meta)
		if err != nil {
			return fmt.Errorf("preparar manifesto: %w", err)
		}

		// Confere se snapshot permaneceu intacto após as leituras
		snapSHAFinal, err := hashArquivo(*dbPath)
		if err != nil {
			return fmt.Errorf("hash final do snapshot: %w", err)
		}
		if snapSHAFinal != snapSHAInit {
			return errors.New("snapshot foi alterado durante a preparação do manifesto")
		}

		if len(m.Itens) == 0 {
			fmt.Fprintf(out, "Nenhum hino elegível encontrado para o manifesto (batch %s).\n", *batchID)
			return nil
		}

		if err := etl.SalvarManifesto(*manifestOut, m); err != nil {
			return fmt.Errorf("salvar manifesto: %w", err)
		}

		fmt.Fprintf(out, "Manifesto preparado com %d hinos em %s\n", len(m.Itens), *manifestOut)
		return nil
	}

	// Modo 3: Consumo de manifesto (-manifest)
	if *manifestPath != "" {
		if *csvPath == "" || *provPath == "" || *dbPath == "" {
			return errors.New("modo de consumo (-manifest) exige -csv, -provenance e -db")
		}
		if *checkArg == "" && !*validarFlag && *sqlOut == "" {
			return errors.New("modo -manifest exige ao menos uma ação: -check, -validar ou -sql-out")
		}

		m, manifestSHA, err := etl.LerManifesto(*manifestPath)
		if err != nil {
			return fmt.Errorf("ler manifesto: %w", err)
		}

		// Revalida hashes dos artefatos físicos no disco contra os metadados do manifesto
		snapSHAInit, err := hashArquivo(*dbPath)
		if err != nil {
			return fmt.Errorf("hash do snapshot %s: %w", *dbPath, err)
		}
		if snapSHAInit != m.Meta.SnapshotSHA256 {
			return fmt.Errorf("snapshot hash diverge do manifesto: obtido %s, esperado %s", snapSHAInit, m.Meta.SnapshotSHA256)
		}

		csvSHA, err := hashArquivo(*csvPath)
		if err != nil {
			return fmt.Errorf("hash do csv %s: %w", *csvPath, err)
		}
		if csvSHA != m.Meta.CSVSHA256 {
			return fmt.Errorf("csv hash diverge do manifesto: obtido %s, esperado %s", csvSHA, m.Meta.CSVSHA256)
		}

		provSHA, err := hashArquivo(*provPath)
		if err != nil {
			return fmt.Errorf("hash do provenance %s: %w", *provPath, err)
		}
		if provSHA != m.Meta.ProvenanceSHA256 {
			return fmt.Errorf("provenance hash diverge do manifesto: obtido %s, esperado %s", provSHA, m.Meta.ProvenanceSHA256)
		}

		tplSHA, err := hashArquivo(*templatePath)
		if err != nil {
			return fmt.Errorf("hash do template %s: %w", *templatePath, err)
		}
		if tplSHA != m.Meta.TemplateSHA256 {
			return fmt.Errorf("template hash diverge do manifesto: obtido %s, esperado %s", tplSHA, m.Meta.TemplateSHA256)
		}

		// Abre snapshot estritamente read-only para verificar guardas
		conn, err := etl.AbrirSnapshot(*dbPath)
		if err != nil {
			return fmt.Errorf("abrir snapshot: %w", err)
		}
		defer conn.Close()

		linhas, err := etl.LerCSV(*csvPath)
		if err != nil {
			return fmt.Errorf("ler csv: %w", err)
		}
		if err := etl.ValidarRecibos(linhas, *provPath); err != nil {
			return fmt.Errorf("validar recibos de proveniência: %w", err)
		}
		esperado, err := etl.PrepararManifesto(ctx, conn, linhas, m.Meta)
		if err != nil {
			return fmt.Errorf("revalidar manifesto: %w", err)
		}
		if !reflect.DeepEqual(m, esperado) {
			return errors.New("itens do manifesto divergem do snapshot, CSV ou recibos de proveniência")
		}

		// Ação: -check
		if *checkArg != "" {
			outStr, err := etl.InspecionarManifesto(m, *checkArg)
			if err != nil {
				return fmt.Errorf("check: %w", err)
			}
			if _, err := fmt.Fprint(out, outStr); err != nil {
				return err
			}
		}

		// Ação: -validar
		if *validarFlag {
			resultados, err := etl.ValidarManifesto(ctx, *dbPath, *templatePath, m, manifestSHA)
			if err != nil {
				return fmt.Errorf("validar manifesto: %w", err)
			}
			falhas := 0
			for _, v := range resultados {
				if v.Erro != nil {
					fmt.Fprintf(out, "  FALHA %s: %v\n", v.Chave, v.Erro)
					falhas++
				} else {
					fmt.Fprintf(out, "  OK %s: %d slides\n", v.Chave, v.Slides)
				}
			}
			if falhas > 0 {
				return fmt.Errorf("validação: %d hino(s) com pacote inválido", falhas)
			}
			fmt.Fprintln(out, "validação: todos os pacotes íntegros")
		}

		// Ação: -sql-out
		if *sqlOut != "" {
			if len(m.Itens) == 0 {
				fmt.Fprintln(out, "Manifesto sem hinos elegíveis: nenhum SQL exportado.")
			} else {
				targetDir := filepath.Dir(*sqlOut)
				parentDir := filepath.Dir(targetDir)
				if parentDir == "" {
					parentDir = "."
				}

				if filepath.Base(*sqlOut) != "import.sql" {
					return fmt.Errorf("-sql-out deve apontar para import.sql em diretório de destino (ex: <dir>/import.sql), recebido: %s", *sqlOut)
				}

				pStat, err := os.Stat(parentDir)
				if err != nil || !pStat.IsDir() {
					return fmt.Errorf("diretório pai %q não existe ou não é diretório: %w", parentDir, err)
				}

				// Pré-checa destino antes de gerar SQL
				if _, err := os.Lstat(targetDir); err == nil {
					return fmt.Errorf("diretório destino %q já existe: recusa sobrescrever", targetDir)
				} else if !os.IsNotExist(err) {
					return fmt.Errorf("verificar diretório destino %q: %w", targetDir, err)
				}

				artefatos, err := etl.GerarSQL(m, manifestSHA)
				if err != nil {
					return fmt.Errorf("gerar SQL: %w", err)
				}

				// Gera os 3 arquivos em diretório temporário no mesmo diretório pai
				stageDir, err := os.MkdirTemp(parentDir, ".sql_stage_*")
				if err != nil {
					return fmt.Errorf("criar diretório temporário de staging: %w", err)
				}
				published := false
				defer func() {
					if !published {
						_ = os.RemoveAll(stageDir)
					}
				}()

				if err := gravarArquivoComFsync(filepath.Join(stageDir, "import.sql"), artefatos.Import); err != nil {
					return fmt.Errorf("gravar import.sql: %w", err)
				}
				if err := gravarArquivoComFsync(filepath.Join(stageDir, "rollback.sql"), artefatos.Rollback); err != nil {
					return fmt.Errorf("gravar rollback.sql: %w", err)
				}
				if err := gravarArquivoComFsync(filepath.Join(stageDir, "verify.sql"), artefatos.Verify); err != nil {
					return fmt.Errorf("gravar verify.sql: %w", err)
				}

				// Publica atomicamente o diretório inteiro
				if err := publicarSQL(stageDir, targetDir); err != nil {
					return fmt.Errorf("publicar SQL: %w", err)
				}
				published = true
				fmt.Fprintf(out, "SQL exportado com sucesso em %s\n", targetDir)
			}
		}

		// Confere se snapshot permaneceu intacto após todas as operações
		snapSHAFinal, err := hashArquivo(*dbPath)
		if err != nil {
			return fmt.Errorf("hash final do snapshot: %w", err)
		}
		if snapSHAFinal != snapSHAInit {
			return errors.New("snapshot foi alterado durante o consumo do manifesto")
		}

		return nil
	}

	return nil
}

// publicarSQL publica de forma atômica o diretório de staging para targetDir.
// Adquire lock de publicação O_CREATE|O_EXCL no pai antes de checar destino,
// recusa qualquer target existente (inclusive symlink ou vazio), faz rename só sob
// lock e remove apenas o lock e o stage de sua propriedade ao falhar.
func publicarSQL(stageDir, targetDir string) error {
	parentDir := filepath.Dir(targetDir)
	if parentDir == "" {
		parentDir = "."
	}
	pStat, err := os.Stat(parentDir)
	if err != nil || !pStat.IsDir() {
		return fmt.Errorf("diretório pai %q não existe ou não é diretório: %w", parentDir, err)
	}

	lockPath := filepath.Join(parentDir, ".sql_export.lock")

	var lockFile *os.File
	start := time.Now()
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			lockFile = f
			break
		}
		if !os.IsExist(err) {
			return fmt.Errorf("criar lock de publicação %s: %w", lockPath, err)
		}
		if time.Since(start) > 5*time.Second {
			return fmt.Errorf("timeout ao aguardar lock de publicação %s", lockPath)
		}
		time.Sleep(10 * time.Millisecond)
	}

	defer func() {
		if lockFile != nil {
			_ = lockFile.Close()
			_ = os.Remove(lockPath)
		}
	}()

	// Sob lock: recusa qualquer target existente (inclusive symlink/vazio)
	if _, err := os.Lstat(targetDir); err == nil {
		return fmt.Errorf("diretório destino %q já existe: recusa sobrescrever", targetDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checar diretório destino %q: %w", targetDir, err)
	}

	// Rename sob lock
	if err := os.Rename(stageDir, targetDir); err != nil {
		return fmt.Errorf("renomear %s para %s: %w", stageDir, targetDir, err)
	}

	return nil
}

func hashArquivo(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func gravarArquivoComFsync(path, content string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.WriteString(f, content); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return f.Close()
}
