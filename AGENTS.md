# AGENTS.md — louvores-go

Porte para Go da aplicação **Louvores** (geração de slides PPTX de hinos). O projeto Python original está em `~/Projetos/louvores` e **não deve ser alterado**.

## Stack

- **Go ≥ 1.25** (`go.mod`), `go 1.25.0`.
- **HTTP**: `net/http` stdlib (ServeMux Go 1.22+, padrões `METHOD /rota/{param}`) — sem framework.
- **API**: REST JSON, fase atual somente leitura.
- **Interface web**: `templ` (templates tipados) + **HTMX** (parcial/atualização assíncrona) + **Tailwind CSS**. As views ficam em `internal/web/`. Os arquivos `_templ.go` gerados e o `main.css` gerado são commitados.
- **DB**: SQLite via `database/sql` + `modernc.org/sqlite` (100% Go, sem CGO). Testes usam `:memory:`.
- **PPTX**: geração própria sobre o pacote OOXML (`archive/zip` + `encoding/xml`), preservando o template byte-a-byte e registrando os slides novos de forma consistente (`[Content_Types].xml`, `.rels`, `sldIdLst`). `github.com/baliance/gooxml` (AGPL-3.0) é usado **somente como validador nos testes** (`presentation.Open`); o código de produção não o importa.
- **Config**: `joho/godotenv` + env vars com defaults.
- **Logging**: `log/slog` → `logs/app.log` + console (nível INFO).

## Setup & comandos

```bash
go mod tidy
go test ./...        # todos os testes (SQLite em memória + template real)
go vet ./...
go run ./cmd/louvores    # sobe o servidor HTTP (padrão :8080)
go build ./cmd/louvores
make build           # compila com versão (VERSION, git commit e data via -ldflags)
./louvores -version  # imprime a versão do binário
```

A versão é definida por `make build VERSION=x.y.z` (default `dev`); o commit e a data vêm do git. Sem build flags, `version.String()` retorna `dev`.

**Geração da interface web** (é preciso rodar antes de alterar `.templ`/CSS e commitá-los):

```bash
templ generate ./...              # gera _templ.go a partir de *.templ
./scripts/build-css.sh            # gera internal/web/static/main.css (Tailwind, via node do Windows/WSL)
```

## Execução

O binário inicia o servidor HTTP com graceful shutdown (SIGINT/SIGTERM).

Endpoints (API somente leitura; geração de slides é download):

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/api/healthz` | liveness |
| `GET` | `/api/coletaneas` | lista as coletâneas |
| `GET` | `/api/coletaneas/{codigo}/hinos` | hinos da coletânea, ordenados pela numeração |
| `GET` | `/api/coletaneas/{codigo}/hinos/{numero}` | detalhe do hino (ex.: CC/42) |
| `GET` | `/api/coletaneas/{codigo}/hinos/{numero}/slides` | gera e baixa o PPTX dos slides do hino |
| `POST` | `/api/coletaneas/{codigo}/slides/lote` | gera um ZIP com o PPTX de cada hino revisado da coletânea |
| `GET` | `/api/stats` | estatísticas agregadas por coletânea |
| `GET` | `/api/openapi.yaml` | especificação OpenAPI (embed, YAML cru) |
| `GET` | `/api/docs` | Swagger UI (assets via CDN) |

Interface web (HTML via templ+HTMX, servida no mesmo binário):

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/stats` | página de estatísticas (shell + placeholder HTMX) |
| `GET` | `/web/stats/data` | fragmento HTML com a tabela de estatísticas (consumido pelo HTMX) |
| `GET` | `/slides` | página de geração de slides (seletor de coletânea + grade de hinos) |
| `GET` | `/web/slides/hinos` | fragmento HTML com a grade de cards dos hinos de uma coletânea (`?codigo=`), consumido pelo HTMX |
| `GET` | `/web/hinos/{codigo}/{numero}/editar` | formulário de edição do hino (título, letra, créditos, revisão) |
| `POST` | `/web/hinos/{codigo}/{numero}` | persiste as alterações do hino (Title Case na letra) e redireciona (303) para `/slides` |
| `GET` | `/static/` | arquivos estáticos (main.css gerado pelo Tailwind) |

