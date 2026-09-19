# Heurística de refrão (detecção)

- Tipo: `wayfinder:grilling` (HITL — Mateus + Adão)
- Estado: fechado (grilling sessão ETL, 09/2026)
- Bloqueado por: [Formato esperado pelo banco](02-formato-esperado.md), [Padrões reais dos PPTs legados](03-padroes-ppts.md)

## Question

Qual bloco repetido vira refrão: limiar de repetição e normalização de
comparação para eleger o refrão candidato a partir dos slides legados?

## Contexto

- Dúvida original do Mateus: "o refrão se repete depois da estrofe, mas não
  é obrigatório em muitas músicas; lendo o arquivo sem delimitação, como
  descobrir qual estrofe se repete para dentá-la diferente?"
- Restrição: refrão no banco = bloco com TODAS as linhas indentadas
  (espaço/tab); `titularLetra` preserva indentação.
- Casos concretos da evidência: `A Sua imagem` (bloco de 8 linhas ×4 = refrão
  quase certo), `33 Nasceu Jesus` (bloco de 6 linhas ×2 no fim), `Salmo 46`
  (tudo se repete — é estrutura da música ou duplicação de slides?),
  `01.Antífona` (4 estrofes distintas, sem repetição — sem refrão?),
  `121 É Natal` (rodapé `É NATAL DE CRISTO` concatenado — não é refrão).
- O que fazer COM o refrão eleito (rejuntar estrofes partidas, gravar 1× ou
  N×, papel de LLM) vive em [Junção, armazenamento e papel do LLM](09-juncao-armazenamento-llm.md), não aqui.

## Resolução (grilling, Mateus 09/2026 — versão simples, sem overthinking)

- Bloco idêntico **≥2×** (normalizado: caixa, espaços, pontuação;
  rodapés descartados) = refrão. Zero repetição = sem refrão.
- Empate = vence o **mais repetido**; persistindo, `titulo-ambiguo`
  para o humano (sem regra de "mais curto").
- A heurística **pré-marca** no `revisao-pack`; Gabriel/Jônatas
  **confirmam** os 2 espaços na planilha (camada máquina-sugere,
  humano-confirma).
