# Formato intermediário de revisão

- Tipo: `wayfinder:grilling` (HITL — Mateus + Jônatas + Gabriel + louvor)
- Estado: aberto, não atribuído (bloqueado)
- Bloqueado por: [Formato esperado pelo banco](02-formato-esperado.md), [Junção, armazenamento e papel do LLM](09-juncao-armazenamento-llm.md)

## Question

Qual artefato a extração gera para o mutirão revisar (comparando com o
hinário) antes de virar importação final: markdown por hino, CSV/SQL gigante,
ou página temporária — e onde ele vive?

## Contexto

- Exigência do Mateus: "etapa intermediária de revisão entre a extração e o
  resultado final pronto para importar no banco de produção".
- Ideias no chat: CSV ou SQL gigante em batch (evita lidar com o SQLite
  direto); Adão: "baixar o arquivo e trabalhar localmente"; Mateus propôs
  `hinos/<hinario>/*.md` + git (overkill, fora de escopo — banco segue fonte).
- Revisores: Jônatas/Gabriel passam o olho letra a letra vs hinário; louvor
  valida o cantado. O artefato de revisão é **temporário e não versionado**
  (pasta local, Drive ou página temporária) — versionar letras em git (repo
  privado por direitos autorais) está fora de escopo, ver mapa. Atenção à
  busca por palavra-chave (motivo do banco seguir como fonte).
- Formato tem que servir a **não-técnicos** (ver mapa): planilha em
  primeiro lugar (Google Sheets/Excel, uma linha por hino, colunas de
  decisão simples); nada de git, terminal ou CSV cru como interface
  principal.
- Decisão assentada (Mateus, 09/2026): revisão no **Google Sheets**
  (colaborativo). A planilha leva **todos os slides**, com a letra
  integral editável (correção direto na célula, sem coluna de comentário),
  ordenada por **pasta de origem** (coluna `pasta` no CSV). Status:
  `IMPORTADO` (letra já no banco), `PENDENTE` (falta revisar/importar),
  `IGNORAR` (versão antiga). Arquivo: `corpus/revisao.csv` (gitignored,
  2282 linhas: pasta, arquivo, hinario, numero, titulo, letra, status,
  observacao).
- Instruções aos revisores em `wayfinder/evidencia/instrucoes-revisao.md`
  (regras + exemplo antes/depois): refrão repetido N× com 2 espaços por
  linha, sem `--- slide ---`/contadores/rodapés/numeração; correção na
  própria célula; `PENDENTE` → **`OK`** quando pronto (importador lê `OK`).

## Feito quando

- [ ] Grilling trava: colunas/campos do artefato (base `corpus.csv` +
      colunas de revisão codex: `titulo_db`, `candidatos_db`, `score`,
      `metodo`, `trecho_inicial`, `refrao_fingerprint`, `num_slides`,
      `arquivo_vencedor`, `decisao`, `motivo_decisao`, `revisor_em` —
      preview que permite decidir o link em segundos por linha),
      local (preferir pasta local ou Drive; página web temporária só como
      fallback com dono explícito — o middleware protege tudo exceto
      `/login`, `/api/healthz` e `/static/`), e critério de "pronto para
      importar" (quem dá o OK por hino)
- Decisão codex (sessão ETL): UI de import/link **adiada** — primeiro um
  CLI idempotente que valida e aplica decisões explícitas do CSV (dry-run
  + relatório de reconciliação). Página web só se a revisão recorrente
  superar o conforto de editar CSV.
- [ ] Resposta registra o formato escolhido + modelo de 1 hino revisado
