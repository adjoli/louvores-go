# Importação segura no banco de produção

- Tipo: `wayfinder:task` (AFK quando o desenho existir; HITL para o backup/acesso Turso)
- Estado: aberto, não atribuído (bloqueado)
- Bloqueado por: [Desenho da pipeline ETL](06-desenho-pipeline.md)

## Question

Como o artefato revisado entra no SQLite/Turso de produção sem risco:
backup, batch idempotente (insert vs upsert), validação slide a slide e
rollback?

## Contexto

- Turso free só guarda 24h; Adão faz backup manual local de vez em quando —
  Mateus pediu "a versão atual do sql, ou um backup" antes de qualquer
  escrita. `.env` (`TURSO_DATABASE_URL`, `TURSO_AUTH_TOKEN`) dá acesso.
- Regras: `revisado` irreversível (serviço mantém `true`); lote pula
  não-revisados/sem letra/sem numeração (`GerarSlidesColetanea`); nome de
  saída `{CODIGO}-{NUM:03d}-{TITULO}.pptx`; spec OpenAPI + teste de paridade
  precisam continuar passando (`go test ./...`, `go vet ./...`).
- Decidir: pular hinos que já têm letra? sobrescrever só `revisado=false`?
  `INSERT ... ON CONFLICT`? transação única vs por coletânea? como validar
  (`GerarSlides` em staging + `TestGeneratedPackageIntegrity`)?

## Feito quando

- [ ] Backup pré-importação guardado + documentado (onde, quando, quem)
- [ ] Comando de importação definido com `--dry-run` e re-execução segura
      (2ª execução = 0 mudanças)
- [ ] Resposta registra política upsert + validação (quantos gerados/pulados
      no lote, PPTX de amostra abre sem reparo) — aí o mapa está pronto para
      execução
