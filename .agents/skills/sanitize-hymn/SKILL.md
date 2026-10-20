---
name: sanitize-hymn
description: "Higieniza slides legados e letras de hinos para o formato canônico do banco com indentação exata de refrão (4 espaços), Title Case, separação de créditos e rastreabilidade de proveniência. Use sempre que o usuário pedir para higienizar hinos, processar slides legados ou preparar a importação de letras."
---

# Sanitize Hymn

Pipeline autônomo, rastreável e offline para higienizar letras brutas de hinos e apresentações legadas, convertendo-as no formato canônico da plataforma louvores-go sem risco de corrupção ou perda de dados.

## Instrução Fundamental: Autonomia Total

Quando invocado para higienizar hinos ou preparar o auto-import, **NÃO peça ao usuário para rodar comandos de setup, preparar arquivos ou executar etapas manualmente.**
Você (o agente) deve executar todas as etapas de ponta a ponta de forma autônoma:
1. Configurar o ambiente temporário e compilar o ETL;
2. Preparar a campanha (ou retomar se já iniciada);
3. Executar o ciclo de lotes (chunks) até que todos estejam concluídos;
4. Consolidar os artefatos com `assemble`;
5. Gerar o manifesto e os scripts SQL (`import.sql`, `rollback.sql`, `verify.sql`) via `cmd/etl`;
6. Apresentar o relatório final consolidado para o usuário.

---

## Princípios e Regras Centrais

1. **Idioma de Resposta**: Responda sempre no idioma utilizado pelo usuário na conversa (ex.: Português).
2. **Regras Canônicas Embutidas**:
   - [format.md](references/format.md) — Regras de formatação do banco: estrofes com 0 espaços no início, refrões com **exatamente 4 espaços** (`    `), blocos separados por linha em branco (`\n\n`), no máximo 64 caracteres por linha e 10 linhas por bloco.
   - [operations.md](references/operations.md) — Tipos permitidos de operações estruturais descartadas (`title`, `stanza_number`, `refrain_label`, `slide_marker`, `credits`, `counter`) e esquemas de decisão.
3. **Isolamento de Contexto**:
   - Nunca tente carregar todas as letras de uma vez no contexto.
   - Processe um lote (`chunk`) de até 10 hinos por vez via `scripts/campaign.py`.
4. **Sem Alucinações de Letra**:
   - Ajuste espaçamentos, pontuações estruturais e indentação de refrão.
   - **Desmembramento de Linhas Longas Aglutinadas**: Quando versos estiverem agrupados em linha única por pontuação (`.`, `,`, `;`, `!`, `?` ou seguidos de maiúscula), particione-os em linhas $\le 64$ caracteres sem excluir ou alterar nenhum caractere não-whitespace, preservando a igualdade `nfc_lower_no_ws`.
   - **Nunca** invente palavras, não altere a ordem dos versos, não suprima estrofes e não invente sobrenomes de autores.
   - Em caso de dúvida ou ambiguidade estrutural, classifique a fonte como `doubt` com a razão explicada. Fontes em dúvida vão para `duvidas.csv` para conferência humana e não travam o processo.
5. **Formatação Universal para Dúvidas**:
   - Mesmo para itens classificados como `doubt` (hinos sem chave oficial, pendências de autor, etc.), formate **sempre** a proposta de `letra` no padrão canônico (Title Case, versos $\le 64$ caracteres, blocos separados e 4 espaços no refrão) para que a planilha de conferência já saia pronta para uso humano.
6. **Extração Rigorosa de Metadados de Slides Legados**:
   - **Títulos Multi-Linha**: Em slides de abertura com títulos longos quebrados em várias linhas (ex.: `CANTAREI DAS\nBÊNÇÃOS DO SENHOR`), estenda a operação `title` para cobrir **todas** as linhas do título até a primeira estrofe poética. Nunca corte na primeira quebra de linha para evitar vazamento do título para a estrofe 1.
   - **Cabeçalhos Numerados no Slide de Abertura**: Em slides como `322. COMO UM CALMO RIO`, cubra tanto o número quanto o título na operação `title` (ou `stanza_number` + `title`), sem deixar o título na letra.
   - **Contadores de Slides Residuais**: Números de página isolados no topo ou canto de caixas de texto (ex.: `48`) devem ser extraídos como operação `counter` (`evidence: "contador de slide residual"`).
   - **Numerais de Estrofe**: Prefixos como `"1. "`, `"2. "` devem ser mapeados para operação `stanza_number` (`evidence: "numeral de estrofe"`), removendo aspas soltas ou quebras artificiais.
   - **Indentações de Refrão (4 espaços)**: Todas as linhas de blocos de refrão devem ter estritamente 4 espaços (`    `) no início.
