# Extração dos .ppt binários (OLE)

- Tipo: `wayfinder:research` (AFK)
- Estado: fechado (resolvido + executado na sessão ETL, 09/2026)
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

## Resolução

Ferramenta: **LibreOffice headless** (`soffice --headless --convert-to pptx
--outdir`, batch de 50, chamada via lista de args) + extração
`zipfile`+`a:p`/`a:t` existente; `olefile` descartado como primário
(lixo de masters + segmentação por slide variante-dependente).
Fidelidade nas amostras: ordem, quebras, acentos e refrões preservados.
**Executado no corpus inteiro: 650/650 convertidos** (2 colisões de stem
— `Porque Ele vive`, `Ele é meu e teu Senhor` — reconvertidas
separadas), 649 com texto, 1 vazio legítimo (`000 - Modelo.pps`).
Convertidos crus em `corpus/pptx_convertidos/` (gitignored); texto em
`corpus/txt/`; `corpus/revisao-ole-update.csv` (chave `arquivo`) levado
ao Drive para XLOOKUP na planilha do Mateus. Validação por arquivo
(slides>0, não-vazio) obrigatória — `.pps` tem containers diferentes
conforme o arquivo.
