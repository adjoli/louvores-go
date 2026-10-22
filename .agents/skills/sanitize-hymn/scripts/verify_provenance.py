"""Verifiable provenance gate for hymn sanitization.

Validates that worker results strictly conserve textual provenance, follow canonical
formatting, map disjoint partitions of raw_text via operations and blocks, and enforce
exact block-level NFC(lower(no_ws)) conservation.
"""

from __future__ import annotations

import re
import unicodedata
from typing import Any

# Allowed operation types (strict allowlist)
ALLOWED_OP_TYPES: set[str] = {
    "title",
    "stanza_number",
    "refrain_label",
    "slide_marker",
    "credits",
    "counter",
}

# Regex to split blocks by blank lines
BLOCK_SPLIT_RE = re.compile(r"\n\s*\n")


def no_ws(text: str) -> str:
    """Removes all whitespace characters from string."""
    return "".join(c for c in text if not c.isspace())


def nfc_lower_no_ws(text: str) -> str:
    """Computes NFC normalized lowercase string without whitespace."""
    return unicodedata.normalize("NFC", no_ws(text).lower())


def verify(source: dict[str, Any], result: dict[str, Any]) -> list[str]:
    """Verifies that result adheres to the shared contract and provably conserves source text.

    Returns a list of violation strings. Empty list indicates full validity.
    """
    violations: list[str] = []

    # 1. Validate required keys
    required_keys = {
        "source_id",
        "source_sha256",
        "state",
        "codigo",
        "numero",
        "titulo",
        "letra",
        "creditos",
        "observacao",
        "reasons",
        "identity_decision",
        "operations",
        "blocks",
    }
    missing_keys = required_keys - set(result.keys())
    if missing_keys:
        violations.append(f"missing keys in result: {sorted(missing_keys)}")
        return violations

    # Check NUL bytes in string fields
    for field_name in ["codigo", "titulo", "letra", "creditos", "observacao"]:
        val = result.get(field_name)
        if isinstance(val, str) and "\x00" in val:
            violations.append(f"field '{field_name}' contains illegal NUL byte")

    # 2. Check source hash / ID match
    if result.get("source_id") != source.get("source_id"):
        violations.append(
            f"source_id mismatch: expected '{source.get('source_id')}', got '{result.get('source_id')}'"
        )
    if result.get("source_sha256") != source.get("source_sha256"):
        violations.append(
            f"source_sha256 mismatch: expected '{source.get('source_sha256')}', got '{result.get('source_sha256')}'"
        )

    # 3. Check state
    state = result.get("state")
    if state not in {"OK", "doubt", "skip"}:
        violations.append(f"invalid state: '{state}', expected 'OK', 'doubt', or 'skip'")
        return violations

    if not isinstance(result.get("reasons"), list):
        violations.append("'reasons' must be a list of strings")

    # 4. Handle non-OK states (doubt or skip)
    if state in {"doubt", "skip"}:
        if not result.get("reasons"):
            violations.append(f"state '{state}' requires at least one explanatory reason")
        id_dec = result.get("identity_decision")
        if not isinstance(id_dec, dict):
            violations.append("'identity_decision' must be a dict")
        else:
            if "state" not in id_dec or "origin" not in id_dec:
                violations.append("identity_decision missing 'state' or 'origin'")
        return violations

    # 5. State == "OK" validation
    raw_text = source.get("raw_text", "")
    if not raw_text or not raw_text.strip():
        violations.append("source raw_text is empty; state cannot be 'OK'")
        return violations

    # Check required fields for OK
    if not result.get("codigo") or not isinstance(result.get("codigo"), str):
        violations.append("state 'OK' requires non-empty string 'codigo'")
    if not isinstance(result.get("numero"), int) or result.get("numero", 0) <= 0:
        violations.append("state 'OK' requires positive integer 'numero'")
    if not result.get("titulo") or not isinstance(result.get("titulo"), str):
        violations.append("state 'OK' requires non-empty string 'titulo'")
    letra = result.get("letra")
    if not letra or not isinstance(letra, str):
        violations.append("state 'OK' requires non-empty string 'letra'")
        return violations

    # Validate identity_decision schema
    id_dec = result.get("identity_decision")
    if not isinstance(id_dec, dict):
        violations.append("'identity_decision' must be a dictionary")
    else:
        id_state = id_dec.get("state")
        id_origin = id_dec.get("origin")
        id_ev = id_dec.get("evidence")

        if id_state != "confirmed":
            violations.append(f"identity_decision state must be 'confirmed' for OK, got '{id_state}'")
        if id_origin not in {"exact", "explicit_reference", "alias", "human_fuzzy"}:
            violations.append(f"invalid identity_decision origin for OK: '{id_origin}'")
        if not isinstance(id_ev, list) or len(id_ev) == 0:
            violations.append("identity_decision must include non-empty list of evidence")

    # Validate operations schema
    operations = result.get("operations")
    if not isinstance(operations, list):
        violations.append("'operations' must be a list of dicts")
        operations = []

    op_spans: list[tuple[int, int, str]] = []
    for idx, op in enumerate(operations):
        if not isinstance(op, dict):
            violations.append(f"operation {idx} is not a dictionary")
            continue
        start = op.get("start")
        end = op.get("end")
        op_type = op.get("type")
        evidence = op.get("evidence")

        if not isinstance(start, int) or not isinstance(end, int):
            violations.append(f"operation {idx} start/end must be integers")
            continue
        if start < 0 or end > len(raw_text) or start >= end:
            violations.append(f"operation {idx} invalid span [{start}, {end}) for raw_text length {len(raw_text)}")
            continue
        if op_type not in ALLOWED_OP_TYPES:
            violations.append(f"operation {idx} unauthorized type '{op_type}'")
        if not evidence:
            violations.append(f"operation {idx} missing required evidence")

        # Monotonic ordering check for operations
        if op_spans and start < op_spans[-1][1]:
            violations.append(
                f"operations spans must be in monotonically increasing order: "
                f"operation {idx+1} starts at {start} before operation {idx} ends at {op_spans[-1][1]}"
            )

        # Semantic content validation of operation span in raw_text
        span_text = raw_text[start:end].strip()
        if not span_text:
            violations.append(f"operation {idx} ({op_type}) covers only whitespace")
        elif op_type == "stanza_number":
            if not re.search(r"\b\d{1,3}\b", span_text) and not re.search(r"\b[IVXLCDM]+\b", span_text):
                violations.append(f"operation {idx} (stanza_number) text '{span_text}' does not contain numeric stanza numbering syntax")
        elif op_type == "refrain_label":
            if not re.search(r"\b(coro|refr[aã]o|estribilho|chorus|refrain)\b", span_text, re.IGNORECASE):
                violations.append(f"operation {idx} (refrain_label) text '{span_text}' does not contain chorus/refrain label syntax")
        elif op_type == "slide_marker":
            if not re.search(r"[-=]{2,}|\[\s*slide|\bslide\b|\bsld\b|\bcapa\b", span_text, re.IGNORECASE):
                violations.append(f"operation {idx} (slide_marker) text '{span_text}' does not contain slide marker syntax")
        elif op_type == "counter":
            if not re.search(r"\(\s*\d+\s*/\s*\d+\s*\)|\[\s*\d+\s*\]|\b\d+\s*/\s*\d+\b|^\s*\d+\s*$", span_text):
                violations.append(f"operation {idx} (counter) text '{span_text}' does not contain counter syntax")
        elif op_type == "credits":
            credit_keywords = r"\b(letra|m[uú]sica|autor|melodia|texto|arranjo|vers[aã]o|comp\b|copyright|hin[aá]rio|salmo|trad|adapt|getty|newton|watts)\b"
            prop_c = str(result.get("creditos") or "")
            matching_words = [w for w in re.findall(r"\w+", span_text) if len(w) > 2 and w.lower() in prop_c.lower()]
            if not re.search(credit_keywords, span_text, re.IGNORECASE) and not matching_words:
                violations.append(f"operation {idx} (credits) text '{span_text}' does not contain recognized credits syntax")
        elif op_type == "title":
            ev_str = str(evidence).lower()
            is_cover = any(kw in ev_str for kw in ["capa", "cover", "header", "titulo", "título", "abertura", "slide inicial", "stem", "linha"])
            if not is_cover:
                violations.append(f"operation {idx} (title) text '{span_text}' requires explicit structural evidence")

        op_spans.append((start, end, str(op_type)))

    # Validate blocks schema
    blocks = result.get("blocks")
    if not isinstance(blocks, list) or len(blocks) == 0:
        violations.append("'blocks' must be a non-empty list of dicts")
        blocks = []

    block_spans: list[tuple[int, int, str]] = []
    for idx, blk in enumerate(blocks):
        if not isinstance(blk, dict):
            violations.append(f"block {idx} is not a dictionary")
            continue
        b_type = blk.get("type")
        if b_type not in {"estrofe", "refrao"}:
            violations.append(f"block {idx} invalid type '{b_type}', must be 'estrofe' or 'refrao'")

        b_start = blk.get("start")
        b_end = blk.get("end")
        if not isinstance(b_start, int) or not isinstance(b_end, int):
            violations.append(f"block {idx} start/end must be integers")
            continue
        if b_start < 0 or b_end > len(raw_text) or b_start >= b_end:
            violations.append(f"block {idx} invalid span [{b_start}, {b_end})")
            continue

        # Monotonic ordering check for blocks
        if block_spans and b_start < block_spans[-1][1]:
            violations.append(
                f"blocks spans must be in monotonically increasing order: "
                f"block {idx+1} starts at {b_start} before block {idx} ends at {block_spans[-1][1]}"
            )

        block_spans.append((b_start, b_end, str(b_type)))

    # 6. Check disjointness and coverage of operations and blocks
    # Build mask of covered characters in raw_text
    char_covered: list[int] = [0] * len(raw_text)

    for start, end, op_t in op_spans:
        for i in range(start, end):
            if char_covered[i] != 0:
                violations.append(f"overlapping span at index {i} in operation [{start}, {end})")
                break
            char_covered[i] = 1

    for start, end, b_t in block_spans:
        for i in range(start, end):
            if char_covered[i] != 0:
                violations.append(f"overlapping span at index {i} in block [{start}, {end})")
                break
            char_covered[i] = 2

    # Check combined partition ordering (operations and blocks must not interleave out of order)
    all_combined = sorted(
        [(s, e, "op", t) for s, e, t in op_spans] + [(s, e, "block", t) for s, e, t in block_spans],
        key=lambda x: x[0],
    )
    for i in range(1, len(all_combined)):
        if all_combined[i][0] < all_combined[i - 1][1]:
            violations.append(
                f"partition spans overlap: span {all_combined[i]} starts before previous span {all_combined[i-1]} ends"
            )

    # Verify that all non-whitespace characters in raw_text are accounted for
    for i, ch in enumerate(raw_text):
        if not ch.isspace() and char_covered[i] == 0:
            snippet = raw_text[max(0, i - 10): min(len(raw_text), i + 15)].replace("\n", " ")
            violations.append(
                f"raw_text non-whitespace character '{ch}' at index {i} ('...{snippet}...') "
                f"not accounted for in operations or blocks (unaccounted / partição incompleta)"
            )
            break

    # 7. Validate letra structure against blocks
    if "\r" in letra:
        violations.append("letra contains CRLF or CR carriage return line breaks; only LF is allowed")

    letra_clean = letra.replace("\r\n", "\n").replace("\r", "\n").strip()
    letra_blocks = [b for b in BLOCK_SPLIT_RE.split(letra_clean) if b.strip()]

    if len(letra_blocks) != len(block_spans):
        violations.append(
            f"blocks count mismatch: letra has {len(letra_blocks)} blocks, result['blocks'] has {len(block_spans)}"
        )
        return violations

    # 8. Block-by-block formatting and exact textual conservation
    for k, (b_start, b_end, b_type) in enumerate(block_spans):
        cur_block_text = letra_blocks[k]
        lines = [ln for ln in cur_block_text.split("\n") if ln.strip()]

        # 10 lines limit per block
        if len(lines) > 10:
            violations.append(f"block {k+1} has {len(lines)} lines, exceeding the 10 lines limit")

        # Indentation and line length limits
        for line_idx, line in enumerate(lines):
            if b_type == "refrao":
                if (
                    not line.startswith("    ")
                    or line.startswith("     ")
                    or line.startswith("\t")
                    or (len(line) > 4 and line[4] in {" ", "\t"})
                ):
                    violations.append(
                        f"block {k+1} (refrao) line {line_idx+1} does not have exactly 4 spaces indentation"
                    )
                content = line[4:]
            else:
                if line.startswith(" ") or line.startswith("\t"):
                    violations.append(
                        f"block {k+1} (estrofe) line {line_idx+1} has unexpected leading whitespace"
                    )
                content = line

            # 64 runes limit (excluding refrain 4-space indent)
            if len(content) > 64:
                violations.append(
                    f"block {k+1} line {line_idx+1} length {len(content)} runes exceeds 64 runes limit"
                )

        # Exact block-level textual conservation: NFC(lower(no_ws))
        # Extract raw block characters (only those assigned to this block)
        raw_chars: list[str] = []
        for i in range(b_start, b_end):
            if char_covered[i] == 2 and not raw_text[i].isspace():
                raw_chars.append(raw_text[i])

        raw_block_norm = unicodedata.normalize("NFC", "".join(raw_chars).lower())
        letra_block_norm = nfc_lower_no_ws(cur_block_text)

        if raw_block_norm != letra_block_norm:
            violations.append(
                f"block {k+1} content mismatch: text was altered, words deleted, or verses reordered "
                f"(raw='{raw_block_norm[:30]}...', letra='{letra_block_norm[:30]}...')"
            )

    # 9. Credits validation
    prop_cred = result.get("creditos")
    dest_cred = source.get("creditos_destino")
    raw_lower = raw_text.lower()
    prop_cred_str = str(prop_cred or "").strip()

    if prop_cred is not None and isinstance(prop_cred, str) and prop_cred.strip():
        # If destination already had non-empty credits and they conflict
        if dest_cred is not None and isinstance(dest_cred, str) and dest_cred.strip():
            if prop_cred.strip().lower() != dest_cred.strip().lower():
                violations.append(
                    f"credito proposto '{prop_cred}' conflita com crédito destino vivo '{dest_cred}'"
                )

    # Melodia: Autor check - if raw_text contains melody credit, it must not be discarded from creditos
    if re.search(r"\bmelodia\s*:\s*[^\n\r]+", raw_text, re.IGNORECASE):
        if not prop_cred or "melodia" not in prop_cred_str.lower():
            violations.append("melody credit ('Melodia: ...') found in source was discarded from result['creditos']")

    # Keith and Kristyn Getty check - must not invent surnames
    if "keith and kristyn getty" in raw_lower or "keith e kristyn getty" in raw_lower:
        if "keith getty" in prop_cred_str.lower():
            violations.append("invented surname detected in creditos for 'Keith and Kristyn Getty'")

    # Authorship vs Biblical reference check
    if re.search(r"\b(salmo|sl\.)\s*\d+", raw_lower) and re.search(r"\bm[uú]sica\s*:\s*([^\n\r|]+)", raw_text, re.IGNORECASE):
        comp_match = re.search(r"\bm[uú]sica\s*:\s*([^\n\r|]+)", raw_text, re.IGNORECASE)
        comp_name = comp_match.group(1).strip() if comp_match else ""
        if comp_name and comp_name.lower() not in prop_cred_str.lower():
            violations.append(f"composer '{comp_name}' was omitted from creditos in favor of biblical reference")

    # Partial / ambiguous credits check (e.g. "Trad. e Adapt." or single initials without resolution)
    if re.search(r"(?:trad\.\s*e\s*adapt\.|autor\s+desconhecido\s*\/\s*[a-z]|[a-z]\.\s*[a-z]\.)", raw_lower):
        ev_texts = " ".join(result.get("identity_decision", {}).get("evidence", [])).lower()
        if "ambig" not in ev_texts and "resolv" not in ev_texts and "parcial" not in ev_texts:
            violations.append("partial or ambiguous credits in source require explicit resolution or state: 'doubt'")

    return violations
