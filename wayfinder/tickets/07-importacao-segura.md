# Regras de importação segura

- Tipo: `wayfinder:grilling` (HITL — Mateus + Adão; acesso ao Turso na execução)
- Estado: aberto, não atribuído (bloqueado)
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
- Regras de lote: `GerarSlidesColetanea` pula não-revisados/sem letra/sem
  numeração; nome de saída `{CODIGO}-{NUM:03d}-{TITULO}.pptx`; spec OpenAPI +
  teste de paridade precisam continuar passando (`go test ./...`).

## Feito quando

- [ ] Grilling trava: (a) estratégia de unicidade/idempotência (2ª execução
      = 0 mudanças), (b) política upsert (pular com letra? sobrescrever só
      `revisado=false`? com qual WHERE que preserve o invariante),
      (c) validação: **gerar e verificar os slides de TODOS os hinos
      alterados** (PPTX abre sem reparo, `TestGeneratedPackageIntegrity`
      verde), cada skip do lote justificado — contagem + amostra não basta,
      (d) comando com `--dry-run` e exigência de backup prévio documentado
- [ ] Resposta registra as 4 regras — aí o mapa está pronto para execução
      (backup + importação real ficam para depois do mapa)
