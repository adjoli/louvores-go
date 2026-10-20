# AGENTS.md — cmd/etl

CLI offline de autoimport assistido e validação de slides.

## Princípios

- **Somente-leitura sobre o banco**: O CLI nunca escreve em `data/hinos.db` nem acessa bancos remotos. Trabalha estritamente sobre snapshots locais consolidados (`mode=ro&immutable=1`).
- **Arquitetura de manifesto**: Todas as entradas são congeladas em um manifesto imutável com hashes SHA-256.
- **Saída em SQL puro**: Produz scripts atômicos (`import.sql`, `rollback.sql`, `verify.sql`) para aplicação manual pelo mantenedor via `sqlite3` ou `turso db shell`.

## Comandos

### 1. Normalizar letra via stdin
```bash
printf 'Ó VEM' | go run ./cmd/etl -normalizar-stdin
```

### 2. Gerar manifesto a partir de planilha sanitizada
```bash
go run ./cmd/etl \
  -csv <sanitized.csv> \
  -provenance <provenance.jsonl> \
  -db <snapshot.db> \
  -batch-id <id> \
  -manifest-out <manifesto.json>
```

### 3. Inspecionar, validar e exportar SQL
As ações podem ser combinadas na mesma execução:
```bash
go run ./cmd/etl \
  -manifest <manifesto.json> \
  -csv <sanitized.csv> \
  -provenance <provenance.jsonl> \
  -db <snapshot.db> \
  -check all \
  -validar \
  -sql-out ./export/import.sql
```
- `-check <CODIGO/NUM|all>`: exibe contagem de blocos e primeiras linhas formatadas.
- `-validar`: valida a geração de PPTX em clone descartável (`VACUUM INTO`) com verificação estrutural OOXML.
- `-sql-out <dir>/import.sql`: gera atomicamente `import.sql`, `rollback.sql` e `verify.sql` no diretório informado (que não deve existir previamente).

## Segurança e Direitos Autorais

- **Conformidade de Propriedade Intelectual**: O repositório é público no GitHub. É estritamente proibido incluir letras reais de hinos ou dados protegidos por direitos autorais em testes, fixtures ou código em `cmd/etl/`. Todos os testes devem utilizar exclusivamente fixtures sintéticas.
- **Isolamento de Saídas SQL e Manifestos**: Planilhas, manifestos e scripts SQL gerados (`import.sql`, `rollback.sql`, `verify.sql`) contêm letras completas e devem ser gerados EXCLUSIVAMENTE em diretórios de runtime temporários fora do git (ex.: `$RUN/sql/` ou `/var/folders/...`). Nunca aponte `-sql-out` para diretórios versionados do repositório.
- Rejeita URLs remotas (`libsql://`, `http://`, etc.), schemes `file:` e referências a `data/hinos.db` (incluindo symlinks/hardlinks).
- Exige snapshot consolidado (sem arquivos `-wal` ou `-shm` ativos).
- Aplicação em produção é feita exclusivamente de forma manual por um mantenedor humano após conferência de amostra.