As rotas web são mais específicas que o `/` e, por isso, têm prioridade no
mux raiz: a API é delegada para `/` e as páginas/fragmentos web para os
caminhos acima. `main.go` combina ambos via `web.New(apiHandler, statsSvc)`. A
rota `POST /web/hinos/{codigo}/{numero}` é o primeiro ponto de escrita da
aplicação (os demais endpoints continuam somente leitura).

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
  services/                 hino (leitura + atualização de hinos), stats — recebem repos via DI
  api/                      Handlers HTTP finos → JSON; erros → 400/404/500
  version/                  Versão do binário (injetada via -ldflags, ver Makefile)
  web/
    handler.go              Interface web: mux raiz (API em "/" + páginas/fragmentos/static)
    templates/*.templ       Views templ (layout base + stats + slides + editar) → _templ.go gerado
    static/                 input.css (fonte Tailwind) + main.css (gerado) + logo/ícones PNG
  domain/slide_parts.go     TipoParte, ParteHino, SequenciaHino
  processors/lyrics_parser.go  Letra → estrofes/refrões (por indentação)
  ppt/
    layouts.go              LayoutTitulo=1, LayoutEstrofe=2, LayoutRefrao=3 (índices reais do template)
    ppt_generator.go        template → slides → []byte (ZIP/OOXML manual, preserva as partes)
```

Fluxo da API: HTTP (internal/api) → Services → Repository → SQLite. Erros como valores (sentinelas `repository.ErrHinoNotFound`, `repository.ErrColetaneaNotFound`). Escrita (edição de hinos via interface web; geração de slides via download) ocorre sobre os mesmos serviços/repositórios.

Fluxo da interface web: HTTP (internal/web) → Services (mesmos serviços da API, sem chamada HTTP interna) → Templates templ + HTMX. A página `/stats` carrega um placeholder que o HTMX preenche ao fazer `GET /web/stats/data` (`hx-trigger="load"`), devolvendo apenas o fragmento `StatsTable`. A página `/slides` lista coletâneas num combobox; ao trocar a seleção, o HTMX faz `GET /web/slides/hinos?codigo=` (`hx-trigger="change"`, `hx-include="this"`) e substitui o container `#hinos` pelo fragmento `HinosGrid`. A edição de um hino segue o fluxo PRG (Post/Redirect/Get): `GET /web/hinos/{codigo}/{numero}/editar` renderiza o formulário e `POST /web/hinos/{codigo}/{numero}` persiste (via `HinoService.AtualizarHino`, que aplica Title Case na letra e mantém revisão irreversível) e redireciona (303) para `/slides`.

## Convenções

- **Chave de negócio**: coletânea + número (ex.: CC/42); IDs internos não são expostos nas rotas de navegação.
- **Contrato JSON**: tags `snake_case` nos models/serviços; campos opcionais nulos serializados como `null` explícito (sem `omitempty`); `id`/`coletanea_id` visíveis como identificadores internos.
- **Documentação da API**: spec em `internal/api/openapi.yaml` (OpenAPI 3.0.3) e página em `internal/api/docs.html`, ambos embutidos com `go:embed`; rotas declaradas em `API.rotas()` (fonte única usada por `Routes()`); teste de paridade garante spec ↔ mux sincronizados.
- **Resposta de erro**: `{"error": "..."}`; 404 para sentinelas de não encontrado, 400 para parâmetro inválido, 500 genérico com detalhe só no log.
- **Estatísticas** (`services.stats_service.go`): `percentual` = hinos com letra ÷ total × 100; `percentual_revisados` = hinos revisados ÷ hinos com letra × 100. Ambos `0.0` quando o denominador é zero.
- **Detecção de refrão**: todas as linhas do bloco começam com espaço/tab → refrão (indentação removida); senão → estrofe.
- **Interface web**: views `templ` em `internal/web/templates`; o `Layout(title, version, children)` é o esqueleto HTML base (Tailwind + HTMX via CDN); a `version` é exibida no rodapé (`v{version}`). Página (shell) e fragmento (dados) são separados para permitir atualização parcial via HTMX (`hx-get`/`hx-trigger`/`hx-swap`). Os arquivos gerados (`_templ.go`, `main.css`) são commitados; para regenerá-los use `templ generate ./...` e `./scripts/build-css.sh`.
- **Versão** (`internal/version`): variáveis `Version`/`Commit`/`Date` injetadas via `-ldflags -X` no `make build`; `String()` formata (com ou sem commit/data). A versão flui de `main.go` → `web.New` → `Layout` (rodapé).
- **Cards de hinos** (`slides.templ`): exibidos em grade responsiva de até 8 colunas em telas largas (`grid auto-rows-fr grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8`); `auto-rows-fr` + `h-full` no card garantem **altura uniforme** entre linhas. Cada card tem conteúdo **centralizado** (`items-center text-center`), mostra numeração em destaque (`%03d`, fonte maior que o título) e o título na linha abaixo. No **rodapé do card** (linha `mt-auto flex items-center justify-center`, empurrada para a base), há os ícones de ação: `edit.png` (edição, sempre visível, aponta para `/web/hinos/{codigo}/{numero}/editar`) e `ppt.png` (gerar slide, apenas hinos revisados) — ambos em `internal/web/static/`, servidos em `/static/`, com `aria-label`. O `ppt.png` aponta para `/api/coletaneas/{codigo}/hinos/{numero}/slides`. Cor de fundo por estado do hino — sem letra `#FFB7B2`, letra não revisada `#FFF5BA`, revisado `#B5EAD7` — aplicada via `style` inline (cores fora do palette padrão do Tailwind).
- **Edição de hinos** (`editar.templ` + `HinoService.AtualizarHino`): formulário com título, letra (textarea), créditos e checkbox "revisado". Número e coletânea vêm da rota (não editáveis). A letra é convertida para Title Case antes de salvar (`titularLetra`, preservando indentação de refrões e texto já em caixa mista). Revisão é **irreversível**: se o hino já é revisado, o checkbox fica `disabled` e o serviço mantém `Revisado=true` mesmo se o formulário o enviar desmarcado. Uso de `templ.Component` + render via `Render(ctx, w)`; o POST segue PRG (303 → `/slides`).
- **Quebras de linha**: o textarea do formulário pode enviar CRLF (`\r\n`). `HinoService.titularLetra` e `processors.ProcessarHino` normalizam para LF (`normalizeNewlines`), evitando linha em branco extra nos slides. Ordem: `\r\n` → `\n` primeiro, depois `\r` isolado.
- **Blocos**: separados por linha em branco (`\n\s*\n`).
- **Rodapé**: `N/total` no placeholder body de índice 10 (`sz="quarter"`) do template.
- **Exibição por coletânea** (`services.textosSlides`): para coletâneas comuns (≠ Corinhos, código `COR`), o 1º slide mostra `Nome da Coletânea - Número` abaixo do título e os slides de conteúdo usam `NÚMERO{CÓDIGO} - TÍTULO` (ex.: `42CC - Antífona`) no topo direito; para Corinhos, o subtítulo fica vazio e os slides de conteúdo mantêm o título original. Os créditos do hino não são exibidos nos slides.
- **Template `default.pptx`**: layouts `[1] TITULO` (ctrTitle + subTitle), `[2] ESTROFE`, `[3] REFRAO` (title + body idx=1 + body idx=10 para rodapé). **Não alterar esta estrutura** — o gerador depende dela. O slide de título do template (`ppt/slides/slide1.xml`) é reutilizado (injeção de texto); os slides de conteúdo são novos e referenciam os layouts 2/3 via `.rels`.
- **Integridade do pacote**: para cada slide novo o gerador sincroniza 4 registros — `<Override>` no `[Content_Types].xml`, `<Relationship>` no `presentation.xml.rels`, `.rels` do slide→layout e `<p:sldId>` no `presentation.xml`. O teste `TestGeneratedPackageIntegrity` valida essa consistência para que o PowerPoint não peça reparo.
- **Nomenclatura de saída**: `{CODIGO}-{NUM:03d}-{TITULO}.pptx` (título em maiúsculas).
- **Template**: único e fixo (`default.pptx`) — não há seleção de template.

## Ambiente

`data/hinos.db`, `output/`, `logs/`, `.env`, dumps `*.sql` e `*.db-wal` estão no `.gitignore` — não commitar.
