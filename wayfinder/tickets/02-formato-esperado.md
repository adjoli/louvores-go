# Formato esperado pelo banco

- Tipo: `wayfinder:research` (AFK)
- Estado: aberto, não atribuído (fronteira — pode ser pego agora)
- Bloqueado por: nada

## Question

Qual é exatamente o contrato que a letra importada precisa satisfazer para
que `ProcessarHino` → `GerarSlides` funcione sem reparo: schema, normalização,
Title Case, blocos e regra estrofe/refrão?

## Contexto

- Fontes (não perguntar ao usuário — ler no repo): `internal/database/db.go`
  (tabelas `coletanea`/`hino`, `revisado INTEGER DEFAULT 0`),
  `internal/models/models.go` (`Numeracao/Letra/Creditos` ponteiro = NULL),
  `internal/processors/lyrics_parser.go` (split `\n\s*\n`, refrão = TODAS as
  linhas com espaço/tab, `normalizeNewlines` CRLF→LF),
  `internal/services/hino_service.go` (`titularLetra`, revisão irreversível,
  `textosSlides` com regra COR sem prefixo, `nomeArquivoSlides`).
- Adão confirmou (chat 18/09): "se a letra seguir a indentação do refrão,
  basta dar insert com letra+título+créditos e o slide gera".

## Feito quando

- [ ] Subagente de research leu os 4 arquivos acima + `internal/domain/slide_parts.go`
- [ ] Resposta registra: DDL, exemplo canônico de letra (estrofe + refrão
      indentado + linha em branco), o que zera `percentual` em stats, e o que
      impede geração — distinguindo unitário (só `!revisado` e sem
      `numeracao`; letra nil gera PPTX só-título sem erro) de lote
      (pula `!revisado`, letra nil e sem `numeracao`)
