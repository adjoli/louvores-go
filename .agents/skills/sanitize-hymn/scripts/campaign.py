"""Resumable campaign manager for hymn sanitization.

Implements the offline campaign lifecycle: prepare, next, accept, and assemble.
Uses Python 3 stdlib only with zero third-party dependencies.
Advisory flock ensures single-writer mutual exclusion and automatic OS release on crash.
"""

from __future__ import annotations

import argparse
import csv
import fcntl
import hashlib
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

SKILL_ROOT = Path(__file__).resolve().parent.parent
if str(SKILL_ROOT) not in sys.path:
    sys.path.insert(0, str(SKILL_ROOT))

from scripts.prepare import (
    compute_final_sha256,
    is_sem_conteudo,
    prepare_sources,
    validate_snapshot,
)
from scripts.verify_provenance import verify


class CampaignLock:
    """Advisory flock context manager for exclusive single-writer access."""

    def __init__(self, run_dir: Path) -> None:
        self.run_dir = run_dir.resolve()
        self.lock_file_path = self.run_dir / ".lock"
        self._fd: Any = None

    def __enter__(self) -> CampaignLock:
        self.run_dir.mkdir(parents=True, exist_ok=True)
        self._fd = open(self.lock_file_path, "a+")
        try:
            fcntl.flock(self._fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except (BlockingIOError, OSError) as e:
            self._fd.close()
            self._fd = None
            raise RuntimeError(f"campaign directory is locked by another process: {e}")
        return self

    def __exit__(self, exc_type: Any, exc_val: Any, exc_tb: Any) -> None:
        if self._fd is not None:
            try:
                fcntl.flock(self._fd, fcntl.LOCK_UN)
            except OSError:
                pass
            self._fd.close()
            self._fd = None


def file_sha256(path: Path) -> str:
    """Computes SHA-256 of a file."""
    h = hashlib.sha256()
    with path.open("rb") as f:
        while chunk := f.read(65536):
            h.update(chunk)
    return h.hexdigest()


def compute_scripts_sha256(scripts_dir: Path) -> str:
    """Computes combined SHA-256 of all python scripts in scripts_dir."""
    h = hashlib.sha256()
    if scripts_dir.is_dir():
        for p in sorted(scripts_dir.glob("*.py")):
            if p.name.startswith("."):
                continue
            h.update(p.name.encode("utf-8"))
            h.update(b"\x00")
            h.update(p.read_bytes())
            h.update(b"\x00")
    return h.hexdigest()


def compute_skill_sha256(skill_dir: Path) -> str:
    """Computes combined SHA-256 of all skill documentation and reference files."""
    h = hashlib.sha256()
    paths: list[Path] = []
    skill_md = skill_dir / "SKILL.md"
    if skill_md.is_file():
        paths.append(skill_md)
    for subdir in ["references", "evals"]:
        d = skill_dir / subdir
        if d.is_dir():
            for p in d.glob("*"):
                if p.is_file() and not p.name.startswith("."):
                    paths.append(p)
    paths.sort(key=lambda p: p.relative_to(skill_dir).as_posix())
    for p in paths:
        rel = p.relative_to(skill_dir).as_posix()
        h.update(rel.encode("utf-8"))
        h.update(b"\x00")
        h.update(p.read_bytes())
        h.update(b"\x00")
    return h.hexdigest()


def validate_integrity(run_data: dict[str, Any], skill_dir: Path) -> None:
    """Verifies that no input files, snapshot, ETL binary, or scripts have been tampered with."""
    csv_p = Path(run_data["csv_path"])
    if not csv_p.is_file() or file_sha256(csv_p) != run_data["input_sha256"]:
        raise RuntimeError(f"CSV file tampered, modified, or missing: {csv_p}")

    snap_p = Path(run_data["snapshot_path"])
    if not snap_p.is_file() or file_sha256(snap_p) != run_data["snapshot_sha256"]:
        raise RuntimeError(f"snapshot file tampered, modified, or missing: {snap_p}")
    validate_snapshot(snap_p)

    etl_p = Path(run_data["etl_path"])
    if not etl_p.is_file() or file_sha256(etl_p) != run_data["etl_sha256"]:
        raise RuntimeError(f"ETL binary tampered, modified, or missing: {etl_p}")

    if run_data.get("ole_csv_path"):
        ole_p = Path(run_data["ole_csv_path"])
        if not ole_p.is_file() or file_sha256(ole_p) != run_data["ole_sha256"]:
            raise RuntimeError(f"OLE CSV file tampered, modified, or missing: {ole_p}")

    scripts_h = compute_scripts_sha256(skill_dir / "scripts")
    if scripts_h != run_data.get("scripts_sha256"):
        raise RuntimeError("skill scripts have been modified since campaign prepare")

    skill_h = compute_skill_sha256(skill_dir)
    if skill_h != run_data.get("skill_sha256"):
        raise RuntimeError("skill files have been modified since campaign prepare")


def write_json_atomic(path: Path, data: Any) -> None:
    """Writes JSON data to path atomically using a temporary file and os.replace."""
    tmp_fd, tmp_path_str = tempfile.mkstemp(dir=path.parent, prefix=f"{path.stem}.", suffix=".tmp")
    tmp_path = Path(tmp_path_str)
    with os.fdopen(tmp_fd, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
    os.replace(tmp_path, path)


def reconcile_state(run_dir: Path, run_data: dict[str, Any]) -> bool:
    """Reconciles in-memory run_data with completed partial files on disk."""
    dirty = False
    for chunk in run_data["chunks"]:
        chunk_id = chunk["id"]
        partial_p = run_dir / f"partial-{chunk_id}.json"
        if partial_p.is_file():
            try:
                partial_data = json.loads(partial_p.read_text(encoding="utf-8"))
                expected_sha = partial_data.get("incoming_sha256")
                if chunk["state"] != "complete" or chunk.get("result_sha256") != expected_sha:
                    chunk["state"] = "complete"
                    chunk["result_sha256"] = expected_sha
                    dirty = True
            except (json.JSONDecodeError, OSError):
                # Unfinished or broken write, partial file will be re-accepted
                pass

    if dirty:
        write_json_atomic(run_dir / "run.json", run_data)
    return dirty


def partition_groups(
    ordered_groups: list[str],
    group_sources: dict[str, list[dict[str, Any]]],
    chunk_size: int = 10,
) -> list[list[str]]:
    """Partitions groups into batches.

    Groups with >10 sources get a dedicated chunk.
    Regular groups accumulate up to chunk_size groups.
    Siblings are never split across chunks.
    """
    chunks: list[list[str]] = []
    current_chunk: list[str] = []

    for gid in ordered_groups:
        num_sources = len(group_sources[gid])
        if num_sources > 10:
            if current_chunk:
                chunks.append(current_chunk)
                current_chunk = []
            chunks.append([gid])
        else:
            current_chunk.append(gid)
            if len(current_chunk) >= chunk_size:
                chunks.append(current_chunk)
                current_chunk = []

    if current_chunk:
        chunks.append(current_chunk)

    return chunks


def prepare(
    csv_path: Path,
    snapshot_path: Path,
    run_dir: Path,
    etl_path: Path,
    ole_csv_path: Path | None = None,
    chunk_size: int = 10,
    skill_dir: Path = SKILL_ROOT,
) -> dict[str, Any]:
    """Initializes a new sanitization campaign run."""
    run_dir = run_dir.resolve()
    if run_dir.exists():
        raise FileExistsError(f"run directory {run_dir} already exists")

    csv_path = csv_path.resolve()
    snapshot_path = snapshot_path.resolve()
    etl_path = etl_path.resolve()
    ole_csv_path = ole_csv_path.resolve() if ole_csv_path is not None else None

    if not csv_path.is_file():
        raise FileNotFoundError(f"CSV file not found: {csv_path}")
    if not snapshot_path.is_file():
        raise FileNotFoundError(f"snapshot file not found: {snapshot_path}")
    if not etl_path.is_file():
        raise FileNotFoundError(f"ETL binary not found: {etl_path}")
    if ole_csv_path is not None and not ole_csv_path.is_file():
        raise FileNotFoundError(f"OLE CSV file not found: {ole_csv_path}")

    run_dir.mkdir(parents=True)

    with CampaignLock(run_dir):
        # 1. Compute hashes
        input_sha256 = file_sha256(csv_path)
        snapshot_sha256 = file_sha256(snapshot_path)
        etl_sha256 = file_sha256(etl_path)
        ole_sha256 = file_sha256(ole_csv_path) if ole_csv_path is not None else None
        scripts_sha256 = compute_scripts_sha256(skill_dir / "scripts")
        skill_sha256 = compute_skill_sha256(skill_dir)

        # 2. Extract and prepare sources
        sources = prepare_sources(csv_path, snapshot_path, ole_csv_path)

        # Persist prepared sources inside run_dir
        write_json_atomic(run_dir / "sources.json", sources)

        # 3. Group and chunk
        ordered_groups: list[str] = []
        group_sources: dict[str, list[dict[str, Any]]] = {}
        for s in sources:
            gid = s["group_id"]
            if gid not in group_sources:
                ordered_groups.append(gid)
                group_sources[gid] = []
            group_sources[gid].append(s)

        group_chunks = partition_groups(ordered_groups, group_sources, chunk_size=chunk_size)

        chunks_meta: list[dict[str, Any]] = []
        for idx, g_list in enumerate(group_chunks):
            s_ids: list[str] = []
            for gid in g_list:
                for s in group_sources[gid]:
                    s_ids.append(s["source_id"])
            chunks_meta.append({
                "id": idx,
                "group_ids": g_list,
                "source_ids": s_ids,
                "state": "pending",
                "result_sha256": None,
            })

        run_data: dict[str, Any] = {
            "version": 1,
            "csv_path": str(csv_path),
            "snapshot_path": str(snapshot_path),
            "etl_path": str(etl_path),
            "ole_csv_path": str(ole_csv_path) if ole_csv_path is not None else None,
            "input_sha256": input_sha256,
            "snapshot_sha256": snapshot_sha256,
            "ole_sha256": ole_sha256,
            "skill_sha256": skill_sha256,
            "scripts_sha256": scripts_sha256,
            "etl_sha256": etl_sha256,
            "chunk_size": chunk_size,
            "chunks": chunks_meta,
        }

        write_json_atomic(run_dir / "run.json", run_data)
        return run_data


def get_next(run_dir: Path, skill_dir: Path = SKILL_ROOT) -> dict[str, Any]:
    """Retrieves the next pending chunk to be processed by a worker."""
    run_dir = run_dir.resolve()
    with CampaignLock(run_dir):
        run_json_p = run_dir / "run.json"
        if not run_json_p.is_file():
            raise FileNotFoundError(f"run.json not found in {run_dir}")

        run_data = json.loads(run_json_p.read_text(encoding="utf-8"))
        validate_integrity(run_data, skill_dir)
        reconcile_state(run_dir, run_data)

        # Find first pending chunk
        pending_chunk: dict[str, Any] | None = None
        for chunk in run_data["chunks"]:
            if chunk["state"] == "pending":
                pending_chunk = chunk
                break

        if pending_chunk is None:
            return {"version": 1, "complete": True}

        # Load sources for this chunk
        sources_data = json.loads((run_dir / "sources.json").read_text(encoding="utf-8"))
        sources_by_id = {s["source_id"]: s for s in sources_data}

        chunk_sources = [sources_by_id[sid] for sid in pending_chunk["source_ids"]]

        return {
            "version": 1,
            "chunk_id": pending_chunk["id"],
            "groups": pending_chunk["group_ids"],
            "sources": chunk_sources,
        }


def accept(
    run_dir: Path,
    chunk_file: Path,
    skill_dir: Path = SKILL_ROOT,
) -> dict[str, Any]:
    """Validates worker results for a chunk, normalizes lyrics via Go ETL, and records provenance receipts."""
    run_dir = run_dir.resolve()
    chunk_file = chunk_file.resolve()

    if not chunk_file.is_file():
        raise FileNotFoundError(f"chunk output file not found: {chunk_file}")

    with CampaignLock(run_dir):
        run_json_p = run_dir / "run.json"
        if not run_json_p.is_file():
            raise FileNotFoundError(f"run.json not found in {run_dir}")

        run_data = json.loads(run_json_p.read_text(encoding="utf-8"))
        validate_integrity(run_data, skill_dir)
        reconcile_state(run_dir, run_data)

        chunk_data = json.loads(chunk_file.read_text(encoding="utf-8"))
        chunk_id = chunk_data.get("chunk_id")
        results = chunk_data.get("results")

        if chunk_id is None or not isinstance(results, list):
            raise ValueError("chunk file missing 'chunk_id' or 'results' list")

        chunk_id = int(chunk_id)
        matching_chunks = [c for c in run_data["chunks"] if c["id"] == chunk_id]
        if not matching_chunks:
            raise ValueError(f"unknown chunk_id {chunk_id}")
        target_chunk = matching_chunks[0]

        incoming_sha256 = hashlib.sha256(
            json.dumps(results, sort_keys=True, ensure_ascii=False).encode("utf-8")
        ).hexdigest()

        # Idempotency check: if chunk is already complete
        if target_chunk["state"] == "complete":
            if target_chunk.get("result_sha256") == incoming_sha256:
                return {
                    "status": "noop",
                    "chunk_id": chunk_id,
                    "message": "chunk already completed with identical results",
                }
            raise RuntimeError(f"chunk {chunk_id} already completed with different results")

        # Validate source_ids
        expected_ids = set(target_chunk["source_ids"])
        actual_ids = [r.get("source_id") for r in results]

        if set(actual_ids) != expected_ids or len(actual_ids) != len(expected_ids):
            missing = expected_ids - set(actual_ids)
            extra = set(actual_ids) - expected_ids
            dups = len(actual_ids) - len(set(actual_ids))
            raise ValueError(
                f"source_ids mismatch in chunk {chunk_id}: "
                f"missing={sorted(missing)}, extra={sorted(extra)}, dups={dups}"
            )

        # Load sources dictionary
        sources_data = json.loads((run_dir / "sources.json").read_text(encoding="utf-8"))
        sources_by_id = {s["source_id"]: s for s in sources_data}

        # Check at most 1 OK per destination key
        # 1. Existing OK keys from previously completed chunks
        existing_ok_keys: set[tuple[str, int]] = set()
        for other_chunk in run_data["chunks"]:
            if other_chunk["id"] != chunk_id and other_chunk["state"] == "complete":
                part_p = run_dir / f"partial-{other_chunk['id']}.json"
                if part_p.is_file():
                    part_content = json.loads(part_p.read_text(encoding="utf-8"))
                    for rec in part_content.get("receipts", []):
                        if rec.get("state") == "OK":
                            existing_ok_keys.add((str(rec["codigo"]).upper(), int(rec["numero"])))

        # 2. Check incoming OK results
        incoming_ok_keys: set[tuple[str, int]] = set()
        for res in results:
            if res.get("state") == "OK":
                cod = str(res.get("codigo", "")).upper()
                num = int(res.get("numero", 0))
                dest_key = (cod, num)
                if dest_key in incoming_ok_keys:
                    raise ValueError(f"multiple OK results for destination key {cod}/{num} in chunk {chunk_id}")
                if dest_key in existing_ok_keys:
                    raise ValueError(f"destination key {cod}/{num} already accepted as OK in a previous chunk")
                incoming_ok_keys.add(dest_key)

        # Process results and construct provenance receipts
        receipts: list[dict[str, Any]] = []
        etl_path = Path(run_data["etl_path"])

        for res in results:
            src = sources_by_id[res["source_id"]]
            state = res.get("state")

            if state == "OK":
                # A. Canonicalize proposed credit: empty or ASCII-whitespace-only becomes None
                cred = res.get("creditos")
                if is_sem_conteudo(cred):
                    res["creditos"] = None

                # B. Normalize lyrics via Go ETL binary (-normalizar-stdin)
                raw_letra = res.get("letra", "")
                proc = subprocess.run(
                    [str(etl_path), "-normalizar-stdin"],
                    input=raw_letra.encode("utf-8"),
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    check=True,
                )
                normalized_letra = proc.stdout.decode("utf-8")
                res["letra"] = normalized_letra

                # C. Verify with verify_provenance gate
                violations = verify(src, res)
                if violations:
                    raise ValueError(
                        f"provenance verification failed for source {res['source_id']}: {violations}"
                    )

                # D. Compute HashFinal matching Go
                final_sha = compute_final_sha256(
                    res["codigo"],
                    res["numero"],
                    res["titulo"],
                    res["letra"],
                    res["creditos"],
                )
            else:
                # doubt or skip
                violations = verify(src, res)
                if violations:
                    raise ValueError(
                        f"verification failed for non-OK source {res['source_id']}: {violations}"
                    )
                final_sha = None

            receipt = {
                "version": 1,
                "source_id": res["source_id"],
                "source_sha256": res["source_sha256"],
                "final_sha256": final_sha,
                "state": res["state"],
                "codigo": res.get("codigo"),
                "numero": res.get("numero"),
                "titulo": res.get("titulo"),
                "letra": res.get("letra"),
                "creditos": res.get("creditos"),
                "observacao": res.get("observacao", ""),
                "reasons": res.get("reasons", []),
                "identity_decision": res.get("identity_decision"),
                "operations": res.get("operations", []),
                "blocks": res.get("blocks", []),
                "source": src,
            }
            receipts.append(receipt)

        # 3. Publish partial chunk atomically
        partial_payload = {
            "version": 1,
            "chunk_id": chunk_id,
            "incoming_sha256": incoming_sha256,
            "receipts": receipts,
        }
        partial_file = run_dir / f"partial-{chunk_id}.json"
        write_json_atomic(partial_file, partial_payload)

        # 4. Update run.json
        target_chunk["state"] = "complete"
        target_chunk["result_sha256"] = incoming_sha256
        write_json_atomic(run_json_p, run_data)

        return {
            "status": "ok",
            "chunk_id": chunk_id,
            "receipts_count": len(receipts),
        }


def assemble(run_dir: Path, skill_dir: Path = SKILL_ROOT) -> dict[str, Any]:
    """Assembles completed chunks into sanitized.csv, duvidas.csv, provenance.jsonl, and relatorio.md."""
    run_dir = run_dir.resolve()
    with CampaignLock(run_dir):
        run_json_p = run_dir / "run.json"
        if not run_json_p.is_file():
            raise FileNotFoundError(f"run.json not found in {run_dir}")

        run_data = json.loads(run_json_p.read_text(encoding="utf-8"))
        validate_integrity(run_data, skill_dir)
        reconcile_state(run_dir, run_data)

        incomplete = [c["id"] for c in run_data["chunks"] if c["state"] != "complete"]
        if incomplete:
            raise RuntimeError(f"cannot assemble: chunks {incomplete} are not complete")

        # Collect all receipts in order of chunks
        all_receipts: list[dict[str, Any]] = []
        for chunk in run_data["chunks"]:
            part_p = run_dir / f"partial-{chunk['id']}.json"
            part_content = json.loads(part_p.read_text(encoding="utf-8"))
            all_receipts.extend(part_content["receipts"])

        # Check destination key uniqueness across all OK receipts
        ok_keys: set[tuple[str, int]] = set()
        for rec in all_receipts:
            if rec["state"] == "OK":
                key = (str(rec["codigo"]).upper(), int(rec["numero"]))
                if key in ok_keys:
                    raise RuntimeError(f"duplicate destination key in OK receipts: {key[0]}/{key[1]}")
                ok_keys.add(key)

        # 1. Generate sanitized.csv
        sanitized_csv_p = run_dir / "sanitized.csv"
        sanitized_header = [
            "pasta",
            "arquivo",
            "hinario",
            "numero",
            "titulo",
            "letra",
            "status",
            "observacao",
            "creditos",
            "source_id",
            "source_sha256",
        ]
        with sanitized_csv_p.open("w", encoding="utf-8", newline="") as f:
            writer = csv.DictWriter(f, fieldnames=sanitized_header)
            writer.writeheader()
            for rec in all_receipts:
                if rec["state"] == "OK":
                    src = rec.get("source", {})
                    writer.writerow({
                        "pasta": src.get("pasta", ""),
                        "arquivo": src.get("arquivo", ""),
                        "hinario": rec.get("codigo", ""),
                        "numero": str(rec.get("numero", "")),
                        "titulo": rec.get("titulo", ""),
                        "letra": rec.get("letra", ""),
                        "status": "OK",
                        "observacao": rec.get("observacao", "") or "",
                        "creditos": rec.get("creditos") or "",
                        "source_id": rec.get("source_id", ""),
                        "source_sha256": rec.get("source_sha256", ""),
                    })

        # 2. Generate duvidas.csv
        duvidas_csv_p = run_dir / "duvidas.csv"
        duvidas_header = [
            "pasta",
            "arquivo",
            "hinario",
            "numero",
            "titulo",
            "letra",
            "status",
            "observacao",
            "reasons",
            "source_id",
            "source_sha256",
        ]
        with duvidas_csv_p.open("w", encoding="utf-8", newline="") as f:
            writer = csv.DictWriter(f, fieldnames=duvidas_header)
            writer.writeheader()
            for rec in all_receipts:
                if rec["state"] != "OK":
                    src = rec.get("source", {})
                    writer.writerow({
                        "pasta": src.get("pasta", ""),
                        "arquivo": src.get("arquivo", ""),
                        "hinario": rec.get("codigo") or src.get("codigo") or "",
                        "numero": str(rec.get("numero") or src.get("numero") or ""),
                        "titulo": rec.get("titulo") or src.get("titulo_fonte") or "",
                        "letra": rec.get("letra") or src.get("raw_text") or "",
                        "status": rec.get("state", ""),
                        "observacao": rec.get("observacao", "") or "",
                        "reasons": "; ".join(rec.get("reasons", [])),
                        "source_id": rec.get("source_id", ""),
                        "source_sha256": rec.get("source_sha256", ""),
                    })

        # 3. Generate provenance.jsonl
        prov_p = run_dir / "provenance.jsonl"
        with prov_p.open("w", encoding="utf-8") as f:
            for rec in all_receipts:
                f.write(json.dumps(rec, ensure_ascii=False) + "\n")

        # 4. Generate relatorio.md
        rel_p = run_dir / "relatorio.md"
        count_ok = sum(1 for r in all_receipts if r["state"] == "OK")
        count_doubt = sum(1 for r in all_receipts if r["state"] == "doubt")
        count_skip = sum(1 for r in all_receipts if r["state"] == "skip")
        total_sources = len(all_receipts)

        # Breakdown by hymnal
        by_hinario: dict[str, dict[str, int]] = {}
        for r in all_receipts:
            h_code = str(r.get("codigo") or r.get("source", {}).get("codigo") or "SEM_CHAVE").upper()
            if h_code not in by_hinario:
                by_hinario[h_code] = {"total": 0, "OK": 0, "doubt": 0, "skip": 0}
            by_hinario[h_code]["total"] += 1
            st = r["state"]
            if st in by_hinario[h_code]:
                by_hinario[h_code][st] += 1

        rel_lines = [
            "# Relatório de Sanitização de Hinos",
            "",
            "## Estatísticas Gerais",
            "",
            "| Métrica | Quantidade |",
            "|---|---|",
            f"| Total de fontes processadas | {total_sources} |",
            f"| Elegíveis para importação (OK) | {count_ok} |",
            f"| Dúvidas / Pendências (doubt) | {count_doubt} |",
            f"| Ignorados / Descartados (skip) | {count_skip} |",
            f"| Chaves de destino únicas | {len(ok_keys)} |",
            "",
            "## Distribuição por Hinário / Coletânea",
            "",
            "| Coletânea | Total | OK | Dúvidas | Ignorados |",
            "|---|---|---|---|---|",
        ]
        for h_code, stats in sorted(by_hinario.items()):
            rel_lines.append(
                f"| {h_code} | {stats['total']} | {stats['OK']} | {stats['doubt']} | {stats['skip']} |"
            )

        # Doubts and Skips details
        non_ok_receipts = [r for r in all_receipts if r["state"] != "OK"]
        if non_ok_receipts:
            rel_lines.extend([
                "",
                "## Resumo de Dúvidas e Itens Ignorados",
                "",
                "| Arquivo | Chave Proposta | Estado | Razões | Observação |",
                "|---|---|---|---|---|",
            ])
            for r in non_ok_receipts:
                src = r.get("source", {})
                arq = src.get("arquivo", "")
                cod = r.get("codigo") or src.get("codigo") or "-"
                num = r.get("numero") or src.get("numero") or "-"
                reasons_str = "; ".join(r.get("reasons", [])) or "-"
                obs = r.get("observacao", "") or "-"
                rel_lines.append(f"| {arq} | {cod}/{num} | {r['state']} | {reasons_str} | {obs} |")

        # Before / After comparisons
        ok_receipts = [r for r in all_receipts if r["state"] == "OK"]
        if ok_receipts:
            rel_lines.extend([
                "",
                "## Amostra de Comparações Antes e Depois (Itens OK)",
                "",
            ])
            for r in ok_receipts[:10]:
                src = r.get("source", {})
                cod = r.get("codigo")
                num = r.get("numero")
                tit = r.get("titulo")
                raw = src.get("raw_text", "").strip()
                final_l = r.get("letra", "").strip()
                rel_lines.extend([
                    f"### {cod}/{num} — {tit}",
                    "",
                    "**Antes (Fonte Bruta):**",
                    "```text",
                    raw[:200] + ("..." if len(raw) > 200 else ""),
                    "```",
                    "",
                    "**Depois (Sanitizado):**",
                    "```text",
                    final_l[:200] + ("..." if len(final_l) > 200 else ""),
                    "```",
                    "",
                ])

        rel_p.write_text("\n".join(rel_lines) + "\n", encoding="utf-8")

        return {
            "total_sources": total_sources,
            "count_ok": count_ok,
            "count_doubt": count_doubt,
            "count_skip": count_skip,
            "unique_keys": len(ok_keys),
        }


def main(argv: list[str] | None = None) -> int:
    """CLI dispatcher for the campaign manager."""
    parser = argparse.ArgumentParser(description="Resumable campaign manager for hymn sanitization.")
    subparsers = parser.add_subparsers(dest="subcommand", required=True)

    # prepare
    p_prep = subparsers.add_parser("prepare", help="Initialize a new campaign run.")
    p_prep.add_argument("--csv", required=True, type=Path, help="Input CSV path")
    p_prep.add_argument("--snapshot", required=True, type=Path, help="Snapshot SQLite DB path")
    p_prep.add_argument("--out", required=True, type=Path, help="Output RUN directory path")
    p_prep.add_argument("--etl", required=True, type=Path, help="ETL binary path")
    p_prep.add_argument("--ole-csv", required=False, type=Path, default=None, help="Optional OLE CSV path")
    p_prep.add_argument("--chunk-size", required=False, type=int, default=10, help="Groups per chunk (default: 10)")

    # next
    p_next = subparsers.add_parser("next", help="Get next pending chunk for worker.")
    p_next.add_argument("--out", required=True, type=Path, help="RUN directory path")

    # accept
    p_acc = subparsers.add_parser("accept", help="Accept worker chunk result.")
    p_acc.add_argument("--out", required=True, type=Path, help="RUN directory path")
    p_acc.add_argument("--chunk", required=True, type=Path, help="Chunk JSON result path")

    # assemble
    p_asm = subparsers.add_parser("assemble", help="Assemble all completed chunks into final outputs.")
    p_asm.add_argument("--out", required=True, type=Path, help="RUN directory path")

    args = parser.parse_args(argv)

    try:
        if args.subcommand == "prepare":
            res = prepare(
                csv_path=args.csv,
                snapshot_path=args.snapshot,
                run_dir=args.out,
                etl_path=args.etl,
                ole_csv_path=args.ole_csv,
                chunk_size=args.chunk_size,
            )
            print(f"Prepared campaign at {args.out} with {len(res['chunks'])} chunks.")
            return 0

        elif args.subcommand == "next":
            res = get_next(run_dir=args.out)
            sys.stdout.write(json.dumps(res, ensure_ascii=False) + "\n")
            return 0

        elif args.subcommand == "accept":
            res = accept(run_dir=args.out, chunk_file=args.chunk)
            print(f"Accepted chunk {res.get('chunk_id')}: {res.get('status')}")
            return 0

        elif args.subcommand == "assemble":
            res = assemble(run_dir=args.out)
            print(f"Assembled campaign at {args.out}: {res['count_ok']} OK items, {res['count_doubt']} doubts.")
            return 0

        else:
            parser.print_help(sys.stderr)
            return 1
    except Exception as e:
        sys.stderr.write(f"Error: {e}\n")
        return 1


if __name__ == "__main__":
    sys.exit(main())
