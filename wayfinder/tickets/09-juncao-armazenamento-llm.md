# Junção, armazenamento e papel do LLM

- Tipo: `wayfinder:grilling` (HITL — Mateus + Adão)
- Estado: fechado (grilling sessão ETL, 09/2026)
- Bloqueado por: [Heurística de refrão (detecção)](04-heuristica-refrao.md)

## Question

Com o refrão já eleito, como reconstruir a letra canônica: rejuntar estrofes
partidas em 2+ slides, gravar o refrão 1× ou N× no banco, e qual papel (se
algum) um LLM tem nisso?

## Contexto

- Estrofes longas são quebradas em 2 slides na projeção (Adão) — evidência:
  `121 É Natal` (cada estrofe em 2 slides), `Salmo 46` (unidade em 3 slides).
- Regra de negócio: blocos separados por linha em branco viram partes/slides
  via `ProcessarHino`; a decisão aqui define onde vai a quebra no texto
  gravado (slides curtos consecutivos não-repetidos = mesma estrofe?).
- Armazenamento: refrão gravado uma única vez (menos linhas para revisar,
  como na proposta `hinos/*.md`) vs repetido após cada estrofe (fiel ao
  cantado). Afeta o mutirão de revisão ([Formato intermediário de revisão](05-formato-revisao.md)).
  Evidência do banco (sessão ETL): CC 7 repete o refrão indentado após
  cada estrofe (8 blocos) — convenção vigente é **N×**; grilling confirma.
- LLM: Adão tentou gerar letra via IA e veio mal formatada e não confiável.
  Opções: sem LLM (só determinístico) vs LLM só classificando candidatos,
  nunca gerando letra do zero.

## Feito quando

## Resolução (grilling, Mateus 09/2026)

- Junção mínima: slides curtos consecutivos não-repetidos que somam
  ≤8 linhas = mesma estrofe; slide com 4+ linhas não junta; nunca
  junta com refrão candidato. Casos duvidosos ficam para o revisor.
- Armazenamento: **N×** (convenção vigente, ex. CC 7).
- **Sem LLM** na ETL (regra determinística + confirmação humana).
