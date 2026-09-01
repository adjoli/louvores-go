# louvores-go

Geração automatizada de slides PowerPoint para hinos e louvores cristãos a partir de um template `.pptx`, com banco SQLite. Porte em Go da aplicação [Louvores](https://github.com/anomalyco/opencode) (Python).

## Funcionalidades

- API REST de leitura sobre o banco de hinos
- Interface web (templ + HTMX + Tailwind) consumindo a API — ver estatísticas e slides
- Estatísticas por coletânea (percentual de hinos com letra e percentual de revisados sobre os que têm letra)
- Separação inteligente de estrofes e refrões por indentação
- Edição de hinos pela interface web (título, letra com Title Case, créditos e revisão irreversível)
- Geração e download de slides (PPTX) a partir de um template único, preservando todas as partes do template
- Revisão de letras (aprovação) — implementada na edição, irreversível

## Requisitos

- Go ≥ 1.25

## Instalação

```bash
go mod tidy
go build ./cmd/louvores
```

## Uso

```bash
louvores    # sobe o servidor HTTP (padrão :8080)
```

### Endpoints

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/api/healthz` | liveness |
| `GET` | `/api/coletaneas` | lista as coletâneas |
| `GET` | `/api/coletaneas/{codigo}/hinos` | hinos da coletânea, ordenados pela numeração |
| `GET` | `/api/coletaneas/{codigo}/hinos/{numero}` | detalhe do hino (ex.: CC/42) |
| `GET` | `/api/coletaneas/{codigo}/hinos/{numero}/slides` | gera e baixa o PPTX dos slides do hino |
| `POST` | `/api/coletaneas/{codigo}/slides/lote` | gera um ZIP com o PPTX de cada hino revisado da coletânea |
| `GET` | `/api/stats` | estatísticas agregadas por coletânea |
| `GET` | `/api/openapi.yaml` | especificação OpenAPI (embutida no binário) |
| `GET` | `/api/docs` | documentação interativa (Swagger UI) |

A documentação interativa fica em http://localhost:8080/api/docs — os assets do Swagger UI são carregados via CDN.

Erros retornam `{"error": "..."}` com status 404 (coletânea/hino inexistente), 400 (número inválido), 409 (hino não revisado) ou 500. O contrato JSON usa snake_case; campos opcionais ausentes no banco são serializados como `null`. Um teste garante a paridade entre as rotas registradas e a spec OpenAPI.

### Interface web

Uma interface web HTML é servida no mesmo binário, em `http://localhost:8080/`, usando `templ` (templates tipados em Go), **HTMX** (atualização parcial assíncrona) e **Tailwind CSS**.

| Rota | Descrição |
|---|---|
| `GET /stats` | página de estatísticas (shell + placeholder carregado via HTMX) |
| `GET /web/stats/data` | fragmento HTML com a tabela (consumido pelo HTMX) |
| `GET /slides` | página de geração de slides (seletor de coletânea + grade de hinos) |
| `GET /web/slides/hinos` | fragmento HTML com os cards dos hinos (`?codigo=`, consumido pelo HTMX) |
| `GET /web/hinos/{codigo}/{numero}/editar` | formulário de edição do hino (título, letra, créditos, revisão) |
| `POST /web/hinos/{codigo}/{numero}` | persiste as alterações do hino e redireciona (303) para `/slides` |
| `GET /static/` | arquivos estáticos (CSS gerado pelo Tailwind) |

A página `/stats` renderiza o layout base e um placeholder; o HTMX faz `GET /web/stats/data` (`hx-trigger="load"`) e substitui o placeholder pelo fragmento `StatsTable`. A página `/slides` exibe um combobox de coletâneas; ao trocar a seleção, o HTMX busca `GET /web/slides/hinos?codigo=` e substitui o container pela grade de cards dos hinos (responsiva, até 8 colunas, altura uniforme, conteúdo centralizado). Cada card mostra a numeração em destaque (três dígitos, fonte maior que o título) com o título abaixo, e cor de fundo por estado (sem letra `#FFB7B2`, não revisado `#FFF5BA`, revisado `#B5EAD7`). No rodapé do card há os ícones de ação: edição (`edit.png`, abre o formulário de edição) e geração de slide (`ppt.png`, apenas hinos revisados, apontando para o endpoint de download). Na edição, a letra é convertida para **Title Case** antes de salvar, e a revisão é **irreversível** (checkbox desabilitado para hinos já revisados). Os handlers web reutilizam os mesmos serviços da API (sem chamada HTTP interna). Detalhes em `AGENTS.md`.

### Geração de slides

`GET /api/coletaneas/{codigo}/hinos/{numero}/slides` gera o PPTX dos slides do hino e o retorna como download (`{CODIGO}-{NUM:03d}-{TITULO}.pptx`). Apenas hinos revisados geram slides — hinos não revisados retornam **409**.

O conteúdo dos slides varia conforme a coletânea:

- **Coletâneas comuns** (≠ Corinhos): no primeiro slide, o campo abaixo do título mostra `Nome da Coletânea - Número` (ex.: `Cantor Cristão - 42`); nos slides de conteúdo, o título do canto superior direito recebe o prefixo `NÚMERO{CÓDIGO}`, no formato `NÚMERO{CÓDIGO} - TÍTULO` (ex.: `42CC - Antífona`).
- **Corinhos** (código `COR`): o campo abaixo do título do primeiro slide fica em branco e os slides de conteúdo mantêm o título original, sem prefixo.

Os créditos do hino não são mais exibidos nos slides.

`POST /api/coletaneas/{codigo}/slides/lote` gera um **ZIP** com o PPTX de todos os hinos revisados (e com letra) da coletânea, baixado como `{CODIGO}-slides.zip`. Hinos não revisados, sem letra ou com erro de geração são pulados.

## Dados

- `data/templates/default.pptx` — template dos slides (layouts: título, estrofe, refrão)

## Configuração

Variáveis de ambiente (todas com defaults):

| Variável | Default |
|---|---|
| `DB_PATH` | `data/hinos.db` |
| `TEMPLATE_PATH` | `data/templates/default.pptx` |
| `LOG_PATH` | `logs/app.log` |
| `HOST` | *(vazio — todas as interfaces)* |
| `PORT` | `8080` |

Um `.env` opcional é carregado na inicialização.

## Testes

```bash
go test ./...
```

## Desenvolvimento da interface web

Os arquivos `_templ.go` (gerados por `templ`) e o `main.css` (gerado pelo Tailwind) são **commitados** — o binário funciona sem a toolchain de frontend. Para regenerá-los após alterar `.templ`/CSS:

```bash
templ generate ./...            # gera _templ.go a partir de *.templ
./scripts/build-css.sh          # gera internal/web/static/main.css (Tailwind)
```

## Arquitetura

`internal/app` (composition root) monta as dependências e as injeta nos handlers HTTP — `internal/api` (REST/JSON) e `internal/web` (interface HTML/templ+HTMX) — → `internal/services` → `internal/repository` → SQLite (`modernc.org/sqlite`). Detalhes em `AGENTS.md`.

### Geração de slides (PPTX)

A geração (`internal/ppt`) manipula o pacote OOXML diretamente (`archive/zip` + `encoding/xml`), preservando **byte-a-byte** todas as partes do template e apenas acrescentando/registrando os slides novos. Para cada slide novo ela sincroniza quatro fontes de verdade — `[Content_Types].xml`, `presentation.xml.rels`, o `.rels` do slide→layout e o `sldIdLst` em `presentation.xml` — de modo que o PowerPoint abra o arquivo sem pedir reparo. O teste `TestGeneratedPackageIntegrity` valida essa consistência. O gooxml é usado somente como validador de reabertura nos testes. O endpoint `GET /api/coletaneas/{codigo}/hinos/{numero}/slides` aciona a geração e devolve o arquivo como download.

## Licença

MIT.