7. **Auditoria em Partições Paralelas (Double-Check Rigoroso)**:
   - Para volumes expressivos de hinos (ex.: centenas ou milhares de hinos), divida a carga em partições iguais (ex.: 8 partições de ~100 hinos) e despache subagentes de auditoria (`audit_partition.py --partition N`) para fazer o double-check estrito.
   - O auditor verifica: conformidade total do texto fonte (`verify_provenance`), conformidade de formatação (limite de 64 caracteres, refrões com 4 espaços), ausência de títulos vazados no primeiro verso, ausência de contadores ou numerais residuais no corpo do texto.
8. **Tratamento de Cânticos Avulsos / Corinhos sem Chave Oficial**:
   - Músicas sem número que não pertencem a hinários oficiais (CC, HCC, VM) e que não possuem correspondente exato na tabela `COR` são cânticos avulsos.
   - Devem ser mantidos em `doubt` com a letra **100% canonicamente formatada**, e exportados em artefatos dedicados de revisão humana (`$RUN/revisao_duvidas.md` e CSV com opções Manter/Descartar), aguardando decisão do operador.

---

## Roteiro de Execução Autônoma pelo Agente

### 1. Preparação Automática do Ambiente

Execute os comandos no terminal para garantir os pré-requisitos:
```bash
# Nova execução: diretório exclusivo; não sobrescrever snapshots anteriores.
WORKDIR=$(mktemp -d "${TMPDIR:-/tmp}/louvores-XXXXXX")
SNAPSHOT="$WORKDIR/snapshot.db"
ETL="$WORKDIR/etl"
RUN="$WORKDIR/campanha_louvores"

# Backup SQLite inclui commits presentes no WAL; cp de banco aberto não inclui.
python3 - "$SNAPSHOT" <<'PY'
import sqlite3
import sys
from pathlib import Path

source = sqlite3.connect(Path("data/hinos.db").resolve().as_uri() + "?mode=ro", uri=True)
try:
    target = sqlite3.connect(sys.argv[1])
    try:
        source.backup(target)
        target.execute("PRAGMA journal_mode=DELETE")
        if target.execute("PRAGMA quick_check").fetchone() != ("ok",):
            raise RuntimeError("snapshot inválido")
    finally:
        target.close()
finally:
    source.close()
PY

# Compilar o binário ETL do Go para normalização e validação rápida
go build -o "$ETL" ./cmd/etl

# Inicializar a campanha em diretório novo (informando a planilha fornecida pelo operador).
python3 .agents/skills/sanitize-hymn/scripts/campaign.py prepare \
  --csv "$CSV_PATH" \
  --snapshot "$SNAPSHOT" \
  --out "$RUN" \
  --etl "$ETL"
# Nota: se houver planilha complementar de atualização OLE, adicione `--ole-csv "$OLE_CSV_PATH"`.
```
Guarde os caminhos gerados e reutilize os mesmos valores nos comandos seguintes. Ao retomar uma campanha, use o snapshot, binário e diretório originais registrados no `run.json`: não gere outra cópia nem sobrescreva os artefatos. A fonte local não substitui o dump fresco exigido para o aceite de produção. Nenhum agente aplica o SQL em produção.

### 2. Ciclo de Processamento dos Lotes

Execute o loop até que todos os lotes estejam finalizados:

1. **Obter o próximo lote:**
   ```bash
   python3 .agents/skills/sanitize-hymn/scripts/campaign.py next --out "$RUN"
   ```
   - Se a saída for `{"version": 1, "complete": true}`, o ciclo terminou! Vá para a etapa 3.
   - Se contiver um `chunk_id` e lista de `sources`:

2. **Processar o lote:**
   Para cada fonte (`source`) dentro do lote:
   - Compare todas as fontes irmãs antes de selecionar a letra. `IGNORAR`/mtime são histórico, não prova de identidade ou qualidade. Referência interna incompatível implica `doubt`.
   - Se for um slide duplicado de menor qualidade, após comparar as alternativas:
     - Defina `state: "skip"` com a respectiva justificativa em `reasons`.
   - Se a letra for válida para o hino de destino:
     - Separe cabeçalho/título (operação `title`), números de estrofe (`stanza_number`), marcadores de coro (`refrain_label`) e créditos (`credits`).
     - Formate as estrofes sem recuo e todas as linhas do refrão com 4 espaços.
     - Documente a partição em `operations` e `blocks` (os spans de caracteres de `raw_text`).
     - Defina `state: "OK"`.
   - Se houver conflito insolúvel, letra ilegível ou dúvida:
     - Defina `state: "doubt"` com a justificativa em `reasons`.

