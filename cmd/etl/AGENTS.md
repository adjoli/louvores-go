# AGENTS.md — cmd/etl (tutorial do mutirão + dia-da-prod)

CLI do operador. Revisores nunca encostam aqui — eles vivem na planilha.

## Mutirão (quando chegarem slides novos)

1. Jogue os `pptx`/`ppt`/`pps` numa pasta (ex. `corpus/novos/`).
2. Extraia o texto: `soffice --headless --convert-to pptx` pros OLE
   (`ppt`/`pps` antigos) + `zipfile` + `a:p`/`a:t` pros OOXML.
   Lixo conhecido: `--- slide ---`, `N/M`, `CC n°`, `IBRECEM`,
   `Clique para adicionar texto`, capa de título.
3. Monte a planilha com colunas
   `pasta,arquivo,hinario,numero,titulo,letra,status,observacao`
   (`status`: `IMPORTADO`/`PENDENTE`/`IGNORAR`; mesma chave = vale o
   arquivo mais novo). Suba no Sheets e mande
   `wayfinder/evidencia/instrucoes-revisao.md` no grupo.
4. Revisores corrigem a `letra` na célula e marcam `OK` + `REVISADO POR`.
   Pronto = `OK` com nome. Sem validação extra.

## Dia-da-prod (import oficial)

1. Backup: baixe o `hinos.db` atual (Turso free só guarda 24h).
2. Baixe a sheet como CSV.
3. Dry-run contra uma **cópia** do banco. Tem que dar 0 erros.
   Confira novos vs modificados, global e por coletânea.
4. Grave: `go run ./cmd/etl -csv revisao.csv -db copia.db -dry-run=false -yes`.
   Nunca `--force` em prod. Nunca `data/hinos.db`, nunca Turso direto.
5. Valide: `... -validar` gera o PPTX de cada alterado e confere o
   pacote (falha bloqueia com a chave). Abra uma amostra no PowerPoint.
   `go test ./...` verde.
6. Suba a cópia pra prod (fora do CLI, com o Adão junto).
7. Re-rodar é seguro: quem já tem letra vira pulado (retoma sozinho).

## Regras que não mudam

- ETL toca **só `letra`** (nunca título/créditos/revisão).
- Refrão: repetido N×, toda linha com 2 espaços (igual CC 7 no banco).
- Caixa alta não importa (máquina aplica Title Case); indentação sim.
- Dúvida: deixa `PENDENTE` e pergunta no grupo. Não chuta.
