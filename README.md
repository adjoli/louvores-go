# louvores-go

Geração automatizada de slides PowerPoint para hinos e louvores cristãos a partir de um template `.pptx`, com banco SQLite. Porte em Go da aplicação [Louvores](https://github.com/anomalyco/opencode) (Python).

## Funcionalidades

- API REST de leitura sobre o banco de hinos
- Interface web (templ) sobre os mesmos serviços — ver estatísticas, listar e gerenciar hinos/slides
- Estatísticas por coletânea (percentual de hinos com letra e percentual de revisados sobre os que têm letra)
- Separação inteligente de estrofes e refrões por indentação
- Edição de hinos pela interface web (título, letra com Title Case, créditos e revisão irreversível)
- Criação de hinos pela interface web, disponível apenas na coletânea Corinhos, com numeração automática (último + 1)
- Geração e download de slides (PPTX) a partir de um template único, preservando todas as partes do template
- Revisão de letras (aprovação) — implementada na edição, irreversível
- Autenticação por senha única compartilhada, com sessão em cookie assinado (HMAC) e tela de login

## Requisitos

- Go ≥ 1.25

## Instalação

```bash
go mod tidy
make build   # compila com versão (VERSION, commit git e data via -ldflags)
```

## Uso

```bash
./louvores          # sobe o servidor HTTP (padrão :8080)
./louvores -version # imprime a versão do binário
```

A versão pode ser definida em `make build VERSION=1.2.3` (default `dev`); o commit e a data vêm do git. Ela é exibida no rodapé das páginas web.

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

Erros retornam `{"error": "..."}` com status 404 (coletânea/hino inexistente), 400 (número inválido), 409 (hino não revisado), 401 (não autenticado) ou 500. O contrato JSON usa snake_case; campos opcionais ausentes no banco são serializados como `null`. Um teste garante a paridade entre as rotas registradas e a spec OpenAPI.

Com a autenticação ativa, os endpoints `/api/*` exigem sessão válida (exceto `/api/healthz`) e respondem **401** com `{"error":"não autenticado"}` quando ela falta.

### Interface web

Uma interface web HTML é servida no mesmo binário, em `http://localhost:8080/`, usando `templ` (templates tipados em Go). As páginas são servidas como HTML completo, já com os dados embutidos no render (sem HTMX). Os estilos ficam em `internal/web/static/main.css`, mantido manualmente (sem build).

| Rota | Descrição |
|---|---|
| `GET /` | redireciona (302) para `/slides` (destino do logo no cabeçalho) |
| `GET /login` | formulário de login (senha única) |
| `POST /login` | valida a senha, emite o cookie de sessão e redireciona (303) para `/slides` |
| `POST /logout` | invalida o cookie de sessão e redireciona (303) para `/login` |
| `GET /stats` | página de estatísticas (tabela embutida no HTML) |
| `GET /slides` | página de geração de slides (seletor de coletânea; `?codigo=` preenche a grade de hinos) |
| `GET /web/hinos/{codigo}/novo` | formulário de criação de hino (apenas Corinhos; 404 nas demais) |
| `POST /web/hinos/{codigo}` | cria o hino (numeração automática = maior + 1) e redireciona (303) para `/slides?codigo={codigo}` |
| `GET /web/hinos/{codigo}/{numero}/editar` | formulário de edição do hino (título, letra, créditos, revisão) |
| `POST /web/hinos/{codigo}/{numero}` | persiste as alterações do hino e redireciona (303) para `/slides` |
| `GET /static/` | arquivos estáticos (main.css, logo/ícones) |

A página `/stats` renderiza a tabela de estatísticas diretamente no HTML. A página `/slides` exibe um combobox de coletâneas; ao selecionar e enviar o formulário (`GET /slides?codigo=`), a página recarrega com a grade de cards dos hinos (responsiva, até 8 colunas, altura uniforme, conteúdo centralizado). Cada card mostra a numeração em destaque (três dígitos, fonte maior que o título) com o título abaixo, e cor de fundo por estado (sem letra `#FFB7B2`, não revisado `#FFF5BA`, revisado `#B5EAD7`). No rodapé do card há os ícones de ação: edição (`edit.png`, abre o formulário de edição) e geração de slide (`ppt.png`, apenas hinos revisados, apontando para o endpoint de download). Na edição, a letra é convertida para **Title Case** antes de salvar, e a revisão é **irreversível** (checkbox desabilitado para hinos já revisados).

A criação de hino é permitida **apenas na coletânea Corinhos** (`COR`): com ela selecionada, a grade exibe o botão "Adicionar hino" que leva a `GET /web/hinos/{codigo}/novo`. O formulário não tem campo de numeração — o serviço atribui automaticamente a próxima numeração (maior existente + 1) e aplica Title Case na letra. Os handlers web reutilizam os mesmos serviços da API (sem chamada HTTP interna). Detalhes em `AGENTS.md`.

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
| `TURSO_DATABASE_URL` | *(vazio — usa o SQLite local)* |
| `TURSO_AUTH_TOKEN` | *(vazio)* |
| `TEMPLATE_PATH` | `data/templates/default.pptx` |
| `LOG_PATH` | `logs/app.log` |
| `HOST` | *(vazio — todas as interfaces)* |
| `PORT` | `8080` |
| `AUTH_PASSWORD` | *(vazio — autenticação desligada)* |
| `SESSION_SECRET` | *(vazio — chave aleatória em runtime)* |
| `COOKIE_SECURE` | `false` |
| `SESSION_TTL` | `24h` |

Um `.env` opcional é carregado na inicialização.

### Autenticação

A aplicação usa **senha única compartilhada** com sessão em cookie assinado (HMAC-SHA256) — sem tabela de usuários. Defina `AUTH_PASSWORD` para ativar a proteção; com ela vazia, a autenticação fica desligada (útil apenas em desenvolvimento local).

Com a autenticação ativa, **tudo fica protegido** — API REST, páginas web e geração de slides — exceto as rotas públicas `/login`, `/api/healthz` e `/static/`. Requisições não autenticadas a `/api/*` recebem **401**; navegadores são redirecionados para `/login`. O cookie é `HttpOnly` e `SameSite=Lax`. Após o login (e ao acessar a raiz `/`), o usuário é levado à página `/slides`.

```bash
export AUTH_PASSWORD="minha-senha"
export SESSION_SECRET="$(openssl rand -hex 32)"   # evita logout a cada restart
export COOKIE_SECURE=true                          # atrás de HTTPS
./louvores
```

- `SESSION_SECRET` assina o cookie de sessão. Se não for definida, uma chave aleatória é gerada em runtime e as sessões caem a cada restart (um aviso é registrado).
- `COOKIE_SECURE=true` faz o navegador enviar o cookie apenas via HTTPS; deixe `false` apenas em acesso local por HTTP.
- `SESSION_TTL` controla a validade da sessão (ex.: `12h`).

### Banco de dados: Turso na nuvem ou SQLite local

Quando `TURSO_DATABASE_URL` está definida (ex.: `libsql://meu-banco.turso.io`), a aplicação conecta ao **Turso na nuvem** usando o driver `libsql-client-go` (puro Go, sem CGO); `TURSO_AUTH_TOKEN` é **obrigatório** nesse modo. Caso a variável não esteja definida, a aplicação mantém o **SQLite local** em `DB_PATH` (`modernc.org/sqlite`). O schema é aplicado automaticamente nos dois modos (`Migrate`, DDL idempotente).

```bash
export TURSO_DATABASE_URL="libsql://meu-banco.turso.io"
export TURSO_AUTH_TOKEN="seu-token-aqui"
./louvores
```

Nunca comite o token — mantenha-o no `.env` (já ignorado pelo git) ou em variáveis de ambiente.

## Testes

```bash
go test ./...
```

## Desenvolvimento da interface web

Os arquivos `_templ.go` (gerados por `templ`) são **commitados** — o binário funciona sem a toolchain de frontend. O `main.css` é mantido manualmente. Para regenerar os templates após alterar `.templ`:

```bash
templ generate ./...            # gera _templ.go a partir de *.templ
```

## Arquitetura

`internal/app` (composition root) monta as dependências e as injeta nos handlers HTTP — `internal/api` (REST/JSON) e `internal/web` (interface HTML/templ) — → `internal/services` → `internal/repository` → SQLite (`modernc.org/sqlite`) ou Turso (`libsql-client-go`), conforme `TURSO_DATABASE_URL`. O `internal/auth` fornece o middleware de sessão que envolve todas as rotas (exceto as públicas). Detalhes em `AGENTS.md`.

### Geração de slides (PPTX)

A geração (`internal/ppt`) manipula o pacote OOXML diretamente (`archive/zip` + `encoding/xml`), preservando **byte-a-byte** todas as partes do template e apenas acrescentando/registrando os slides novos. Para cada slide novo ela sincroniza quatro fontes de verdade — `[Content_Types].xml`, `presentation.xml.rels`, o `.rels` do slide→layout e o `sldIdLst` em `presentation.xml` — de modo que o PowerPoint abra o arquivo sem pedir reparo. O teste `TestGeneratedPackageIntegrity` valida essa consistência. O gooxml é usado somente como validador de reabertura nos testes. O endpoint `GET /api/coletaneas/{codigo}/hinos/{numero}/slides` aciona a geração e devolve o arquivo como download.

## Licença

MIT.
