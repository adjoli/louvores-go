"""Hymn source preparation, hashing, and candidate grouping.

Implements offline source preparation from CSV and SQLite snapshots.
Uses Python 3 stdlib only with zero third-party dependencies.
"""

from __future__ import annotations

import csv
import hashlib
import json
import re
import sqlite3
import unicodedata
from pathlib import Path
from typing import Any

# Exact stopwords defined by the specification
STOPWORDS: set[str] = {"a", "o", "as", "os", "de", "da", "do", "das", "dos", "e"}

# Regex for word tokens (excluding punctuation)
WORD_RE = re.compile(r"\w+", re.UNICODE)

# Regex to detect internal references (must require either explicit hymnal code or explicit keyword)
EXPLICIT_CODE_REF_RE = re.compile(
    r"\b(HCC|CC|COR|VM)\s*(\d{1,4})\b",
    re.IGNORECASE,
)
KEYWORD_REF_RE = re.compile(
    r"\b(?:veja|vejam|ver|v\.)\s*(?:(HCC|CC|COR|VM)\s*)?(\d{1,4})\b",
    re.IGNORECASE,
)


def compute_source_sha256(raw_text: str) -> str:
    """Computes SHA-256 of raw UTF-8 text."""
    return hashlib.sha256(raw_text.encode("utf-8")).hexdigest()


def compute_source_id(row_dict: dict[str, Any], has_creditos_col: bool = False) -> str:
    """Computes source_id as SHA256(UTF8(arquivo) || NUL || UTF8(canonical_row_json)).

    canonical_row_json includes the 8 original columns as strings:
    pasta, arquivo, hinario, numero, titulo, letra, status, observacao.
    Includes creditos only if the column exists in the CSV.
    """
    original_cols = [
        "pasta",
        "arquivo",
        "hinario",
        "numero",
        "titulo",
        "letra",
        "status",
        "observacao",
    ]
    canonical_dict: dict[str, str] = {}
    for col in original_cols:
        val = row_dict.get(col)
        if val is None:
            val = row_dict.get(f"{col} original", "")
        canonical_dict[col] = str(val if val is not None else "")
    if has_creditos_col or "creditos" in row_dict or "créditos" in row_dict:
        val = row_dict.get("creditos", row_dict.get("créditos", ""))
        canonical_dict["creditos"] = str(val if val is not None else "")

    canonical_json = json.dumps(
        canonical_dict,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    )
    arquivo = str(canonical_dict["arquivo"])
    payload = arquivo.encode("utf-8") + b"\x00" + canonical_json.encode("utf-8")
    return hashlib.sha256(payload).hexdigest()


def compute_final_sha256(
    codigo: str,
    numero: int,
    titulo: str,
    letra: str,
    creditos: str | None,
) -> str:
    """Computes final canonical SHA-256 matching Go's HashFinal:

    SHA256(UTF8(codigo) || NUL || ASCII(numero_decimal) || NUL || UTF8(titulo_destino) || NUL || UTF8(letra_final) || NUL || credito_tipado)
    where credito_tipado is byte 'N' if creditos is None, or byte 'S' + creditos UTF-8.
    """
    if numero <= 0:
        raise ValueError(f"invalid numero: {numero}")
    if "\x00" in codigo or "\x00" in titulo or "\x00" in letra:
        raise ValueError("field contains NUL byte")
    if creditos is not None and "\x00" in creditos:
        raise ValueError("creditos contains NUL byte")

    buf = bytearray()
    buf.extend(codigo.encode("utf-8"))
    buf.append(0)
    buf.extend(str(numero).encode("ascii"))
    buf.append(0)
    buf.extend(titulo.encode("utf-8"))
    buf.append(0)
    buf.extend(letra.encode("utf-8"))
    buf.append(0)
    if creditos is None:
        buf.append(ord("N"))
    else:
        buf.append(ord("S"))
        buf.extend(creditos.encode("utf-8"))

    return hashlib.sha256(buf).hexdigest()


def is_sem_conteudo(s: str | None) -> bool:
    """Checks whether string is None or only ASCII whitespace.

    ASCII whitespace: space, \t, \n, \r, \v, \f. NBSP (\\u00a0) is considered content.
    """
    if s is None:
        return True
    ascii_ws = {" ", "\t", "\n", "\r", "\v", "\f"}
    return all(c in ascii_ws for c in s)


