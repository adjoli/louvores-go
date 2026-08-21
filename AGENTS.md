# AGENTS.md — louvores-go

Porte para Go da aplicação **Louvores** (geração de slides PPTX de hinos). O projeto Python original está em `~/Projetos/louvores` e **não deve ser alterado**.

## Stack

- **Go ≥ 1.25** (`go.mod`), `go 1.25.0`.
- **HTTP**: `net/http` stdlib (ServeMux Go 1.22+, padrões `METHOD /rota/{param}`) — sem framework.
- **API**: REST JSON, fase atual somente leitura (sem CLI; a interface HTMX fica para fase futura).
- **DB**: SQLite via `database/sql` + `modernc.org/sqlite` (100% Go, sem CGO). Testes usam `:memory:`.
- **PPTX**: `baliance/gooxml` (AGPL-3.0, sem chave de licença). `gooxml.DisableLogging()` silencia os logs internos da lib.
- **Config**: `joho/godotenv` + env vars com defaults.
- **Logging**: `log/slog` → `logs/app.log` + console (nível INFO).

## Setup & comandos

```bash
go mod tidy
go test ./...        # todos os testes (SQLite em memória + template real)
go vet ./...
go run ./cmd/louvores    # sobe o servidor HTTP (padrão :8080)
go build ./cmd/louvores
```

## Execução

O binário inicia o servidor HTTP com graceful shutdown (SIGINT/SIGTERM).

Endpoints (fase atual: somente leitura):

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/api/healthz` | liveness |
| `GET` | `/api/coletaneas` | lista as coletâneas |
| `GET` | `/api/coletaneas/{codigo}/hinos` | hinos da coletânea, ordenados pela numeração |
| `GET` | `/api/coletaneas/{codigo}/hinos/{numero}` | detalhe do hino (ex.: CC/42) |
| `GET` | `/api/stats` | estatísticas agregadas por coletânea |
| `GET` | `/api/openapi.yaml` | especificação OpenAPI (embed, YAML cru) |
| `GET` | `/api/docs` | Swagger UI (assets via CDN) |

Variáveis de ambiente (com defaults): `DB_PATH` (`data/hinos.db`), `TEMPLATE_PATH` (`data/templates/default.pptx`), `LOG_PATH` (`logs/app.log`), `HOST` (vazio = todas as interfaces), `PORT` (`8080`). `.env` opcional.

## Arquitetura

```
cmd/louvores/main.go        Entrypoint: app.New → servidor HTTP + shutdown
internal/
  config/                   Paths + env (DB, TEMPLATE, LOG, HOST, PORT) + Addr()
  logging/                  slog → arquivo + console
  database/
    db.go                   SQLite (database/sql + modernc), DDL idempotente
  models/models.go          structs Coletanea, Hino
  repository/               Repositórios por entidade (única camada que fala SQL)
    hino_repository.go      CRUD + ListByColetanea, FindByNumero, StatsPorColetanea
    coletanea_repository.go CRUD + FindByCodigo
    errors.go               ErrHinoNotFound / ErrColetaneaNotFound (wrap sql.ErrNoRows)
  services/                 hino (leitura), stats — recebem repos via DI
  api/                      Handlers HTTP finos → JSON; erros → 400/404/500
  domain/slide_parts.go     TipoParte, ParteHino, SequenciaHino
  processors/lyrics_parser.go  Letra → estrofes/refrões (por indentação)
  ppt/
    layouts.go              LayoutEstrofe=1, LayoutRefrao=2
    ppt_generator.go        gooxml: template → slides → []byte
```

Fluxo: HTTP (internal/api) → Services → Repository → SQLite. Erros como valores (sentinelas `repository.ErrHinoNotFound`, `repository.ErrColetaneaNotFound`). Escrita (revisão de letras, geração de slides via download) será adicionada em fases futuras sobre os mesmos repositórios.

## Convenções

- **Chave de negócio**: coletânea + número (ex.: CC/42); IDs internos não são expostos nas rotas de navegação.
- **Contrato JSON**: tags `snake_case` nos models/serviços; campos opcionais nulos serializados como `null` explícito (sem `omitempty`); `id`/`coletanea_id` visíveis como identificadores internos.
- **Documentação da API**: spec em `internal/api/openapi.yaml` (OpenAPI 3.0.3) e página em `internal/api/docs.html`, ambos embutidos com `go:embed`; rotas declaradas em `API.rotas()` (fonte única usada por `Routes()`); teste de paridade garante spec ↔ mux sincronizados.
- **Resposta de erro**: `{"error": "..."}`; 404 para sentinelas de não encontrado, 400 para parâmetro inválido, 500 genérico com detalhe só no log.
- **Detecção de refrão**: todas as linhas do bloco começam com espaço/tab → refrão (indentação removida); senão → estrofe.
- **Blocos**: separados por linha em branco (`\n\s*\n`).
- **Rodapé**: `N/total` no placeholder body de índice 10 do template.
- **Template `default.pptx`**: layouts `[0] TITULO` (ctrTitle + subTitle), `[1] ESTROFE`, `[2] REFRAO` (title + body idx=1 + body idx=10). **Não alterar esta estrutura** — o gerador depende dela.
- **Nomenclatura de saída** (geração futura): `{CODIGO}-{NUM:03d}-{TITULO}.pptx` (título em maiúsculas).
- **Template**: único e fixo (`default.pptx`) — não há seleção de template.

## Ambiente

`data/hinos.db`, `output/`, `logs/`, `.env`, dumps `*.sql` e `*.db-wal` estão no `.gitignore` — não commitar.
