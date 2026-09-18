# Heurística de refrão e junção de estrofes

- Tipo: `wayfinder:grilling` (HITL — Mateus + Adão)
- Estado: aberto, não atribuído (bloqueado)
- Bloqueado por: [Formato esperado pelo banco](02-formato-esperado.md), [Padrões reais dos PPTs legados](03-padroes-ppts.md)

## Question

Como transformar slides legados (sem tags, com refrão repetido e estrofes
quebradas em 2+ slides) na letra canônica: qual bloco repetido vira refrão
indentado, quando repetir o refrão no banco vs uma única vez, e como
rejuntar estrofes divididas?

## Contexto

- Dúvida original do Mateus: "o refrão se repete depois da estrofe, mas não
  é obrigatório em muitas músicas; lendo o arquivo sem delimitação, como
  descobrir qual estrofe se repete para dentá-la diferente? Muitas estrofes
  são divididas entre mais de um slide."
- Restrição: refrão no banco = bloco com TODAS as linhas indentadas
  (espaço/tab); `titularLetra` preserva indentação; revisão é irreversível.
- Casos concretos da evidência: `A Sua imagem` (bloco de 8 linhas ×4 = refrão
  quase certo), `33 Nasceu Jesus` (bloco de 6 linhas ×2 no fim), `Salmo 46`
  (tudo se repete — é estrutura da música ou duplicação de slides?),
  `01.Antífona` (4 estrofes distintas, sem repetição — sem refrão?).

## Feito quando

- [ ] Grilling com Mateus/Adão trava: (a) limiar de repetição (ex. bloco
      idêntico ≥2× após normalizar caixa/espaço = refrão candidato),
      (b) refrão gravado 1× ou N× no banco, (c) regra de junção (slides
      curtos consecutivos não-repetidos = mesma estrofe?), (d) papel de LLM
      (só classificar candidato, nunca gerar letra) — ou "sem LLM"
- [ ] Resposta registra as 4 decisões + exemplos antes/depois com 2 hinos
      da evidência
