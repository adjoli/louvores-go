# Como revisar cada hino na planilha (Gabriel e Jônatas)

Vocês receberam a planilha com todos os slides. Filtre `status = PENDENTE`
e compare cada letra com o **hinário físico**. O que a máquina já fez:
tirou marcações de slide (`--- slide ---`), contadores (`1/8`), rodapés
(`CC n°230`, `IBRECEM`) e a capa de título. O que **só vocês** resolvem:

## Regras (formato final que o sistema aceita)

1. **Uma estrofe por bloco**: blocos separados por **uma linha em branco**.
   Sem linha em branco no meio da estrofe.
2. **Refrão se repete** depois de cada estrofe (igual se canta) e **toda
   linha do refrão começa com 2 espaços**. É assim que o sistema sabe que
   é refrão — sem os espaços vira estrofe comum.
3. **Apague**: numeração de estrofe (`4.` no início), título repetido no
   meio da letra, versículos, créditos e qualquer resto de rodapé.
4. **Ordem**: mantenha a ordem em que se canta. Confira cada palavra com
   o hinário e corrija erros de digitação **na própria célula da letra**.
5. **Caixa alta/baixa**: não se preocupem — a máquina ajusta depois.
6. Quando terminar a linha: mude `status` de `PENDENTE` para **`OK`**.
   `IMPORTADO` e `IGNORAR` não se mexe. Se souber hinário+número de uma
   linha sem vínculo, preencha as colunas.

## Exemplo (CC 230, Deus Chamando)

Errado (como veio da extração):

```
EI-LO A CONVIDAR-ME!
ELE QUER SALVAR-ME!
E, COM PERSISTÊNCIA, DEUS
ME CHAMA SEMPRE;
```

Certo (refrão repetido após cada estrofe, 2 espaços em toda linha):

```
DEUS SEMPRE INSISTE EM
ME CHAMAR,
...

  EI-LO A CONVIDAR-ME!
  ELE QUER SALVAR-ME!
  E, COM PERSISTÊNCIA, DEUS
  ME CHAMA SEMPRE;
  COM TERNURA CHAMA,
  COM AMOR ME CHAMA,
  O SENHOR INSISTE SEMPRE EM
  ME CHAMAR.
```

Meta: lotes de 15–20 por pessoa/semana. Dúvida: deixa `PENDENTE` e avisa
no grupo.
