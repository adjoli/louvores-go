# Inventário do corpus legado

- Tipo: `wayfinder:task` (HITL — precisa de acesso ao Drive/NOVA IDENTIDADE)
- Estado: aberto, não atribuído (fronteira — pode ser pego agora)
- Bloqueado por: nada

## Question

Qual é o universo real a importar: quantos arquivos PPT/PPTX existem no
Drive (pasta NOVA IDENTIDADE, por coletânea CC/HL/VM/HCC/COR), quais são
duplicatas, e quantos correspondem a hinos já com letra / sem letra / com
erro de extração no banco de produção?

## Contexto

- Hoje só temos 10 amostras em `wayfinder/evidencia/` (8× `.pptx` + 2× `.ppt`
  binário OLE: `Ele é meu e teu Senhor.ppt`, `Porque Ele vive.ppt`).
- Adão (chat 18/09): sem letra = 2 situações — erro na extração OU hino não
  cantado (sem slides nas pastas). Faltam ~300, sobretudo HL/HCC/corinhos.
- Sem esse número não dá para dimensionar mutirão nem lote.

## Feito quando

- [ ] Listagem `corpus.csv` (arquivo, coletânea, número, título, bytes, `ppt|pptx`)
      cobrindo o Drive inteiro, salva em `wayfinder/evidencia/`
- [ ] Cruzamento **por hino** (chave de negócio coletânea+número, ex. `CC/42`):
      cada linha do `corpus.csv` classificada contra o banco em
      `com-letra` / `sem-letra` / `erro-extracao` / `nao-cantado`
      (sem slides nas pastas). `GET /api/stats` **não basta** — só devolve
      agregados por coletânea, sem dizer quais hinos faltam
- [ ] Resposta registrada como comentário de resolução: contagens por
      coletânea e por status + onde está o `corpus.csv` + quantos `.ppt` OLE
      precisam de conversão
