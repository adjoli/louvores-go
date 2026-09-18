# Padrões reais dos PPTs legados

- Tipo: `wayfinder:research` (AFK)
- Estado: aberto, não atribuído (fronteira — pode ser pego agora)
- Bloqueado por: nada

## Question

O que os 10 PPTs de `wayfinder/evidencia/` realmente contêm quando extraídos
como texto: layouts de título, rodapés (`1/4`, `Hino VM nº 121`, `IBRECEM`,
`Copyright`), slides de capa/versículo, e como estrofes/refrões aparecem
(divididos, repetidos, ALL CAPS)?

## Contexto

- Amostras já copiadas: `01.Antífona`, `03. A TI A GLÓRIA`, `121 É Natal de
  Cristo`, `310 Meu tempo a sós com Deus`, `33 Nasceu Jesus`,
  `47 Senhor me Ensina`, `Musica A Sua imagem`, `Salmo 46` (`.pptx`) +
  `Ele é meu e teu Senhor`, `Porque Ele vive` (`.ppt` OLE — não é zip).
- Achados preliminares (extração `archive/zip` + `a:t`): `01.Antífona` tem
  5 slides (título `Antífona/CC n°1` + 4 partes com rodapé `N/4`, tudo CAPS);
  `A Sua imagem` repete o mesmo bloco de 8 linhas em 4 slides (refrão
  candidato) intercalado com estrofes de 4 linhas + rodapé `IBRECEM`;
  `Salmo 46` repete estrofe+refrão duas vezes; `121 É Natal` embute
  `É NATAL DE CRISTO` + `Hino VM nº 121` no corpo de todo slide; `.ppt`
  antigos são OLE (`D0 CF 11 E0`) e exigem outro extrator.
- Adão: estrofes longas são quebradas em 2 slides na projeção; IA anterior
  alucinou letra — extração deve ser determinística.

## Feito quando

- [ ] Subagente extraiu texto slide-a-slide das 8 `.pptx` (ex. `python3` com
      `zipfile` + `a:p`/`a:t`) e documentou por arquivo: nº slides, título,
      rodapés/ruído, blocos repetidos, caixa alta/baixa
- [ ] Resposta registra tabela + decisão: o que é ruído descartável
      (contadores, cabeçalhos repetidos, `Clique para adicionar texto`,
      copyright, versículos) e quais 2 `.ppt` ficam para o ticket do OLE
