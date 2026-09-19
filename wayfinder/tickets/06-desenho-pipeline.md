# Desenho da pipeline ETL

- Tipo: `wayfinder:prototype` (HITL — artefato barato para reagir)
- Estado: aberto, não atribuído (bloqueado)
- Bloqueado por: [Inventário do corpus legado](01-inventario-corpus.md), [Heurística de refrão (detecção)](04-heuristica-refrao.md), [Junção, armazenamento e papel do LLM](09-juncao-armazenamento-llm.md), [Formato intermediário de revisão](05-formato-revisao.md), [Extração dos .ppt binários (OLE)](08-extracao-ppt-ole.md)

## Question

Qual é o desenho ponta-a-ponta (extrair → normalizar → revisar → importar):
linguagem/ferramentas, onde roda, e como cada etapa é reproduzível?

## Contexto

- Entrada: Drive/NOVA IDENTIDADE (`.pptx` + `.ppt` OLE) +
  [Inventário do corpus legado](01-inventario-corpus.md) (o volume por
  formato decide o tratamento dos `.ppt`: conversor automático vs triagem
  manual, ver [Extração dos .ppt binários](08-extracao-ppt-ole.md)).
- Normalização usa [Formato esperado pelo banco](02-formato-esperado.md) +
  detecção do [Heurística de refrão](04-heuristica-refrao.md) +
  reconstrução do [Junção, armazenamento e papel do LLM](09-juncao-armazenamento-llm.md);
  saída da revisão usa [Formato intermediário](05-formato-revisao.md).
- Restrições: código prod não importa `gooxml` (só validador em testes);
  `main.css`/`templ` irrelevantes; sem CGO (preferir `archive/zip` +
  `encoding/xml` em Go ou script Python com `zipfile`).
- Restrição assentada: a ETL é um programa **separado** (CLI simples sobre
  uma pasta de `pptx`/`ppt`) — **não modifica** `internal/ppt`,
  `processors`, `services` nem o template. Ela termina no artefato de
  revisão/importação; a geração de slides continua intacta.
- Interface assentada (Mateus, 09/2026): **CLI em Go com TUI `pterm`**
  (só para o operador): `DefaultInteractiveSelect` (menu),
  `DefaultInteractiveTextInput` (path do input), `DefaultProgressbar`
  (progresso do import). Comandos: `extrair` (pasta → txt + corpus.csv),
  `revisao-pack` (gera pacote de revisão), `importar --csv` (planilha
  revisada → banco, `.env` decide dev vs prod). **Dry-run obrigatório**
  antes de qualquer escrita, mostrando novos vs modificados vs pulados,
  global e por coletânea.
- Match em estágios (codex, sessão ETL): número → título exato →
  similaridade (tokens/refrão) só como `candidato` para revisão humana
  (candidato único acima do limiar E com margem; nunca autoaceito em
  `chave_db`).

## Progresso (sessão ETL, 09/2026)

- Protótipo funcional em `cmd/etl/main.go` (Go, sem deps novas): lê a
  planilha, dry-run obrigatório (novos vs modificados vs pulados, por
  coletânea), grava em transação única com `RowsAffected=1`, trava de
  segurança (recusa `data/hinos.db`), `-check CODIGO/NUM` exibe blocos
  parseados. Testado em `data/hinos-test.db` (clone, gitignored) com
  `wayfinder/evidencia/amostra-revisao.csv` (3 hinos OK): CC/36 parseia em
  estrofe/refrão/estrofe/refrão, Title Case igual ao manual, original
  intacto, `go test ./...` verde.
- Implementado (sessão ETL, `internal/etl` + `cmd/etl`, 17 testes): lotes
  transacionais (default 100; falha reverte só o lote e re-rodar retoma,
  pois gravados viram pulados); exceção à regra zero-mudanças: exportar
  helpers puros (`services.TitularLetra`) é permitido — proibido é mexer
  em `ppt`/`processors`/template.

## Feito quando

- [ ] Protótipo (não a pipeline final): diagrama + esqueleto **separado do
      binário principal** (`cmd/etl --dry-run` ou script + `Makefile`
      target) que processa a pasta `wayfinder/evidencia/` e extrai 2 hinos
      até o formato de revisão, sem tocar no código de geração
- [ ] Resposta linka o protótipo e trava: Go vs Python por etapa, comando de
      cada fase, onde roda (local vs CI), e como re-rodar sem duplicar
