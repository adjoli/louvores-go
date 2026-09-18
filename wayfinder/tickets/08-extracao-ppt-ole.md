# Extração dos .ppt binários (OLE)

- Tipo: `wayfinder:research` (AFK)
- Estado: aberto, não atribuído (bloqueado)
- Bloqueado por: [Padrões reais dos PPTs legados](03-padroes-ppts.md), [Inventário do corpus legado](01-inventario-corpus.md)

## Question

Como extrair texto dos `.ppt` binários OLE com fidelidade: LibreOffice
headless (`soffice --headless --convert-to pptx`), biblioteca `olefile` (ou
equivalente), ou triagem manual?

## Contexto

- Dois arquivos confirmados como OLE (`Composite Document File V2`, cabeçalho
  `D0 CF 11 E0`): `Ele é meu e teu Senhor.ppt`, `Porque Ele vive.ppt`.
- O [Inventário do corpus legado](01-inventario-corpus.md) diz quantos `.ppt`
  existem no total — se forem só esses 2, a triagem manual pode vencer; se
  forem dezenas, precisa de conversor automático.
- Exigência: extração determinística (a tentativa anterior com IA gerou letra
  mal formatada e não confiável). O conversor precisa preservar ordem dos
  slides e quebras de linha.

## Feito quando

- [ ] Subagente testou o(s) conversor(es) nos 2 `.ppt` da evidência e
      comparou o texto extraído slide a slide com o esperado
- [ ] Resposta registra: ferramenta escolhida + comando exato + fidelidade
      observada (o que se perde no caminho) + recomendação manual-vs-auto
      conforme o volume do inventário
