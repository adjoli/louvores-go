# Formato intermediário de revisão

- Tipo: `wayfinder:grilling` (HITL — Mateus + Jônatas + Gabriel + louvor)
- Estado: aberto, não atribuído (bloqueado)
- Bloqueado por: [Formato esperado pelo banco](02-formato-esperado.md)

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
  valida o cantado; atenção a direitos autorais (repo precisaria ser privado
  se versionar letras) e à busca por palavra-chave (motivo do banco).

## Feito quando

- [ ] Grilling trava: colunas/campos do artefato (código, número, título,
      letra já em Title Case + refrão indentado, créditos, flag revisado),
      local (pasta local? git privado? Drive?), e critério de "pronto para
      importar" (quem dá o OK por hino)
- [ ] Resposta registra o formato escolhido + modelo de 1 hino revisado
