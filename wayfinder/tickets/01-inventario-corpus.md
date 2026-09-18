# Inventário do corpus legado

- Tipo: `wayfinder:task` (HITL — precisa de acesso ao Drive/NOVA IDENTIDADE)
- Estado: aberto, não atribuído (fronteira — pode ser pego agora)
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

## Feito quando

- [ ] Listagem `corpus.csv` — **só arquivos** (arquivo, coletânea, número,
      título, bytes, `ppt|pptx`) cobrindo o Drive inteiro, salva em
      `wayfinder/evidencia/`
- [ ] União banco × corpus **por hino** (chave de negócio coletânea+número,
      ex. `CC/42`), com acesso ao banco prod (`TURSO_DATABASE_URL` +
      `TURSO_AUTH_TOKEN`, ou dump passado por Adão/Mateus — declarar no
      início para a sessão não travar): para cada chave, `com-letra` /
      `sem-letra-com-slide` (precisa extração) / `sem-letra-sem-slide`
      (candidato a não-cantado) / `slide-sem-hino` (arquivo sem linha no
      banco). `nao-cantado` e `erro-extracao` **não** são linhas do
      `corpus.csv`: o primeiro é chave no banco sem slide; o segundo é
      resultado da extração, apurado depois. `GET /api/stats` **não basta**
      — só devolve agregados por coletânea
- [ ] Resposta registrada como comentário de resolução: contagens por
      coletânea e por status + onde está o `corpus.csv` + quantos `.ppt` OLE
      precisam de conversão
