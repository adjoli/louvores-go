# louvores-go

Geração automatizada de slides PowerPoint para hinos e louvores cristãos a partir de um template `.pptx`, com banco SQLite. Porte em Go da aplicação [Louvores](https://github.com/anomalyco/opencode) (Python).

## Funcionalidades

- API REST de leitura sobre o banco de hinos (fase atual)
- Estatísticas por coletânea
- Separação inteligente de estrofes e refrões por indentação
- Revisão de letras (aprovação) *(planejado)*
- Geração automática de slides (PPTX) a partir de um template único *(planejado)*

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

### Endpoints (fase atual: somente leitura)

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/api/healthz` | liveness |
| `GET` | `/api/coletaneas` | lista as coletâneas |
| `GET` | `/api/coletaneas/{codigo}/hinos` | hinos da coletânea, ordenados pela numeração |
| `GET` | `/api/coletaneas/{codigo}/hinos/{numero}` | detalhe do hino (ex.: CC/42) |
| `GET` | `/api/stats` | estatísticas agregadas por coletânea |

Erros retornam `{"error": "..."}` com status 404 (coletânea/hino inexistente), 400 (número inválido) ou 500.

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

## Arquitetura

`internal/app` (composition root) monta as dependências e as injeta nos handlers HTTP (`internal/api`) → `internal/services` → `internal/repository` → SQLite (`modernc.org/sqlite`). Detalhes em `AGENTS.md` e `docs/fluxo_execucao.md`.

## Licença

MIT.
