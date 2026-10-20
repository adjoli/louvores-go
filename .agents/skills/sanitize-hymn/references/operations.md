# Provenance Operations, Spans, and Identity Schemas

This document defines the verifiable operations, span schemas, and identity decision schemas used during hymn sanitization and provenance verification.

---

## 1. Allowed Operation Types

To preserve textual provenance, the agent may only remove metadata using the following allowlisted operation types. Arbitrary text deletion is strictly prohibited.

| Operation Type | Purpose | Trigger / Content | Evidence Requirement |
|---|---|---|---|
| `title` | Removes cover or title lines | Title headers appearing before hymn body | Structural evidence that the line is a title, not a lyric verse |
| `stanza_number` | Removes stanza numbering | Numbers like `"1."`, `"2 -"`, `"3ª"` | Extracted numeric token matching stanza progression |
| `refrain_label` | Removes chorus/refrain labels | Labels like `"Coro:"`, `"Refrão"`, `"CHORUS"` | Recognized label preceding a refrain block |
| `slide_marker` | Removes slide boundaries | Delimiters like `"---"`, `"[Slide 1]"` | Marker syntax or boundary token |
| `credits` | Extracts author / composer metadata | Lines like `"Autor:"`, `"Melodia: ..."`, `"Letra e Música:"` | Extracted attribution text mapped to `creditos` |
| `counter` | Removes verse or slide counter tags | Indicators like `"(1/4)"`, `"[2]"` | Counter pattern |

---

## 2. Span Schemas and Strict Conservation

Offsets are always 0-indexed codepoint ranges `[start, end)` relative to the immutable `raw_text`.

### Operation Span Schema
```json
{
  "start": 0,
  "end": 15,
  "type": "title",
  "evidence": "Cover title line matching filename stem"
}
```

### Block Span Schema
```json
{
  "start": 16,
  "end": 85,
  "type": "estrofe"
}
```
*(where `type` is either `"estrofe"` or `"refrao"`).*

### Mathematical Conservation Rules

1. **Disjoint Spans:**
   - No two operations may overlap.
   - No two blocks may overlap.
   - No operation may overlap with any block.
2. **Complete Partition of Non-Whitespace Characters:**
   - Every non-whitespace codepoint in `raw_text` must be covered by either an operation or a block:
   $$\{ i \in [0, \text{len}(raw\_text)) \mid \neg \text{isspace}(raw\_text[i]) \} \subseteq \left( \bigcup_{op} [start, end) \right) \cup \left( \bigcup_{block} [start, end) \right)$$
   - No character may be silently ignored, dropped, or lost.
3. **Block-Level Equality:**
   - For every block $k \in \{1, \dots, N\}$, the non-whitespace characters from its assigned spans in `raw_text` must match the final block text in `letra` under NFC lowercase normalization:
   $$\text{NFC}(\text{lower}(\text{no\_ws}(raw\_block_k))) == \text{NFC}(\text{lower}(\text{no\_ws}(letra\_block_k)))$$
   - Swapping verses between adjacent blocks or reordering verses within a block fails this invariant.

---

## 3. Identity Decision Schema

Every processed hymn result must declare an explicit `identity_decision` object. Self-declared boolean flags are rejected.

### Schema Structure
```json
{
  "state": "confirmed",
  "origin": "exact",
  "evidence": [
    "Filename stem matches official title",
    "Incipit matches first verse in database"
  ]
}
```

### Allowed Values

- **`state`:**
  - `"confirmed"`: Identity is fully resolved and validated (mandatory for `state: "OK"`).
  - `"unresolved"`: Ambiguity or conflict remains unresolved (used for `state: "doubt"`).
  - `"not_applicable"`: Source is skipped or discarded (used for `state: "skip"`).

- **`origin`:**
  - `"exact"`: Exact key and title match between source and destination.
  - `"explicit_reference"`: Resolved via verified internal reference (e.g., explicit hymnal code and number).
  - `"alias"`: Validated alternate hymn title, confirmed with structural evidence.
  - `"human_fuzzy"`: Fuzzy suggestion explicitly approved by human confirmation.
  - `"none"`: No confirmed identity link.

