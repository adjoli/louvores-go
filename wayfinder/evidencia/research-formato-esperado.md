# Research — Formato esperado pelo banco (subagente AFK, 18/09/2026)

Fonte: `wayfinder/tickets/02-formato-esperado.md`. Nada editado, só leitura.

**1. DDL** — `internal/database/db.go:34-52` (idempotente, `IF NOT EXISTS`):
- `coletanea(id PK AUTOINCREMENT, codigo TEXT NOT NULL, titulo TEXT NOT NULL)`
- `hino(id PK AUTOINCREMENT, coletanea_id INTEGER NOT NULL REFERENCES coletanea(id), numeracao INTEGER [nullable], titulo TEXT NOT NULL, letra TEXT [nullable], creditos TEXT [nullable], revisado INTEGER NOT NULL DEFAULT 0)`
- `INDEX idx_hino_coletanea_num ON hino(coletanea_id, numeracao)`; FK via `?_pragma=foreign_keys(1)` (`db.go:72`).

**2. Modelo** — `internal/models/models.go:25-33`: `Numeracao/Letra/Creditos` ponteiros (`nil` = ausente; `Letra nil` = "hino sem letra"); JSON `snake_case` sem `omitempty` (nil → `null`).

**3. Exemplo canônico** (1 estrofe + 1 refrão):
```
Primeira Linha Da Estrofe
Segunda Linha Da Estrofe

  Primeira Linha Do Refrão
  Segunda Linha Do Refrão
```
Refrão = TODAS as linhas com espaço/tab (`lyrics_parser.go:78-85`); indentação removida só no refrão (`:105-111`); estrofe mantém original.

**4. Normalização**:
- CRLF: `\r\n`→`\n` primeiro, depois `\r`→`\n` — `lyrics_parser.go:117-120` e `hino_service.go:360-361`.
- Blocos: split `\n\s*\n` (`lyrics_parser.go:17`), `Trim(texto,"\n")` antes (`:43`), linhas whitespace-only descartadas (`:90-98`).
- Title Case no save (`AtualizarHino:128`, `titularLetra:356-374`): por linha, só se `trimmed` todo-lower OU todo-upper; caixa mista preservada; indentação preservada; brancas puladas.
- Slides: `textosSlides (:312-325)` — não-`COR`: subtítulo `"Nome - numero"`, topo `"NUMCOD - Titulo"`; `COR`: subtítulo vazio, título original. Arquivo (:270-276): `{CODIGO}-{NUM:03d}-{TITULO_UPPER_UNDERLINES}.pptx`. Parte `Numero` 1-based (`lyrics_parser.go:46-68`); rodapé `N/total` (`slide_parts.go:18-20`).

**5. O que impede geração**:
- `geraSlidesHino (:246-251)`: `!Revisado` → `ErrHinoNotReviewed`; `Numeracao==nil` → erro. Inexistente → `ErrColetaneaNotFound/ErrHinoNotFound`.
- Lote (:207-220): pula em `Pulados` se `!Revisado || Letra==nil || Numeracao==nil`; erro individual logado, sem abortar ZIP.
- Revisão irreversível (`:133-135`): `Revisado==true` ignora `false` posterior.

**6. Stats** — `stats_service.go:37-49`: `percentual = ComLetra/Total*100` (`0.0` se `Total==0`); `percentual_revisados = Revisados/ComLetra*100` (`0.0` se `ComLetra==0`).