def normalize_tokens(text: str) -> set[str]:
    """Tokenizes string using NFD lower without accents, punctuation as delimiter,

    splits camelCase/CamelCase, and removes the exact stopwords {a, o, as, os, de, da, do, das, dos, e}.
    """
    if not text:
        return set()

    # Split camelCase / CamelCase into separate words
    text = re.sub(r"([a-z])([A-Z])", r"\1 \2", text)
    # NFD decomposition
    decomposed = unicodedata.normalize("NFD", text)
    # Strip combining diacritical marks
    stripped = "".join(c for c in decomposed if unicodedata.category(c) != "Mn")
    lowered = stripped.lower()

    # Extract word tokens (punctuation serves as delimiter)
    tokens = WORD_RE.findall(lowered)
    return {tok for tok in tokens if tok and tok not in STOPWORDS}


def fuzzy_jaccard(tokens1: set[str], tokens2: set[str]) -> float:
    """Calculates Jaccard similarity between two sets of tokens."""
    if not tokens1 or not tokens2:
        return 0.0
    union = tokens1 | tokens2
    if not union:
        return 0.0
    intersection = tokens1 & tokens2
    return len(intersection) / len(union)


def _extract_title_candidate(filename: str, raw_text: str) -> str:
    """Extracts candidate title from filename stem or header lines (not row['titulo'])."""
    stem = Path(filename).stem
    # If stem has pattern like CODE-NUM-Title, strip prefix
    cleaned_stem = re.sub(r"^[A-Za-z]+[-_]\d+[-_]?", "", stem).strip()
    if cleaned_stem:
        return cleaned_stem

    # Otherwise check first non-empty line of raw_text before stanzas
    lines = [ln.strip() for ln in raw_text.splitlines() if ln.strip()]
    if lines:
        first = lines[0]
        # Avoid lines that are numeric or stanza counters
        if not re.match(r"^\d+[.\-]?$", first) and len(first) <= 60:
            return first

    return stem


def _find_internal_reference_conflicts(
    codigo: str | None,
    numero: int | None,
    raw_text: str,
    arquivo: str,
) -> list[str]:
    """Detects internal reference conflicts like HCC 233 containing 'HCC 23' or 'veja 23'."""
    if not codigo or not numero:
        return []

    conflicts: list[str] = []
    # Search first 300 characters of raw text and filename
    sample = (raw_text[:300] + " " + arquivo).upper()

    # 1. Matches with explicit hymnal code (e.g. "HCC 23")
    for match in EXPLICIT_CODE_REF_RE.finditer(sample):
        ref_code, ref_num_str = match.groups()
        try:
            ref_num = int(ref_num_str)
        except ValueError:
            continue

        if ref_code.upper() == codigo.upper() and ref_num != numero:
            conf = f"referência interna a {ref_code} {ref_num} diferente de {numero}"
            if conf not in conflicts:
                conflicts.append(conf)

    # 2. Matches with explicit reference keyword (e.g. "veja 23" or "veja HCC 23")
    for match in KEYWORD_REF_RE.finditer(sample):
        ref_code, ref_num_str = match.groups()
        try:
            ref_num = int(ref_num_str)
        except ValueError:
            continue

        if ref_code is None or ref_code.upper() == codigo.upper():
            if ref_num != numero:
                conf = f"referência interna a {ref_code or codigo} {ref_num} diferente de {numero}"
                if conf not in conflicts:
                    conflicts.append(conf)

    return conflicts


def validate_snapshot(snapshot_path: Path) -> Path:
    """Reject active WAL and orphaned WAL-format copies before immutable reads."""
    snapshot_path = snapshot_path.resolve(strict=True)
    wal = Path(str(snapshot_path) + "-wal")
    if (wal.exists() and wal.stat().st_size > 0) or Path(str(snapshot_path) + "-shm").exists():
        raise ValueError("snapshot must be consolidated: WAL/SHM present")
    with snapshot_path.open("rb") as f:
        header = f.read(20)
    if header[:16] != b"SQLite format 3\x00" or header[18:20] != b"\x01\x01":
        raise ValueError("snapshot must be SQLite journal_mode DELETE, not WAL")
    return snapshot_path


