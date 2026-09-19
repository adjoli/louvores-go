# Regras de importação segura

- Tipo: `wayfinder:grilling` (HITL — Mateus + Adão; acesso ao Turso na execução)
- Estado: fechado (regras travadas; execução fica para pós-merge)
- Bloqueado por: [Desenho da pipeline ETL](06-desenho-pipeline.md)

## Question

Quais são as regras para o artefato revisado entrar no SQLite/Turso de
produção sem risco: unicidade, upsert, invariante de revisão e validação?
(O backup e a importação real acontecem na **execução**, depois do mapa —
aqui só se decide o COMO.)

## Contexto

- Turso free só guarda 24h; Adão faz backup manual local de vez em quando —
  Mateus pediu "a versão atual do sql, ou um backup" antes de qualquer
  escrita. `.env` (`TURSO_DATABASE_URL`, `TURSO_AUTH_TOKEN`) dá acesso.
- Unicidade: `(coletanea_id, numeracao)` tem índice **não-único**
  (`internal/database/db.go:51`) — `INSERT ... ON CONFLICT` sozinho não
  impede duplicatas. A regra precisa de preflight de duplicatas (por chave
  de negócio `CODIGO/numero`) ou garantia transacional equivalente.
- Invariante: revisão é irreversível no serviço (`AtualizarHino` mantém
  `true`), mas SQL direto bypassa — **nenhuma importação pode levar
  `revisado` de `true` para `false`** nem sobrescrever letra de hino já
  revisado sem OK explícito.
- Guardas do importador (codex, sessão ETL): validar que cada `chave_db`
  resolve **exatamente um hino** e que `letra IS NULL`; atualizar por `id`
  com `RowsAffected=1`, em transação; **rejeitar texto vazio**
  (`letra = ''` conta como "com letra" no `COUNT(h.letra)`); ETL toca
  **somente `letra`** (nunca título/créditos/revisão — o serviço web os
  alteraria de brinde).
- Política default (implementada): **pular quem já tem letra** (só
  `--force` sobrescreve, com OK explícito); reexecução = 0 mudanças;
  backup automático `<db>.bak-<ts>` antes de escrever; `--init` exigido
  para criar banco novo.
- Decisões dia-da-prod (Mateus, 09/2026): import **tudo de uma vez**
  (~822, um dry-run + uma gravação); **`--force` nunca em prod**
  (só entra letra onde está NULL).
- Completude = reconciliação (codex): cada uma das 1187 chaves termina em
  exatamente um estado terminal — `importado`, `sem-fonte-confirmado`,
  `ambiguo-confirmado`, `ole-pendente`, `erro-extracao` ou
  `descartado-com-motivo`. Totais globais e por coletânea, sem chave
  duplicada; preservar LF/indentação (afeta detecção de refrão).
- Regras de lote: `GerarSlidesColetanea` pula não-revisados/sem letra/sem
  numeração; nome de saída `{CODIGO}-{NUM:03d}-{TITULO}.pptx`; spec OpenAPI +
  teste de paridade precisam continuar passando (`go test ./...`).

## Resolução (regras travadas, sessão ETL 09/2026)

- (a) unicidade: preflight por `CODIGO/numero` (índice não-único) +
  idempotência (2ª execução = 0 mudanças) — implementado.
- (b) upsert: pular quem já tem letra; `--force` nunca em prod.
- (c) validação de todos os alterados: procedimento do dia-da-prod
  (gerar slides + abrir amostra + suíte verde) — ver
  `cmd/etl/AGENTS.md`; execução pós-merge.
- (d) dry-run obrigatório + backup prévio — implementado (`-check`,
  `<db>.bak-<ts>`, `--init`).
