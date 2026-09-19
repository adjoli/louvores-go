# Inventário do corpus legado

- Tipo: `wayfinder:task` (HITL — precisa de acesso ao Drive/NOVA IDENTIDADE)
- Estado: fechado (resolvido na sessão ETL, 09/2026)
- Bloqueado por: nada

## Question

Qual é o universo real a importar: quantos arquivos PPT/PPTX existem no
Drive (pasta NOVA IDENTIDADE, por coletânea CC/HL/VM/HCC/COR), quais são
duplicatas, e como cada chave de negócio (coletânea+número) cruza com o
banco — com letra, sem letra com slide, sem letra sem slide?

## Contexto

- Hoje só temos 10 amostras em `wayfinder/evidencia/` (8× `.pptx` + 2× `.ppt`
  binário OLE: `Ele é meu e teu Senhor.ppt`, `Porque Ele vive.ppt`).
- Adão (chat 18/09): sem letra = 2 situações — erro na extração OU hino não
  cantado (sem slides nas pastas). Faltam ~300, sobretudo HL/HCC/corinhos.
- Sem esse número não dá para dimensionar mutirão nem lote.
- Regra assentada (Mateus, 09/2026): **versão mais recente vence** — o mesmo
  hino tem várias versões (2010, 2016, 2024…; 487 grupos duplicados por nome
  normalizado no corpus de 09/2026). Na deduplicação por chave de negócio,
  vale o arquivo com **mtime mais novo**; os demais são ignorados (não
  deletados).

## Progresso (sessão ETL, 09/2026)

- Fonte: `corpus/canticos/` (extração do zip de 1.1G): **2282 slides**
  (`1632 pptx` + `72 ppt` + `578 pps`); resto é lixo (8352 jpg, 152 pdf,
  mp3, bak, lnk) ignorado. mtimes 2006–2026.
- Texto extraído dos 1630 pptx válidos em `corpus/txt/` (espelho de paths,
  gitignored); **650 OLE** (72 ppt + 578 pps — `.pps` aqui é binário
  antigo, não OOXML) com extração adiada para o 08; 5 pptx corrompidos.
- `wayfinder/evidencia/corpus.csv` (2282 linhas, todos os slides, nada
  descartado): arquivo, coletânea_sugerida, numero_extraido,
  titulo_extraido (1ª linha do slide 1), bytes, formato, mtime, status
  (candidato/duplicado-antigo/sem-match/ole-pendente/erro-extracao),
  chave_db, match_tipo (numero/titulo).
- União determinística (número no nome + título normalizado exato):
  **695/1187** chaves sem letra cobertas (CC 151/151, VM 400/400);
  resíduo HCC 426, HL 64, COR 2. 906 arquivos `duplicado-antigo`
  (vale mtime mais novo).
- Decisão codex (sessão ETL): match em estágios — exato primeiro, depois
  similaridade (tokens/refrão) só como **candidato para revisão** (nunca
  autoaceito em `chave_db`); exige candidato único acima do limiar E com
  margem sobre o segundo.

## Resolução

Inventário fechado: 2282 slides catalogados, 695/1187 chaves sem letra com
fonte identificada, resíduo com caminho definido (match estagiado +
  revisão manual via CSV). Contagens por status no `corpus.csv`; OLE para o
  08 (636 dos 650 OLE já mapeados por nome, só o texto está pendente).

## Feito quando

- [x] Listagem `corpus.csv` — **todos os slides** (nada descartado;
      ignorados seguem com status), salva em `wayfinder/evidencia/`
- [x] União banco × corpus **por hino** (número + título exato; similaridade
      só como candidato p/ revisão, decisão codex): `com-letra` /
      `sem-letra-com-slide` / `sem-letra-sem-slide` / `slide-sem-hino`
      (ver coluna status do CSV). `GET /api/stats` **não basta**
      — só devolve agregados por coletânea
- [x] Resposta registrada como comentário de resolução (seção Resolução
      acima): 695/1187 cobertas; OLE (650, texto pendente) para o 08