def prepare_sources(
    csv_path: Path,
    snapshot_path: Path,
    ole_csv_path: Path | None = None,
) -> list[dict[str, Any]]:
    """Reads snapshot and source CSV, grouping siblings and extracting provenance evidence.

    Initial state is 'candidate', 'doubt', or 'skip' (never 'OK').
    OLE CSV is read-only and supplements slide boundary evidence without overlay.
    """
    # 1. Connect to SQLite snapshot read-only
    snapshot_uri = validate_snapshot(snapshot_path).as_uri() + "?mode=ro&immutable=1"
    conn = sqlite3.connect(snapshot_uri, uri=True)
    cur = conn.cursor()

    cur.execute("SELECT id, codigo, titulo FROM coletanea")
    coletaneas: dict[int, dict[str, Any]] = {}
    col_by_code: dict[str, dict[str, Any]] = {}
    for col_id, cod, col_tit in cur.fetchall():
        coletaneas[col_id] = {"id": col_id, "codigo": cod, "titulo": col_tit}
        col_by_code[cod.upper()] = {"id": col_id, "codigo": cod, "titulo": col_tit}

    cur.execute("SELECT id, coletanea_id, numeracao, titulo, letra, creditos, revisado FROM hino")
    hino_by_key: dict[tuple[str, int], dict[str, Any]] = {}
    destinations_without_lyrics: list[dict[str, Any]] = []

    for h_id, c_id, num, tit, letra, creds, rev in cur.fetchall():
        col_info = coletaneas.get(c_id)
        if not col_info:
            continue
        cod = col_info["codigo"].upper()
        h_dict = {
            "id": h_id,
            "coletanea_id": c_id,
            "codigo": cod,
            "numeracao": num,
            "titulo": tit,
            "letra": letra,
            "creditos": creds,
            "revisado": bool(rev),
        }
        if num is not None:
            hino_by_key[(cod, num)] = h_dict

        # Destination without lyrics and not reviewed
        if not rev and is_sem_conteudo(letra) and num is not None:
            destinations_without_lyrics.append(h_dict)

    conn.close()

    # 2. Read optional OLE CSV complement
    ole_by_file: dict[str, dict[str, str]] = {}
    if ole_csv_path is not None and ole_csv_path.is_file():
        with ole_csv_path.open(mode="r", encoding="utf-8-sig") as f:
            reader = csv.DictReader(f)
            for row in reader:
                fname = row.get("arquivo", "").strip()
                if fname:
                    ole_by_file[fname] = dict(row)
                    stem = Path(fname).stem
                    ole_by_file[stem] = dict(row)

    # 3. Read base source CSV
    sources: list[dict[str, Any]] = []
    with csv_path.open(mode="r", encoding="utf-8-sig") as f:
        reader = csv.DictReader(f)
        header = [h.strip() for h in (reader.fieldnames or [])]
        has_creditos_col = any(h.lower() in {"creditos", "créditos"} for h in header)

        rows = list(reader)

    # Determine which destinations are already targeted by keyed rows in the CSV
    keyed_targets: set[tuple[str, int]] = set()
    for row in rows:
        cod_candidate = row.get("hinario", "").strip().upper()
        num_str = row.get("numero", "").strip()
        if cod_candidate and num_str.isdigit():
            num_val = int(num_str)
            if num_val > 0:
                keyed_targets.add((cod_candidate, num_val))

    # Eligible destinations for fuzzy suggestions (must NOT already have a source in CSV)
    fuzzy_eligible_dests = [
        d for d in destinations_without_lyrics
        if (d["codigo"], d["numeracao"]) not in keyed_targets
    ]
    # Precompute tokens for fuzzy eligible destinations
    dest_tokens_cache = [
        (d, normalize_tokens(d["titulo"])) for d in fuzzy_eligible_dests
    ]

    # 4. Process each row into source dict
    for row in rows:
        pasta = (row.get("pasta") if row.get("pasta") is not None else row.get("pasta original", "")).strip()
        arquivo = (row.get("arquivo") if row.get("arquivo") is not None else row.get("arquivo original", "")).strip()
        hinario = row.get("hinario", "").strip()
        numero_str = row.get("numero", "").strip()
        raw_text = row.get("letra", "")
        status_inicial = row.get("status", "").strip()
        observacao_inicial = row.get("observacao", "").strip()

        source_sha = compute_source_sha256(raw_text)
        src_id = compute_source_id(row, has_creditos_col=has_creditos_col)

        codigo: str | None = hinario.upper() if hinario else None
        numero: int | None = None
        if numero_str.isdigit():
            n = int(numero_str)
            if n > 0:
                numero = n

        if codigo and numero:
            group_id = f"{codigo}/{numero}"
            dest_hino = hino_by_key.get((codigo, numero))
            titulo_destino = dest_hino["titulo"] if dest_hino else None
            creditos_destino = dest_hino["creditos"] if dest_hino else None
        else:
            group_id = src_id
            titulo_destino = None
            creditos_destino = None

        # Extract title candidates
        cand_title = _extract_title_candidate(arquivo, raw_text)
        candidates = [cand_title]
        first_line = raw_text.splitlines()[0].strip() if raw_text.splitlines() else ""
        if first_line and first_line != cand_title:
            candidates.append(first_line)

        evidence: dict[str, Any] = {
            "filename_stem": Path(arquivo).stem,
            "candidates": candidates,
            "incipit": first_line,
        }

        # Check OLE boundaries
        ole_match = ole_by_file.get(arquivo) or ole_by_file.get(Path(arquivo).stem)
        if ole_match:
            evidence["ole"] = ole_match

        reasons: list[str] = []
        state = "candidate"

        # Check for empty raw text
        if is_sem_conteudo(raw_text):
            state = "doubt"
            reasons.append("raw_text vazio sem fonte")

        # Check for internal reference conflicts
        ref_conflicts = _find_internal_reference_conflicts(codigo, numero, raw_text, arquivo)
        if ref_conflicts:
            state = "doubt"
            reasons.extend(ref_conflicts)
            evidence["internal_references"] = ref_conflicts

        # Check for unkeyed source and fuzzy match
        if not codigo or not numero:
            state = "doubt"
            reasons.append("fonte sem chave definida")

            candidate_token_sets = [normalize_tokens(c) for c in candidates if c]
            if not candidate_token_sets and first_line:
                candidate_token_sets = [normalize_tokens(first_line)]

            suggestions: list[dict[str, Any]] = []

            scored: list[tuple[float, dict[str, Any]]] = []
            for d, d_tok in dest_tokens_cache:
                score = (
                    max(fuzzy_jaccard(toks, d_tok) for toks in candidate_token_sets)
                    if candidate_token_sets
                    else 0.0
                )
                if score > 0.0:
                    scored.append((score, d))

            scored.sort(key=lambda x: x[0], reverse=True)
            if scored:
                top_score, top_dest = scored[0]
                second_score = scored[1][0] if len(scored) > 1 else 0.0
                margin = top_score - second_score
                if top_score >= 0.75 and margin >= 0.10:
                    suggestions.append({
                        "codigo": top_dest["codigo"],
                        "numero": top_dest["numeracao"],
                        "titulo": top_dest["titulo"],
                        "jaccard": round(top_score, 3),
                        "margin": round(margin, 3),
                    })
                    reasons.append(
                        f"sugestão fuzzy: {top_dest['codigo']}/{top_dest['numeracao']} "
                        f"({top_dest['titulo']}) J={top_score:.2f} margem={margin:.2f}"
                    )

            if suggestions:
                evidence["fuzzy_suggestions"] = suggestions

        # Preserve status IGNORAR info
        if status_inicial.upper() == "IGNORAR" and observacao_inicial:
            reasons.append(f"status inicial IGNORAR: {observacao_inicial}")

        source_entry: dict[str, Any] = {
            "source_id": src_id,
            "source_sha256": source_sha,
            "group_id": group_id,
            "sibling_source_ids": [],  # filled below
            "pasta": pasta,
            "arquivo": arquivo,
            "status_inicial": status_inicial,
            "observacao_inicial": observacao_inicial,
            "codigo": codigo,
            "numero": numero,
            "titulo_destino": titulo_destino,
            "creditos_destino": creditos_destino,
            "titulo_fonte": cand_title,
            "raw_text": raw_text,
            "state": state,
            "reasons": reasons,
            "evidence": evidence,
        }
        sources.append(source_entry)

    # 5. Populate sibling_source_ids for each group
    group_map: dict[str, list[str]] = {}
    for s in sources:
        group_map.setdefault(s["group_id"], []).append(s["source_id"])

    for s in sources:
        s["sibling_source_ids"] = list(group_map[s["group_id"]])

    return sources
