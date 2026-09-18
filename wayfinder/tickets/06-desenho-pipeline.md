# Desenho da pipeline ETL

- Tipo: `wayfinder:prototype` (HITL — artefato barato para reagir)
- Estado: aberto, não atribuído (bloqueado)
- Bloqueado por: [Inventário do corpus legado](01-inventario-corpus.md), [Heurística de refrão (detecção)](04-heuristica-refrao.md), [Junção, armazenamento e papel do LLM](09-juncao-armazenamento-llm.md), [Formato intermediário de revisão](05-formato-revisao.md), [Extração dos .ppt binários (OLE)](08-extracao-ppt-ole.md)

## Question

Qual é o desenho ponta-a-ponta (extrair → normalizar → revisar → importar):
linguagem/ferramentas, onde roda, e como cada etapa é reproduzível?

## Contexto

- Entrada: Drive/NOVA IDENTIDADE (`.pptx` + `.ppt` OLE) +
  [Inventário do corpus legado](01-inventario-corpus.md) (o volume por
  formato decide o tratamento dos `.ppt`: conversor automático vs triagem
  manual, ver [Extração dos .ppt binários](08-extracao-ppt-ole.md)).
- Normalização usa [Formato esperado pelo banco](02-formato-esperado.md) +
  detecção do [Heurística de refrão](04-heuristica-refrao.md) +
  reconstrução do [Junção, armazenamento e papel do LLM](09-juncao-armazenamento-llm.md);
  saída da revisão usa [Formato intermediário](05-formato-revisao.md).
- Restrições: código prod não importa `gooxml` (só validador em testes);
  `main.css`/`templ` irrelevantes; sem CGO (preferir `archive/zip` +
  `encoding/xml` em Go ou script Python com `zipfile`).

## Feito quando

- [ ] Protótipo (não a pipeline final): diagrama + esqueleto
      (`cmd/importar --dry-run` ou script + `Makefile` target) que extrai 2
      hinos da evidência até o formato de revisão
- [ ] Resposta linka o protótipo e trava: Go vs Python por etapa, comando de
      cada fase, onde roda (local vs CI), e como re-rodar sem duplicar