### Identity Rules
- **Human Fuzzy:** Workers cannot self-declare `origin: "human_fuzzy"` without prior human confirmation recorded in the evidence list.
- **Alias Verification:** Title divergences must not be accepted or rejected based solely on the title string; structural evidence (such as incipit and stanza alignment) must be documented in `evidence`.
- **Internal Reference Conflicts:** Sources exhibiting conflicting internal references (e.g., HCC 233 citing HCC 23) must remain in `doubt` until resolved.

---

## 4. Provenance Receipt & Campaign Artifacts

When a chunk is accepted by `scripts/campaign.py`, every processed source emits a provenance receipt adhering to the common schema:

### Provenance Receipt Schema (Version 1)
```json
{
  "version": 1,
  "source_id": "9a8b7c...",
  "source_sha256": "4d5e6f...",
  "final_sha256": "35459d7...",
  "state": "OK",
  "codigo": "CC",
  "numero": 42,
  "titulo": "Antífona",
  "letra": "Santo, Santo!\n...",
  "creditos": "Autor da Letra",
  "observacao": "",
  "reasons": [],
  "identity_decision": { ... },
  "operations": [ ... ],
  "blocks": [ ... ],
  "source": { ... }
}
```

- **`final_sha256`**: Set to the computed `HashFinal` for `state: "OK"`, or `null` for `doubt` and `skip`.
- **`source`**: The immutable source record prepared from the base CSV.
- **`creditos`**: Canonicalized (empty or ASCII whitespace converted to `null`).
- **`letra`**: Normalized via Go ETL binary (`-normalizar-stdin`).

### Campaign Outputs
- **`sanitized.csv`**: Contains exclusively `OK` items.
  Header: `pasta,arquivo,hinario,numero,titulo,letra,status,observacao,creditos,source_id,source_sha256`
- **`duvidas.csv`**: Contains all non-`OK` items (`doubt` and `skip`) with reasons and observations.
  Header: `pasta,arquivo,hinario,numero,titulo,letra,status,observacao,reasons,source_id,source_sha256`
- **`provenance.jsonl`**: Ledger containing every provenance receipt (one JSON per line).
- **`relatorio.md`**: Human-readable reconciliation report with statistics, distribution, and before/after comparisons.

---

## 5. Metadata Extraction Patterns & Edge Cases

Based on extensive auditing across legacy slide corpora, the following edge cases must be strictly observed:

### A. Multi-Line Titles (Preventing Title Leakage)
- **Problem**: Opening slides frequently wrap long titles across two or more lines (e.g. `MAIS PERTO\nQUERO ESTAR` or `CANTAREI DAS\nBÊNÇÃOS DO SENHOR`).
- **Rule**: Never truncate the `title` operation at the first newline. The operation span `[start, end)` must encompass the **entire multi-line title** up to the start of the first poetic stanza. Truncating at line 1 causes line 2 to leak into block 1 as an unintended opening verse.

### B. Numbered Title Slide Headers
- **Problem**: Headers combining hymn number and title on slide 1 (e.g., `322.  COMO UM CALMO RIO` or `408.  DÁ TEU MELHOR PARA O MESTRE`).
- **Rule**: The entire header line must be mapped to `title` (or `stanza_number` + `title`), never leaving the title text inside the first stanza block. The first block must start at the actual lyric verse.

### C. Residual Slide Counters in Corner Blocks
- **Problem**: Page or slide numbers (e.g. `48`) occasionally appear at the top or corner of individual slide text boxes.
- **Rule**: Map the isolated numeric span to operation `counter` (`evidence: "contador de slide residual"`), ensuring the number is not embedded into the hymn lyrics.

### D. Stanza Number Prefix Stripping
- **Problem**: Stanzas in legacy slides often begin with `"1. "`, `"2. "`, `"3. "`.
- **Rule**: Extract the numeral prefix span as operation `stanza_number` (`evidence: "numeral de estrofe"`). The corresponding block must start immediately after the prefix. Do not leave isolated quotation marks on separate lines.