3. **Submeter o resultado do lote:**
   Grave o resultado JSON em `$RUN/chunk_result.json` e execute:
   ```bash
   python3 .agents/skills/sanitize-hymn/scripts/campaign.py accept \
     --out "$RUN" \
     --chunk "$RUN/chunk_result.json"
   ```
   - Se o validador aceitar, repita o passo 1 para o próximo lote.
   - Se o validador apontar alguma violação, corrija o JSON e tente novamente. Em caso de dificuldade de alinhamento em algum item, marque aquele item como `"doubt"`.

### 3. Consolidação dos Resultados

Assim que `next` retornar `complete: true`:
```bash
python3 .agents/skills/sanitize-hymn/scripts/campaign.py assemble --out "$RUN"
```
Isso gera:
- `$RUN/sanitized.csv`: todos os hinos aprovados e higienizados.
- `$RUN/duvidas.csv`: os hinos que exigem conferência humana.
- `$RUN/provenance.jsonl`: recibos e hashes de rastreabilidade (não assinaturas de aprovação humana).
- `$RUN/relatorio.md`: estatísticas completas.

### 4. Auditoria Paralela em 8 Partições (Double-Check Estrito de Zero Defeitos)

Para garantir máxima qualidade antes da homologação final, particione os hinos processados e execute o double-check concorrente:
1. Divida o lote de hinos auditados em 8 partições iguais (`$RUN/audit/partition_1.json` a `partition_8.json`).
2. Despache 8 subagentes concorrentes executando:
   ```bash
   python3 .agents/skills/sanitize-hymn/scripts/audit_partition.py --run-dir "$RUN" --db "$SNAPSHOT" --partition <N>
   ```
3. O script valida automaticamente:
   - Preservação estrita do texto fonte (`nfc_lower_no_ws`);
   - Indentação de 4 espaços em todos os refrões;
   - Linhas $\le 64$ caracteres e blocos $\le 10$ linhas;
   - Ausência de títulos vazados no primeiro verso poético;
   - Ausência de numerais de estrofe (`1. `, `2. `) ou contadores residuais de slides (`48`).
4. Corrija imediatamente eventuais inconformidades e re-execute a auditoria até obter **0 defeitos em todas as 8 partições**.
5. Re-execute `campaign.py assemble --out "$RUN"` para atualizar os arquivos consolidados.

### 5. Documentação de Revisão Manual para Dúvidas e Cânticos Avulsos

Gere os artefatos de revisão para os hinos em dúvida sem chave oficial (corinhos avulsos):
```bash
python3 .agents/skills/sanitize-hymn/scripts/generate_review_doc.py \
  --csv "$RUN/duvidas.csv" \
  --md "$RUN/revisao_duvidas.md" \
  --out-csv "$RUN/duvidas_decisao.csv"
```
Isso fornece um documento interativo em Markdown com checkboxes `[ ] Manter / [ ] Descartar` e uma planilha CSV para o usuário decidir o destino de cada item.

### 6. Geração Automática do Manifesto e SQL

Gere imediatamente os artefatos de importação da plataforma:
```bash
# 1. Congelar o manifesto imutável
go run ./cmd/etl \
  -csv "$RUN/sanitized.csv" \
  -provenance "$RUN/provenance.jsonl" \
  -db "$SNAPSHOT" \
  -batch-id "lote-$(date +%Y%m%d)" \
  -manifest-out "$RUN/manifesto.json"

# 2. Validar slides em sandbox OOXML e exportar SQL puro
go run ./cmd/etl \
  -manifest "$RUN/manifesto.json" \
  -csv "$RUN/sanitized.csv" \
  -provenance "$RUN/provenance.jsonl" \
  -db "$SNAPSHOT" \
  -check all \
  -validar \
  -sql-out "$RUN/sql/import.sql"
```

### 7. Resposta ao Usuário

Ao término, informe ao usuário:
- Total de hinos prontos para importação em `sanitized.csv`;
- Total de dúvidas separadas para revisão em `duvidas.csv`;
- Confirmação do double-check de auditoria (0 defeitos em todas as 8 partições);
- Confirmação de 100% de integridade nos slides PPTX validados via sandbox OOXML;
- Caminho concreto do script SQL gerado: `$RUN/sql/import.sql`;
- Localização dos artefatos de revisão manual das dúvidas (`$RUN/revisao_duvidas.md`).
