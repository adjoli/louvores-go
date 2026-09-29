# AGENTS.md — cmd/etl (tutorial do mutirão + dia-da-prod)

CLI do operador. Revisores nunca encostam aqui — eles vivem na planilha.

## Como a letra vira slide (fluxo da app)

O banco guarda só texto. PPTX nasce na hora do download: hino revisado
→ `GET /api/.../hinos/{numero}/slides` (um hino) ou `POST .../slides/lote`
(ZIP da coletânea). Nada é armazenado. Por isso o import só precisa
acertar o **texto** — e o `-validar` existe para provar, ainda na
importação, que cada texto gera um pacote íntegro (em vez de descobrir
no domingo de manhã).

## Uso do CLI

```
# 1. Overview (sempre primeiro, nunca escreve)
go run ./cmd/etl -csv revisao.csv -db copia.db

# 2. Conferir blocos de um hino (estrofe vs refrão)
go run ./cmd/etl -csv revisao.csv -db copia.db -check CC/36

# 3. Gravar (exige -yes; cria copia.db.bak-* antes)
go run ./cmd/etl -csv revisao.csv -db copia.db -dry-run=false -yes

# 4. Gravar + validar pacotes (recomendado no dia-da-prod)
go run ./cmd/etl -csv revisao.csv -db copia.db -dry-run=false -yes -validar

# Opcionais: -batch 100 (lotes transacionais), -force (sobrescreve letra
# existente — NUNCA em prod), -init (criar banco novo),
# -template outro.pptx (padrão: data/templates/default.pptx)
```

Sem flags abre menu interativo (`1` importar, `2` só validar).
Trava de segurança: recusa `data/hinos.db`, URL remota e path vazio.

## Performance (medido 09/2026)

- Dry-run de 2282 linhas: <1s. Import + `-validar` de 50 hinos: <1s.
- Dia-da-prod (~800, teto 2.2k): minutos no pior caso. Lotes de 100
  por padrão (`-batch`); se o Turso engasgar, baixe para 25.

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
4. Grave com `-dry-run=false -yes` (passo 3/4 acima).
   Nunca `--force` em prod. Nunca `data/hinos.db`, nunca Turso direto.
5. Com `-validar`: cada alterado gera PPTX íntegro ou bloqueia com a
   chave. Abra uma amostra no PowerPoint. `go test ./...` verde.
6. Suba a cópia pra prod (fora do CLI, com o Adão junto).
7. Re-rodar é seguro: quem já tem letra vira pulado (retoma sozinho).

## Regras que não mudam

- ETL toca **só `letra`** (nunca título/créditos/revisão).
- Refrão: repetido N×, toda linha com 2 espaços (igual CC 7 no banco).
- Caixa alta não importa (máquina aplica Title Case); indentação sim.
- Dúvida: deixa `PENDENTE` e pergunta no grupo. Não chuta.
