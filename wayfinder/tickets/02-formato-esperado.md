# Formato esperado pelo banco

- Tipo: `wayfinder:research` (AFK)
- Estado: fechado (OK do Mateus, sessão ETL 09/2026)
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

## Resolução

Contrato confirmado pelo subagente + correção posterior (unitário vs
lote com letra nil, verificado no código): DDL, exemplo canônico,
normalização e bloqueios registrados em
`wayfinder/evidencia/research-formato-esperado.md`. OK do Mateus.
