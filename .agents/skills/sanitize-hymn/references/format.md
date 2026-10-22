# Canonical Hymn Format Specification

This document defines the canonical format specification for sanitized hymn lyrics and credits in the Louvores project, distinguishing it from legacy database patterns observed in historical imports.

---

## 1. Canonical Format Specification

All imported hymns must adhere to the following strict canonical rules:

### Character Encoding & Line Endings
- **Encoding:** UTF-8 without Byte Order Mark (BOM).
- **Line Endings:** Single UNIX Line Feed (`\n`, `0x0A`). Carriage Returns (`\r\n` or `\r`) are forbidden.

### Block Structure
- **Block Separation:** Blocks are separated by exactly one blank line (`\n\n`).
- **Internal Spacing:** No blank lines are permitted within a block.
- **Block Types:** Every block is classified as either `estrofe` (stanza) or `refrao` (refrain/chorus).

### Indentation Rules
- **Stanzas (`estrofe`):**
  - Exactly **0 leading spaces** on all lines.
  - No tabs, non-breaking spaces, or indentation of any kind.
- **Refrains (`refrao`):**
  - Exactly **4 leading ASCII spaces** (`    `) on every line.
  - No tabs (`\t`), no 2-space or 8-space indentation.

### Conservative Seed Limits
- **Line Length:** Maximum **64 runes** (characters) per line.
  - For refrain lines, the 4-space prefix is excluded from the 64-character limit.
- **Block Length:** Maximum **10 lines** per block.
- **Overflow & Concatenated Line Reflow Heuristic:**
  - In raw imports or slide conversions, multiple verses are often concatenated onto a single line separated by punctuation (e.g. `.` , `,` , `;` , `!` , `?`) or punctuation followed by capital letters without whitespace (e.g. `criação.Ele`, `AMOR,LONGE`).
  - Reflowing such concatenated lines into multiple verse lines (each $\le 64$ characters) is permitted and encouraged **provided that** no non-whitespace characters are deleted or altered, preserving mathematical equality under `nfc_lower_no_ws`.
  - When lines exceed 64 characters without natural punctuation or structural boundaries, they cannot be arbitrarily cut and must be routed to human review (`doubt`).
- **Universal Canonical Formatting for Doubts (`doubt`):**
  - Even when an item remains in `doubt` (e.g. unkeyed corinho, ambiguous reference, or pending human confirmation), the agent **must** still format its proposed `letra` in canonical Title Case, with appropriate stanza/refrain indentation and line breaks. This ensures the output review artifacts (`$RUN/duvidas.csv` / `$RUN/revisao_duvidas.md`) are immediately readable and ready for one-click approval.

### Text Capitalization
- Capitalization follows Go's `services.TitularLetra`:
  - Capitalizes the first alphabetical character of each line/verse, preserving preceding quotes or brackets.
  - Preserves mixed-case words (e.g., proper nouns, acronyms).
  - Normalizes all-uppercase words.

### Credits Format
- **Delimiters:** Multiple author or composer credits are joined by `" | "`.
- **Literal Fidelity:** Names must be extracted verbatim without inventing surnames (e.g., `"Keith and Kristyn Getty"` must not be split into `"Keith Getty | Kristyn Getty"`).
- **Attribution Labels:** Preserves functional attributions where applicable (e.g., `"Melodia: Autor"` is retained and not discarded).
- **Null vs. Empty:** An absent credit proposal is explicitly `null`, distinct from an empty string.

---

## 2. Legacy Database Patterns vs. Canonical Standard

Historical database records exhibited various ad-hoc patterns that must **not** be propagated:

| Aspect | Legacy Pattern (Observed in DB) | Canonical Format (Required) |
|---|---|---|
| **Refrain Indentation** | Varied (tabs, 2 spaces, 8 spaces, or no indent) | Exactly 4 ASCII spaces (`    `) on all refrain lines |
| **Labels in Body** | `"CORO:"`, `"Refrão:"`, `"1ª Estrofe"`, `"1."` | Stripped via provenance operations; no labels in body |
| **Separators** | Mixed CRLF, duplicate blank lines | Pure LF (`\n`), exactly one blank line between blocks |
| **Casing** | Fully uppercase (`VIGOR OLHOU PARA A CRUZ`) | Canonical Title Case (`Vigor Olhou Para a Cruz`) |
| **Line Limits** | Long lines up to 100+ chars | Max 64 runes/line (excl. 4-space refrain indent) |
| **Block Limits** | Large blocks up to 20+ lines | Max 10 lines/block |
| **Credits** | Single string with mixed commas/semicolons | Separated by `" | "` with exact deduplication |

---

## 3. Exclusion of CC/2 Contamination

> [!CAUTION]
> Local development snapshots contained artificial test data in `CC/2` (e.g., `"dados de teste"`, credit `"Autor"`).
>
> **Do not treat `CC/2` as a template, rule, or valid reference pattern.** Any heuristics based on `CC/2`'s historical attributes are invalid.
