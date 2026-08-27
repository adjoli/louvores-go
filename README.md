# louvores-go

Geração automatizada de slides PowerPoint para hinos e louvores cristãos a partir de um template `.pptx`, com banco SQLite. Porte em Go da aplicação [Louvores](https://github.com/anomalyco/opencode) (Python).

## Funcionalidades

- API REST de leitura sobre o banco de hinos (fase atual)
- Estatísticas por coletânea
- Separação inteligente de estrofes e refrões por indentação
- Geração de slides (PPTX) a partir de um template único, preservando todas as partes do template
- Revisão de letras (aprovação) *(planejado)*
- Endpoint HTTP para download dos slides *(planejado — a geração já está disponível via serviço)*

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
| `GET` | `/api/openapi.yaml` | especificação OpenAPI (embutida no binário) |
| `GET` | `/api/docs` | documentação interativa (Swagger UI) |

A documentação interativa fica em http://localhost:8080/api/docs — os assets do Swagger UI são carregados via CDN.

Erros retornam `{"error": "..."}` com status 404 (coletânea/hino inexistente), 400 (número inválido) ou 500. O contrato JSON usa snake_case; campos opcionais ausentes no banco são serializados como `null`. Um teste garante a paridade entre as rotas registradas e a spec OpenAPI.

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

`internal/app` (composition root) monta as dependências e as injeta nos handlers HTTP (`internal/api`) → `internal/services` → `internal/repository` → SQLite (`modernc.org/sqlite`). Detalhes em `AGENTS.md`.

### Geração de slides (PPTX)

A geração (`internal/ppt`) manipula o pacote OOXML diretamente (`archive/zip` + `encoding/xml`), preservando **byte-a-byte** todas as partes do template e apenas acrescentando/registrando os slides novos. Para cada slide novo ela sincroniza quatro fontes de verdade — `[Content_Types].xml`, `presentation.xml.rels`, o `.rels` do slide→layout e o `sldIdLst` em `presentation.xml` — de modo que o PowerPoint abra o arquivo sem pedir reparo. O teste `TestGeneratedPackageIntegrity` valida essa consistência. O gooxml é usado somente como validador de reabertura nos testes.

## Licença

MIT.
