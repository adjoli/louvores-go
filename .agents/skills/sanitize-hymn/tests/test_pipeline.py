"""Tests for hymn preparation, identity, and verifiable provenance gates.

All tests use synthetic test fixtures. No real lyrics or private databases
are copied into this repository.
"""

import csv
import hashlib
import json
import os
import shutil
import sqlite3
import sys
import tempfile
import unittest
from pathlib import Path

# Add skill root to sys.path so 'scripts.*' can be imported from any cwd
SKILL_ROOT = Path(__file__).resolve().parent.parent
if str(SKILL_ROOT) not in sys.path:
    sys.path.insert(0, str(SKILL_ROOT))

# Imports from scripts (will fail until implemented in RED phase)
from scripts.prepare import (
    compute_final_sha256,
    compute_source_id,
    compute_source_sha256,
    fuzzy_jaccard,
    normalize_tokens,
    prepare_sources,
)
from scripts.verify_provenance import verify
from scripts.campaign import (
    CampaignLock,
    accept,
    assemble,
    get_next,
    prepare,
)


def create_synthetic_snapshot(db_path: Path) -> None:
    """Creates a synthetic SQLite snapshot for testing."""
    conn = sqlite3.connect(db_path)
    cur = conn.cursor()
    cur.execute("""
        CREATE TABLE coletanea (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            codigo TEXT NOT NULL,
            titulo TEXT NOT NULL
        )
    """)
    cur.execute("""
        CREATE TABLE hino (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            coletanea_id INTEGER NOT NULL REFERENCES coletanea(id),
            numeracao INTEGER,
            titulo TEXT NOT NULL,
            letra TEXT,
            creditos TEXT,
            revisado INTEGER NOT NULL DEFAULT 0
        )
    """)

    cur.execute("INSERT INTO coletanea (id, codigo, titulo) VALUES (1, 'CC', 'Cantor Cristão')")
    cur.execute("INSERT INTO coletanea (id, codigo, titulo) VALUES (2, 'HCC', 'Hinário para o Culto Cristão')")
    cur.execute("INSERT INTO coletanea (id, codigo, titulo) VALUES (3, 'COR', 'Corinhos')")
    cur.execute("INSERT INTO coletanea (id, codigo, titulo) VALUES (4, 'VM', 'Vozes Missionárias')")

    # Synthetic hymns:
    # CC 42: destination for hash vector test
    cur.execute("INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado) VALUES (1, 1, 42, 'Ó Vem', NULL, NULL, 0)")
    # CC 2: contaminated test data in local snapshot
    cur.execute("INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado) VALUES (2, 1, 2, 'CC Dois Contaminado', 'dados de teste fake', 'Autor', 0)")
    # HCC 23: real destination
    cur.execute("INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado) VALUES (3, 2, 23, 'Louvemos ao Rei', NULL, NULL, 0)")
    # HCC 233: destination
    cur.execute("INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado) VALUES (4, 2, 233, 'Vinde Fiéis', NULL, NULL, 0)")
    # COR 132: destination
    cur.execute("INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado) VALUES (5, 3, 132, 'Ao Sentir o Amor de Deus', NULL, NULL, 0)")
    # VM 340: destination with official title
    cur.execute("INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado) VALUES (6, 4, 340, 'Vigor Olhou', NULL, NULL, 0)")
    # COR 10: destination without lyrics, for fuzzy match test
    cur.execute("INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado) VALUES (7, 3, 10, 'Chuva de Graça e Luz', NULL, NULL, 0)")
    # COR 11: destination with lyrics already present (excluded from fuzzy)
    cur.execute("INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado) VALUES (8, 3, 11, 'Outra Chuva de Bênção', 'Letra Existente', NULL, 0)")
    # COR 12: destination already reviewed (excluded from fuzzy)
    cur.execute("INSERT INTO hino (id, coletanea_id, numeracao, titulo, letra, creditos, revisado) VALUES (9, 3, 12, 'Chuva Divina e Luz', NULL, NULL, 1)")

    conn.commit()
    conn.close()


class TestContractHashFormulas(unittest.TestCase):
    """Verifies that Python hash implementations match Go and the spec verbatim."""

    def test_shared_test_vectors_final_sha256(self):
        # Vector 1: codigo CC, numero 42, titulo Ó Vem, letra "    Ó Vem\n", creditos nil
        want1 = "35459d763c853fcd19927927ab8850e0cd5344deb0ae035636507751b175b8ec"
        h1 = compute_final_sha256("CC", 42, "Ó Vem", "    Ó Vem\n", None)
        self.assertEqual(h1, want1)

        # Vector 2: same, with creditos as empty string ""
        want2 = "5bb4297d9baea87d0c749281e41b7e103f0dc1f96ab4d2fc6e69a4e4e7115f93"
        h2 = compute_final_sha256("CC", 42, "Ó Vem", "    Ó Vem\n", "")
        self.assertEqual(h2, want2)

        # Vector 3: with non-empty creditos
        h3 = compute_final_sha256("CC", 42, "Ó Vem", "    Ó Vem\n", "Autor")
        self.assertNotEqual(h3, h1)
        self.assertNotEqual(h3, h2)

        # Rejection of NUL bytes
        with self.assertRaises(ValueError):
            compute_final_sha256("C\x00C", 1, "T", "L", None)
        with self.assertRaises(ValueError):
            compute_final_sha256("CC", 1, "T\x00itulo", "L", None)
        with self.assertRaises(ValueError):
            compute_final_sha256("CC", 1, "T", "L\x00etra", None)
        with self.assertRaises(ValueError):
            compute_final_sha256("CC", 1, "T", "L", "C\x00red")

        # Rejection of invalid number
        with self.assertRaises(ValueError):
            compute_final_sha256("CC", 0, "T", "L", None)
        with self.assertRaises(ValueError):
            compute_final_sha256("CC", -1, "T", "L", None)

    def test_source_id_and_source_sha256(self):
        raw_text = "Primeiro verso\nSegundo verso"
        expected_raw_sha = hashlib.sha256(raw_text.encode("utf-8")).hexdigest()
        self.assertEqual(compute_source_sha256(raw_text), expected_raw_sha)

        row_dict = {
            "pasta": "slides",
            "arquivo": "CC-042.pptx",
            "hinario": "CC",
            "numero": "42",
            "titulo": "Ó Vem",
            "letra": raw_text,
            "status": "PENDENTE",
            "observacao": "",
        }
        canonical_json = json.dumps(row_dict, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
        expected_src_id = hashlib.sha256("CC-042.pptx".encode("utf-8") + b"\x00" + canonical_json.encode("utf-8")).hexdigest()
        self.assertEqual(compute_source_id(row_dict, has_creditos_col=False), expected_src_id)


class TestNoOLEOverlay(unittest.TestCase):
    """Proves that base CSV remains byte-for-byte intact and OLE is read-only complement."""

    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.tmppath = Path(self.tmpdir.name)
        self.db_path = self.tmppath / "snapshot.db"
        create_synthetic_snapshot(self.db_path)

    def tearDown(self):
        self.tmpdir.cleanup()

    def test_rejects_wal_snapshot_through_alias_or_copy(self):
        csv_path = self.tmppath / "empty.csv"
        csv_path.write_text("pasta,arquivo,hinario,numero,titulo,letra,status,observacao\n")
        writer = sqlite3.connect(self.db_path)
        try:
            writer.execute("PRAGMA journal_mode=WAL")
            writer.execute("UPDATE hino SET letra='Conteúdo Vivo No WAL' WHERE id=1")
            writer.commit()
            for kind in ("symlink", "hardlink", "copy"):
                with self.subTest(kind=kind):
                    alias = self.tmppath / f"{kind}.db"
                    if kind == "symlink":
                        alias.symlink_to(self.db_path)
                    elif kind == "hardlink":
                        os.link(self.db_path, alias)
                    else:
                        shutil.copyfile(self.db_path, alias)
                    with self.assertRaises(ValueError):
                        prepare_sources(csv_path, alias)
        finally:
            writer.close()

    def test_base_intact_and_ole_complement_read_only(self):
        base_csv = self.tmppath / "base.csv"
        raw_text = "Vigor olhou para a cruz\nE viu a luz"
        base_content = (
            "pasta,arquivo,hinario,numero,titulo,letra,status,observacao\n"
            f'slides,VM-340.pptx,VM,340,Vigor Olhou,"{raw_text}",PENDENTE,\n'
        )
        base_csv.write_text(base_content, encoding="utf-8")
        base_bytes_before = base_csv.read_bytes()

        ole_csv = self.tmppath / "ole.csv"
        ole_content = (
            "arquivo,slide_count,boundary_markers\n"
            'VM-340.pptx,2,"slide1|slide2"\n'
        )
        ole_csv.write_text(ole_content, encoding="utf-8")

        sources = prepare_sources(base_csv, self.db_path, ole_csv_path=ole_csv)

        # Base file is byte-for-byte identical
        self.assertEqual(base_csv.read_bytes(), base_bytes_before)
        self.assertEqual(len(sources), 1)

        src = sources[0]
        # Text in source is exactly the base CSV text, not overlaid
        self.assertEqual(src["raw_text"], raw_text)
        # OLE boundaries are included in evidence only
        self.assertIn("ole", src["evidence"])
        self.assertEqual(src["evidence"]["ole"].get("slide_count"), "2")


class TestIdentityAndGrouping(unittest.TestCase):
    """Tests grouping of siblings, doubts on reference mismatch, and alias handling."""

    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.tmppath = Path(self.tmpdir.name)
        self.db_path = self.tmppath / "snapshot.db"
        create_synthetic_snapshot(self.db_path)

    def tearDown(self):
        self.tmpdir.cleanup()

    def test_hcc_internal_reference_creates_doubt(self):
        # HCC 233 references HCC 23 inside its text/header -> must flag doubt
        base_csv = self.tmppath / "hcc.csv"
        raw_text = "HCC 23 - Louvemos ao Rei\nTexto diferente do 233"
        content = (
            "pasta,arquivo,hinario,numero,titulo,letra,status,observacao\n"
            f'slides,HCC-233.pptx,HCC,233,Vinde Fiéis,"{raw_text}",PENDENTE,\n'
        )
        base_csv.write_text(content, encoding="utf-8")

        sources = prepare_sources(base_csv, self.db_path)
        self.assertEqual(len(sources), 1)
        src = sources[0]
        self.assertEqual(src["state"], "doubt")
        self.assertTrue(any("23" in r for r in src["reasons"]))

    def test_cor_132_grouped_with_ignorar_alternative(self):
        # Two sources for COR 132: one PENDENTE, one IGNORAR.
        # Must share group_id and sibling_source_ids; neither selected by mtime.
        base_csv = self.tmppath / "cor132.csv"
        content = (
            "pasta,arquivo,hinario,numero,titulo,letra,status,observacao\n"
            'slides,COR-132-old.pptx,COR,132,Ao Sentir,Texto Antigo Errado,PENDENTE,\n'
            'slides,COR-132-new.pptx,COR,132,Ao Sentir o Amor de Deus,Texto Bom,IGNORAR,duplicado mtime\n'
        )
        base_csv.write_text(content, encoding="utf-8")

        sources = prepare_sources(base_csv, self.db_path)
        self.assertEqual(len(sources), 2)
        s1, s2 = sources[0], sources[1]

        self.assertEqual(s1["group_id"], "COR/132")
        self.assertEqual(s2["group_id"], "COR/132")
        self.assertEqual(set(s1["sibling_source_ids"]), {s1["source_id"], s2["source_id"]})
        self.assertEqual(set(s2["sibling_source_ids"]), {s1["source_id"], s2["source_id"]})
        self.assertEqual(s2["status_inicial"], "IGNORAR")
        self.assertEqual(s2["observacao_inicial"], "duplicado mtime")

    def test_vm_340_alias_evidence(self):
        # Source filename or header has alternate title
        base_csv = self.tmppath / "vm340.csv"
        raw_text = "Olhou Para a Cruz\nVigor olhou para a cruz\nE tudo mudou"
        content = (
            "pasta,arquivo,hinario,numero,titulo,letra,status,observacao\n"
            f'slides,VM-340-OlhouCruz.pptx,VM,340,Vigor Olhou,"{raw_text}",PENDENTE,\n'
        )
        base_csv.write_text(content, encoding="utf-8")

        sources = prepare_sources(base_csv, self.db_path)
        self.assertEqual(len(sources), 1)
        src = sources[0]
        # Candidate titles are in evidence
        self.assertIn("Olhou Para a Cruz", src["evidence"]["candidates"])
        self.assertNotEqual(src["state"], "OK")  # never initial OK

    def test_empty_raw_text_becomes_doubt(self):
        base_csv = self.tmppath / "empty.csv"
        content = (
            "pasta,arquivo,hinario,numero,titulo,letra,status,observacao\n"
            'slides,CC-001.pptx,CC,1,Hino Um,"   \n\t",PENDENTE,\n'
        )
        base_csv.write_text(content, encoding="utf-8")
        sources = prepare_sources(base_csv, self.db_path)
        self.assertEqual(len(sources), 1)
        self.assertEqual(sources[0]["state"], "doubt")
        self.assertTrue(any("vazio" in r.lower() for r in sources[0]["reasons"]))

    def test_cc2_contaminated_data_not_manual_rule(self):
        # CC/2 contains test data in the snapshot, should not break prepare_sources
        base_csv = self.tmppath / "cc2.csv"
        content = (
            "pasta,arquivo,hinario,numero,titulo,letra,status,observacao\n"
            'slides,CC-002.pptx,CC,2,CC Dois Contaminado,"Primeiro verso",PENDENTE,\n'
        )
        base_csv.write_text(content, encoding="utf-8")
        sources = prepare_sources(base_csv, self.db_path)
        self.assertEqual(len(sources), 1)
        # Snapshot destination info is captured without adopting contaminated author rule
        self.assertEqual(sources[0]["creditos_destino"], "Autor")

    def test_internal_references_stanza_numbers_not_false_positive(self):
        # Stanza numbers like 1., 2. in lyrics must NOT be flagged as internal reference conflicts
        base_csv = self.tmppath / "stanza_nums.csv"
        raw_text = "1. Primeiro verso da estrofe\nLinha dois\n\n2. Segundo verso da estrofe\nLinha dois"
        content = (
            "pasta,arquivo,hinario,numero,titulo,letra,status,observacao\n"
            f'slides,CC-042.pptx,CC,42,Ó Vem,"{raw_text}",PENDENTE,\n'
        )
        base_csv.write_text(content, encoding="utf-8")
        sources = prepare_sources(base_csv, self.db_path)
        self.assertEqual(len(sources), 1)
        src = sources[0]
        # Must be candidate, NOT doubt
        self.assertEqual(src["state"], "candidate")
        self.assertEqual(src["reasons"], [])


class TestFuzzySuggestions(unittest.TestCase):
    """Tests fuzzy suggestions with exact stopwords, NFD, delimiter, Jaccard >= 0.75, margin >= 0.10."""

    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.tmppath = Path(self.tmpdir.name)
        self.db_path = self.tmppath / "snapshot.db"
        create_synthetic_snapshot(self.db_path)

    def tearDown(self):
        self.tmpdir.cleanup()

    def test_token_normalization_exact_stopwords(self):
        # Stopwords: {a, o, as, os, de, da, do, das, dos, e}
        # NFD lower without accents, punctuation as delimiter
        text = "A Chuva, de Graça & Luz (e os Dias)!"
        tokens = normalize_tokens(text)
        # 'a', 'de', 'e', 'os' are stopwords
        # 'graça' -> 'graca', 'bênção' -> 'bencao'
        self.assertEqual(tokens, {"chuva", "graca", "luz", "dias"})

    def test_fuzzy_suggests_without_auto_approval_and_excludes_covered(self):
        base_csv = self.tmppath / "unkeyed.csv"
        # Unkeyed source with title close to COR 10 ("Chuva de Graça e Luz")
        raw_text = "Chuva de Graça e de Luz\nCai sobre nós"
        content = (
            "pasta,arquivo,hinario,numero,titulo,letra,status,observacao\n"
            f'slides,Avulso-ChuvaGraca.pptx,,,,"{raw_text}",PENDENTE,\n'
        )
        base_csv.write_text(content, encoding="utf-8")

        sources = prepare_sources(base_csv, self.db_path)
        self.assertEqual(len(sources), 1)
        src = sources[0]

        # Unkeyed sources never auto-approve to OK
        self.assertEqual(src["state"], "doubt")
        self.assertEqual(src["group_id"], src["source_id"])

        # Check suggestions in evidence
        suggestions = src["evidence"].get("fuzzy_suggestions", [])
        self.assertTrue(len(suggestions) > 0)
        top = suggestions[0]
        self.assertEqual(top["codigo"], "COR")
        self.assertEqual(top["numero"], 10)
        self.assertGreaterEqual(top["jaccard"], 0.75)
        self.assertGreaterEqual(top["margin"], 0.10)

        # Dest 11 (has lyrics) and Dest 12 (reviewed) must be excluded from suggestions
        suggested_nums = [s["numero"] for s in suggestions]
        self.assertNotIn(11, suggested_nums)
        self.assertNotIn(12, suggested_nums)

    def test_fuzzy_margin_calculation_runner_up_not_dropped(self):
        # Dest A has J = 0.818 (>= 0.75), Dest B has J = 0.727 (< 0.75).
        # Margin is 0.818 - 0.727 = 0.091 < 0.10.
        # Runner-up Dest B must NOT be discarded; no suggestion should be generated.
        words = ["amor", "bencao", "celeste", "divino", "eterno", "fiel", "graca", "harmonia", "infinito", "justo", "luz"]
        tit_src = " ".join(words)       # 11 words
        tit_a = " ".join(words[:9])      # 9 words -> J = 9/11 = 0.8182
        tit_b = " ".join(words[:8])      # 8 words -> J = 8/11 = 0.7273

        conn = sqlite3.connect(self.db_path)
        cur = conn.cursor()
        cur.execute("INSERT INTO hino (coletanea_id, numeracao, titulo, letra, revisado) VALUES (3, 30, ?, NULL, 0)", (tit_a,))
        cur.execute("INSERT INTO hino (coletanea_id, numeracao, titulo, letra, revisado) VALUES (3, 31, ?, NULL, 0)", (tit_b,))
        conn.commit()
        conn.close()

        base_csv = self.tmppath / "unkeyed_runner_up.csv"
        content = (
            "pasta,arquivo,hinario,numero,titulo,letra,status,observacao\n"
            f'slides,Avulso-MarginTest.pptx,,,,"{tit_src}\nVerso",PENDENTE,\n'
        )
        base_csv.write_text(content, encoding="utf-8")

        sources = prepare_sources(base_csv, self.db_path)
        self.assertEqual(len(sources), 1)
        src = sources[0]

        # No suggestions should be generated because margin is 0.091 < 0.10
        suggestions = src["evidence"].get("fuzzy_suggestions", [])
        self.assertEqual(len(suggestions), 0)



class TestProvenanceVerifier(unittest.TestCase):
    """Tests the verifiable provenance gate verify(source, result)."""

    def test_capitalization_and_spacing_changes_pass(self):
        # VIGOROLHOU -> Vigor Olhou passes
        raw = "VIGOR OLHOU PARA A CRUZ\nE VIU A LUZ\n\nCORO:\nSIM, ELE VIU A LUZ\nDE JESUS"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-1",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        # raw breakdown:
        # [0, 36): "VIGOR OLHOU PARA A CRUZ\nE VIU A LUZ" (strophe 1)
        # [36, 44): "\n\nCORO:\n" -> operation 'refrain_label' on "CORO:" [38, 43)
        # [44, 71): "SIM, ELE VIU A LUZ\nDE JESUS" (refrain)
        c_start = raw.index("CORO:")
        c_end = c_start + len("CORO:")

        s1_start = 0
        s1_end = c_start  # covers strophe 1 and trailing whitespace before CORO:

        ref_start = c_end
        ref_end = len(raw)

        cleaned_letra = (
            "Vigor Olhou Para A Cruz\n"
            "E Viu A Luz\n\n"
            "    Sim, Ele Viu A Luz\n"
            "    De Jesus"
        )

        result = {
            "source_id": "src-1",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "VM",
            "numero": 340,
            "titulo": "Vigor Olhou",
            "letra": cleaned_letra,
            "creditos": None,
            "observacao": "formatado canônico",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["titulo correspondente e incipit idêntico"],
            },
            "operations": [
                {
                    "start": c_start,
                    "end": c_end,
                    "type": "refrain_label",
                    "evidence": "label CORO removido",
                }
            ],
            "blocks": [
                {"start": 0, "end": c_start, "type": "estrofe"},
                {"start": c_end, "end": len(raw), "type": "refrao"},
            ],
        }

        violations = verify(source, result)
        self.assertEqual(violations, [])

    def test_deletion_of_word_fails(self):
        # Removing "NÃO" must fail provenance check
        raw = "EU NÃO TEMEREI\nO MAL"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-2",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        # Agent improperly dropped "NÃO"
        bad_letra = "Eu Temerei\nO Mal"
        result = {
            "source_id": "src-2",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 10,
            "titulo": "Não Temerei",
            "letra": bad_letra,
            "creditos": None,
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [],
            "blocks": [{"start": 0, "end": len(raw), "type": "estrofe"}],
        }

        violations = verify(source, result)
        self.assertTrue(any("mismatch" in v.lower() or "alterado" in v.lower() for v in violations))

    def test_alteration_of_accent_or_punctuation_fails(self):
        raw = "Glória, ó Deus!"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-3",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        # Altering accent: "Gloria, o Deus!"
        bad_letra = "Gloria, o Deus!"
        result = {
            "source_id": "src-3",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 10,
            "titulo": "Glória",
            "letra": bad_letra,
            "creditos": None,
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [],
            "blocks": [{"start": 0, "end": len(raw), "type": "estrofe"}],
        }

        violations = verify(source, result)
        self.assertTrue(len(violations) > 0)

    def test_reordering_verses_fails(self):
        raw = "Primeiro verso\nSegundo verso"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-4",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        inverted_letra = "Segundo verso\nPrimeiro verso"
        result = {
            "source_id": "src-4",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 10,
            "titulo": "Versos",
            "letra": inverted_letra,
            "creditos": None,
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [],
            "blocks": [{"start": 0, "end": len(raw), "type": "estrofe"}],
        }

        violations = verify(source, result)
        self.assertTrue(len(violations) > 0)

    def test_swapping_verse_between_blocks_fails(self):
        raw = "Verso Um\nVerso Dois\n\n    Verso Tres\n    Verso Quatro"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-5",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        # Verse 2 moved into refrain!
        swapped_letra = "Verso Um\n\n    Verso Dois\n    Verso Tres\n    Verso Quatro"
        idx_split = raw.index("\n\n")

        result = {
            "source_id": "src-5",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 10,
            "titulo": "Hino",
            "letra": swapped_letra,
            "creditos": None,
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [],
            "blocks": [
                {"start": 0, "end": idx_split, "type": "estrofe"},
                {"start": idx_split, "end": len(raw), "type": "refrao"},
            ],
        }

        violations = verify(source, result)
        self.assertTrue(len(violations) > 0)

    def test_duplicate_refrain_without_spans_fails(self):
        raw = "Estrofe Um\n\n    Refrão Único"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-6",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        # Duplicating refrain in letra without source spans
        duplicated_letra = "Estrofe Um\n\n    Refrão Único\n\n    Refrão Único"
        idx_split = raw.index("\n\n")

        result = {
            "source_id": "src-6",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 10,
            "titulo": "Hino",
            "letra": duplicated_letra,
            "creditos": None,
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [],
            "blocks": [
                {"start": 0, "end": idx_split, "type": "estrofe"},
                {"start": idx_split, "end": len(raw), "type": "refrao"},
            ],
        }

        violations = verify(source, result)
        # Block count mismatch or span conservation mismatch
        self.assertTrue(len(violations) > 0)

    def test_unaccounted_non_whitespace_characters_fails(self):
        raw = "Verso Um\nTexto Perdido\nVerso Dois"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-7",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        # Only covers Verso Um and Verso Dois, leaving "Texto Perdido" silently unmapped
        u_end = raw.index("\nTexto Perdido")
        d_start = raw.index("Verso Dois")
        letra = "Verso Um\n\nVerso Dois"

        result = {
            "source_id": "src-7",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 10,
            "titulo": "Hino",
            "letra": letra,
            "creditos": None,
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [],
            "blocks": [
                {"start": 0, "end": u_end, "type": "estrofe"},
                {"start": d_start, "end": len(raw), "type": "estrofe"},
            ],
        }

        violations = verify(source, result)
        self.assertTrue(any("unaccounted" in v.lower() or "partição" in v.lower() or "perdido" in v.lower() for v in violations))

    def test_crlf_rejected_by_verifier(self):
        raw = "Verso Um\nVerso Dois"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-crlf",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        result = {
            "source_id": "src-crlf",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 10,
            "titulo": "Hino",
            "letra": "Verso Um\r\nVerso Dois",  # CRLF!
            "creditos": None,
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [],
            "blocks": [{"start": 0, "end": len(raw), "type": "estrofe"}],
        }

        violations = verify(source, result)
        self.assertTrue(any("crlf" in v.lower() for v in violations))

    def test_line_length_and_block_line_limits(self):
        # 64 runes limit per line (excluding 4-space refrain indent)
        # 10 lines limit per block
        long_line = "A" * 65
        raw = f"Estrofe\n{long_line}"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-8",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        result = {
            "source_id": "src-8",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 10,
            "titulo": "Hino",
            "letra": raw,
            "creditos": None,
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [],
            "blocks": [{"start": 0, "end": len(raw), "type": "estrofe"}],
        }

        violations = verify(source, result)
        self.assertTrue(any("64" in v for v in violations))

    def test_monotonic_ordering_enforced(self):
        raw = "Verso Um\n\nVerso Dois"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-mono",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        # Blocks specified in reverse order: block 2 starts at 0, block 1 starts at 10
        idx_two = raw.index("Verso Dois")
        result_reversed_blocks = {
            "source_id": "src-mono",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 10,
            "titulo": "Hino",
            "letra": "Verso Dois\n\nVerso Um",
            "creditos": None,
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [],
            "blocks": [
                {"start": idx_two, "end": len(raw), "type": "estrofe"},
                {"start": 0, "end": len("Verso Um"), "type": "estrofe"},
            ],
        }

        violations = verify(source, result_reversed_blocks)
        self.assertTrue(any("monotonically" in v.lower() or "order" in v.lower() for v in violations))

    def test_operation_semantics_validated(self):
        raw = "Aleluia\nVerso Um"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-sem",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        idx_aleluia = len("Aleluia")
        v_start = raw.index("Verso Um")

        # Trying to use stanza_number on non-numeric text "Aleluia"
        result_bad_op = {
            "source_id": "src-sem",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 10,
            "titulo": "Hino",
            "letra": "Verso Um",
            "creditos": None,
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [
                {
                    "start": 0,
                    "end": idx_aleluia,
                    "type": "stanza_number",
                    "evidence": "numero de estrofe",
                }
            ],
            "blocks": [{"start": v_start, "end": len(raw), "type": "estrofe"}],
        }

        violations = verify(source, result_bad_op)
        self.assertTrue(any("stanza_number" in v and "numeric" in v for v in violations))

    def test_title_coincident_with_verse_requires_structural_evidence(self):
        # Title "Ó Deus de Amor" coincides with a line in the hymn body
        raw = "Primeiro verso\nÓ Deus de Amor\nTerceiro verso"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-tit-verse",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        t_start = raw.index("Ó Deus de Amor")
        t_end = t_start + len("Ó Deus de Amor")
        b1_end = t_start
        b2_start = t_end

        # Deleting "Ó Deus de Amor" with generic/non-structural evidence fails
        result = {
            "source_id": "src-tit-verse",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 10,
            "titulo": "Ó Deus de Amor",
            "letra": "Primeiro verso\n\nTerceiro verso",
            "creditos": None,
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [
                {
                    "start": t_start,
                    "end": t_end,
                    "type": "title",
                    "evidence": "apagar",
                }
            ],
            "blocks": [
                {"start": 0, "end": b1_end, "type": "estrofe"},
                {"start": b2_start, "end": len(raw), "type": "estrofe"},
            ],
        }

        violations = verify(source, result)
        self.assertTrue(any("title" in v and "evidence" in v for v in violations))


class TestCreditsRules(unittest.TestCase):
    """Tests credits parsing, preservation, and conflict rules."""

    def test_melodia_autor_not_discarded(self):
        raw = "Melodia: Autor Conhecido\nVerso do hino"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-c1",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        v_start = raw.index("Verso do hino")
        result = {
            "source_id": "src-c1",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 1,
            "titulo": "Hino",
            "letra": "Verso do hino",
            "creditos": "Melodia: Autor Conhecido",
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [
                {
                    "start": 0,
                    "end": v_start,
                    "type": "credits",
                    "evidence": "crédito melodia preservado",
                }
            ],
            "blocks": [{"start": v_start, "end": len(raw), "type": "estrofe"}],
        }

        violations = verify(source, result)
        self.assertEqual(violations, [])

    def test_keith_and_kristyn_getty_no_invented_names(self):
        raw = "Letra e Música: Keith and Kristyn Getty\nVerso do hino"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-c2",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        v_start = raw.index("Verso do hino")
        result = {
            "source_id": "src-c2",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 1,
            "titulo": "Hino",
            "letra": "Verso do hino",
            # Preserving literal string without inventing surnames
            "creditos": "Keith and Kristyn Getty",
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [
                {
                    "start": 0,
                    "end": v_start,
                    "type": "credits",
                    "evidence": "credito de autoria",
                }
            ],
            "blocks": [{"start": v_start, "end": len(raw), "type": "estrofe"}],
        }

        violations = verify(source, result)
        self.assertEqual(violations, [])

    def test_conflicting_destination_credits_flags_doubt(self):
        raw = "Autor Proposto\nVerso do hino"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-c3",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            # Destination in snapshot already has different non-empty credits!
            "creditos_destino": "Autor Diferente Existente",
        }

        v_start = raw.index("Verso do hino")
        result = {
            "source_id": "src-c3",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 1,
            "titulo": "Hino",
            "letra": "Verso do hino",
            "creditos": "Autor Proposto",
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [
                {
                    "start": 0,
                    "end": v_start,
                    "type": "credits",
                    "evidence": "credito proposto",
                }
            ],
            "blocks": [{"start": v_start, "end": len(raw), "type": "estrofe"}],
        }

        violations = verify(source, result)
        self.assertTrue(any("conflito" in v.lower() or "credito" in v.lower() for v in violations))

    def test_credits_negative_discarding_melodia_autor_fails(self):
        # Negative test: raw_text has Melodia: Autor Conhecido, but result omits it
        raw = "Melodia: Autor Conhecido\nVerso do hino"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-c-neg1",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        v_start = raw.index("Verso do hino")
        result = {
            "source_id": "src-c-neg1",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 1,
            "titulo": "Hino",
            "letra": "Verso do hino",
            "creditos": None,  # Discarded!
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [
                {
                    "start": 0,
                    "end": v_start,
                    "type": "credits",
                    "evidence": "credito descartado",
                }
            ],
            "blocks": [{"start": v_start, "end": len(raw), "type": "estrofe"}],
        }

        violations = verify(source, result)
        self.assertTrue(any("melodia" in v.lower() and "discarded" in v.lower() for v in violations))

    def test_credits_negative_inventing_surnames_fails(self):
        # Negative test: Keith and Kristyn Getty expanded to Keith Getty | Kristyn Getty
        raw = "Letra e Música: Keith and Kristyn Getty\nVerso do hino"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-c-neg2",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        v_start = raw.index("Verso do hino")
        result = {
            "source_id": "src-c-neg2",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 1,
            "titulo": "Hino",
            "letra": "Verso do hino",
            "creditos": "Keith Getty | Kristyn Getty",  # Invented surname!
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [
                {
                    "start": 0,
                    "end": v_start,
                    "type": "credits",
                    "evidence": "autoria",
                }
            ],
            "blocks": [{"start": v_start, "end": len(raw), "type": "estrofe"}],
        }

        violations = verify(source, result)
        self.assertTrue(any("invented surname" in v.lower() for v in violations))

    def test_credits_concatenated_authorship_vs_biblical_ref(self):
        # Raw text contains both biblical reference and composer
        raw = "Texto: Salmo 23 | Música: João da Silva\nVerso do hino"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-c-bib",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        v_start = raw.index("Verso do hino")

        # Case 1: Result sets credits only to biblical reference, omitting composer -> fails
        result_omitted_composer = {
            "source_id": "src-c-bib",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 1,
            "titulo": "Hino",
            "letra": "Verso do hino",
            "creditos": "Salmo 23",
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [
                {
                    "start": 0,
                    "end": v_start,
                    "type": "credits",
                    "evidence": "creditos",
                }
            ],
            "blocks": [{"start": v_start, "end": len(raw), "type": "estrofe"}],
        }
        violations1 = verify(source, result_omitted_composer)
        self.assertTrue(any("joão da silva" in v.lower() or "omitted" in v.lower() for v in violations1))

        # Case 2: Result preserves composer -> passes
        result_valid = {
            "source_id": "src-c-bib",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 1,
            "titulo": "Hino",
            "letra": "Verso do hino",
            "creditos": "Salmo 23 | João da Silva",
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [
                {
                    "start": 0,
                    "end": v_start,
                    "type": "credits",
                    "evidence": "creditos",
                }
            ],
            "blocks": [{"start": v_start, "end": len(raw), "type": "estrofe"}],
        }
        violations2 = verify(source, result_valid)
        self.assertEqual(violations2, [])

    def test_credits_partial_ambiguous_doubt(self):
        # Ambiguous credits without author resolution must fail state: 'OK'
        raw = "Trad. e Adapt.\nVerso do hino"
        src_sha = hashlib.sha256(raw.encode("utf-8")).hexdigest()
        source = {
            "source_id": "src-c-amb",
            "source_sha256": src_sha,
            "raw_text": raw,
            "state": "candidate",
            "creditos_destino": None,
        }

        v_start = raw.index("Verso do hino")
        result = {
            "source_id": "src-c-amb",
            "source_sha256": src_sha,
            "state": "OK",
            "codigo": "CC",
            "numero": 1,
            "titulo": "Hino",
            "letra": "Verso do hino",
            "creditos": "Trad. e Adapt.",
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["ok"],
            },
            "operations": [
                {
                    "start": 0,
                    "end": v_start,
                    "type": "credits",
                    "evidence": "creditos",
                }
            ],
            "blocks": [{"start": v_start, "end": len(raw), "type": "estrofe"}],
        }

        violations = verify(source, result)
        self.assertTrue(any("ambiguous" in v.lower() or "doubt" in v.lower() for v in violations))


def create_mock_etl_executable(path: Path) -> Path:
    """Creates a synthetic executable script implementing -normalizar-stdin."""
    code = (
        "#!/usr/bin/env python3\n"
        "import sys\n"
        "if '-normalizar-stdin' in sys.argv:\n"
        "    data = sys.stdin.read()\n"
        "    lines = []\n"
        "    for line in data.replace('\\r\\n', '\\n').replace('\\r', '\\n').split('\\n'):\n"
        "        indent = len(line) - len(line.lstrip(' '))\n"
        "        content = line[indent:]\n"
        "        if content:\n"
        "            titled = content.title()\n"
        "            lines.append(' ' * indent + titled)\n"
        "        else:\n"
        "            lines.append('')\n"
        "    sys.stdout.write('\\n'.join(lines))\n"
        "    sys.exit(0)\n"
        "sys.exit(1)\n"
    )
    path.write_text(code, encoding="utf-8")
    path.chmod(0o755)
    return path


def write_test_csv(path: Path, rows: list[dict[str, str]]) -> None:
    """Writes a synthetic CSV file for campaign testing."""
    fieldnames = [
        "pasta",
        "arquivo",
        "hinario",
        "numero",
        "titulo",
        "letra",
        "status",
        "observacao",
        "creditos",
    ]
    with path.open("w", encoding="utf-8-sig", newline="") as f:
        import csv
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        for r in rows:
            writer.writerow(r)


class TestCampaignChunking(unittest.TestCase):
    """Tests chunking of candidate groups and handling of siblings and large groups."""

    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name)
        self.db_path = self.root / "snapshot.db"
        create_synthetic_snapshot(self.db_path)
        self.etl_path = create_mock_etl_executable(self.root / "mock_etl")

    def tearDown(self):
        self.temp_dir.cleanup()

    def test_group_chunking_23_groups(self):
        """23 groups partition into chunks of 10, 10, and 3 groups."""
        rows = []
        for i in range(1, 24):
            rows.append({
                "pasta": "slides",
                "arquivo": f"CC_{i}.ppt",
                "hinario": "CC",
                "numero": str(i),
                "titulo": f"Hino {i}",
                "letra": f"Letra do hino {i}\nLinha dois",
                "status": "PENDENTE",
                "observacao": "",
                "creditos": "Autor",
            })
        csv_path = self.root / "sources.csv"
        write_test_csv(csv_path, rows)
        run_dir = self.root / "run_23"

        prepare(csv_path, self.db_path, run_dir, self.etl_path)

        run_json = json.loads((run_dir / "run.json").read_text(encoding="utf-8"))
        chunks = run_json["chunks"]
        self.assertEqual(len(chunks), 3)
        self.assertEqual(len(chunks[0]["group_ids"]), 10)
        self.assertEqual(len(chunks[1]["group_ids"]), 10)
        self.assertEqual(len(chunks[2]["group_ids"]), 3)

    def test_group_chunking_large_group_dedicated_chunk(self):
        """A single group with >10 sources gets its own dedicated chunk."""
        rows = [
            # Group 1: 2 sources
            {"pasta": "s", "arquivo": "CC_1a.ppt", "hinario": "CC", "numero": "1", "titulo": "H1", "letra": "L1", "status": "PENDENTE", "observacao": "", "creditos": ""},
            {"pasta": "s", "arquivo": "CC_1b.ppt", "hinario": "CC", "numero": "1", "titulo": "H1", "letra": "L1", "status": "PENDENTE", "observacao": "", "creditos": ""},
        ]
        # Group 2: 12 sources (>10)
        for j in range(12):
            rows.append({
                "pasta": "s", "arquivo": f"CC_2_{j}.ppt", "hinario": "CC", "numero": "2", "titulo": "H2", "letra": "L2", "status": "PENDENTE", "observacao": "", "creditos": "",
            })
        # Group 3: 1 source
        rows.append({"pasta": "s", "arquivo": "CC_3.ppt", "hinario": "CC", "numero": "3", "titulo": "H3", "letra": "L3", "status": "PENDENTE", "observacao": "", "creditos": ""})

        csv_path = self.root / "sources_large.csv"
        write_test_csv(csv_path, rows)
        run_dir = self.root / "run_large"

        prepare(csv_path, self.db_path, run_dir, self.etl_path)

        run_json = json.loads((run_dir / "run.json").read_text(encoding="utf-8"))
        chunks = run_json["chunks"]
        self.assertEqual(len(chunks), 3)
        # Chunk 0 has CC/1
        self.assertEqual(chunks[0]["group_ids"], ["CC/1"])
        # Chunk 1 is dedicated to CC/2 with all 12 sources
        self.assertEqual(chunks[1]["group_ids"], ["CC/2"])
        self.assertEqual(len(chunks[1]["source_ids"]), 12)
        # Chunk 2 has CC/3
        self.assertEqual(chunks[2]["group_ids"], ["CC/3"])

    def test_group_chunking_distant_siblings_never_split(self):
        """Distant siblings in the CSV sharing a business key are never split across chunks."""
        rows = [
            # Row 0: CC/1
            {"pasta": "s", "arquivo": "CC_1a.ppt", "hinario": "CC", "numero": "1", "titulo": "H1", "letra": "L1", "status": "PENDENTE", "observacao": "", "creditos": ""},
        ]
        # Rows 1..10: CC/2 to CC/11
        for i in range(2, 12):
            rows.append({"pasta": "s", "arquivo": f"CC_{i}.ppt", "hinario": "CC", "numero": str(i), "titulo": f"H{i}", "letra": f"L{i}", "status": "PENDENTE", "observacao": "", "creditos": ""})
        # Row 11: CC/1 distant sibling
        rows.append({"pasta": "s", "arquivo": "CC_1b.ppt", "hinario": "CC", "numero": "1", "titulo": "H1", "letra": "L1", "status": "IGNORAR", "observacao": "alt", "creditos": ""})

        csv_path = self.root / "sources_distant.csv"
        write_test_csv(csv_path, rows)
        run_dir = self.root / "run_distant"

        prepare(csv_path, self.db_path, run_dir, self.etl_path)

        run_json = json.loads((run_dir / "run.json").read_text(encoding="utf-8"))
        chunks = run_json["chunks"]

        # Find which chunk has CC/1
        c0 = [c for c in chunks if "CC/1" in c["group_ids"]][0]
        # Both sources for CC/1 must be in c0
        self.assertEqual(len([sid for sid in c0["source_ids"]]), 11)  # 2 for CC/1 + 9 other groups in chunk 0
        self.assertIn("CC/1", c0["group_ids"])
        # CC/1 is not in any other chunk
        for other in chunks:
            if other["id"] != c0["id"]:
                self.assertNotIn("CC/1", other["group_ids"])


class TestCampaignLifecycle(unittest.TestCase):
    """Tests campaign lifecycle: prepare, next, accept, assemble, and error recovery."""

    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name)
        self.db_path = self.root / "snapshot.db"
        create_synthetic_snapshot(self.db_path)
        self.etl_path = create_mock_etl_executable(self.root / "mock_etl")

    def tearDown(self):
        self.temp_dir.cleanup()

    def _create_sample_campaign(self, num_groups: int = 15) -> tuple[Path, Path]:
        rows = []
        for i in range(1, num_groups + 1):
            raw = f"Titulo {i}\nPrimeiro verso {i}\nSegundo verso {i}"
            rows.append({
                "pasta": "slides",
                "arquivo": f"CC_{i}.ppt",
                "hinario": "CC",
                "numero": str(i),
                "titulo": f"Titulo {i}",
                "letra": raw,
                "status": "PENDENTE",
                "observacao": "",
                "creditos": f"Autor {i}",
            })
        csv_path = self.root / "sources.csv"
        write_test_csv(csv_path, rows)
        run_dir = self.root / "run"
        prepare(csv_path, self.db_path, run_dir, self.etl_path)
        return csv_path, run_dir

    def _build_valid_result(self, source: dict) -> dict:
        raw = source["raw_text"]
        title_line = raw.splitlines()[0]
        v_start = raw.index(raw.splitlines()[1])
        return {
            "source_id": source["source_id"],
            "source_sha256": source["source_sha256"],
            "state": "OK",
            "codigo": source["codigo"],
            "numero": source["numero"],
            "titulo": source["titulo_destino"] or source["titulo_fonte"],
            "letra": raw[v_start:],
            "creditos": source["creditos_destino"] or "Autor",
            "observacao": "",
            "reasons": [],
            "identity_decision": {
                "state": "confirmed",
                "origin": "exact",
                "evidence": ["valid"],
            },
            "operations": [
                {
                    "start": 0,
                    "end": v_start,
                    "type": "title",
                    "evidence": "cover line",
                }
            ],
            "blocks": [
                {
                    "start": v_start,
                    "end": len(raw),
                    "type": "estrofe",
                }
            ],
        }

    def test_resume_rejects_snapshot_sidecar(self):
        _, run_dir = self._create_sample_campaign(1)
        Path(str(self.db_path) + "-wal").write_bytes(b"unexpected WAL")
        with self.assertRaises(ValueError):
            get_next(run_dir)

    def test_resumption_after_partial_accept(self):
        """Interruption after 1st accept and resumption preserves results and advances cursor."""
        _, run_dir = self._create_sample_campaign(15)

        # 1. Get first chunk
        chunk_0_info = get_next(run_dir)
        self.assertEqual(chunk_0_info["chunk_id"], 0)
        self.assertEqual(len(chunk_0_info["groups"]), 10)

        # 2. Worker builds valid results for chunk 0
        results = [self._build_valid_result(s) for s in chunk_0_info["sources"]]
        chunk_0_file = run_dir / "chunk-0.json"
        chunk_0_file.write_text(json.dumps({"version": 1, "chunk_id": 0, "results": results}), encoding="utf-8")

        accept(run_dir, chunk_0_file)

        # 3. Resume / get next chunk: should advance to chunk 1
        chunk_1_info = get_next(run_dir)
        self.assertEqual(chunk_1_info["chunk_id"], 1)
        self.assertEqual(len(chunk_1_info["groups"]), 5)

    def test_accept_idempotent_identical_and_blocked_on_different(self):
        """Identical repeat accept is a no-op; different result for completed chunk is blocked."""
        _, run_dir = self._create_sample_campaign(5)
        chunk_0_info = get_next(run_dir)
        results = [self._build_valid_result(s) for s in chunk_0_info["sources"]]
        chunk_file = run_dir / "chunk-0.json"
        chunk_file.write_text(json.dumps({"version": 1, "chunk_id": 0, "results": results}), encoding="utf-8")

        # First accept
        accept(run_dir, chunk_file)

        # Identical repeat accept is a clean no-op
        accept(run_dir, chunk_file)

        # Modified result for same completed chunk must be blocked
        modified_results = list(results)
        modified_results[0] = dict(modified_results[0])
        modified_results[0]["observacao"] = "tampered observacao"
        modified_file = run_dir / "chunk-0-mod.json"
        modified_file.write_text(json.dumps({"version": 1, "chunk_id": 0, "results": modified_results}), encoding="utf-8")

        with self.assertRaises((RuntimeError, ValueError)):
            accept(run_dir, modified_file)

    def test_accept_validates_source_ids(self):
        """Omitted, invented, or duplicated source_ids block accept."""
        _, run_dir = self._create_sample_campaign(5)
        chunk_0_info = get_next(run_dir)
        results = [self._build_valid_result(s) for s in chunk_0_info["sources"]]

        # 1. Omitted source_id
        omitted = results[:-1]
        f_omitted = run_dir / "chunk-omitted.json"
        f_omitted.write_text(json.dumps({"version": 1, "chunk_id": 0, "results": omitted}), encoding="utf-8")
        with self.assertRaises(ValueError):
            accept(run_dir, f_omitted)

        # 2. Invented source_id
        invented = list(results) + [dict(results[0], source_id="invented-src-id")]
        f_invented = run_dir / "chunk-invented.json"
        f_invented.write_text(json.dumps({"version": 1, "chunk_id": 0, "results": invented}), encoding="utf-8")
        with self.assertRaises(ValueError):
            accept(run_dir, f_invented)

        # 3. Duplicated source_id
        duplicated = list(results[:-1]) + [results[0]]
        f_dup = run_dir / "chunk-dup.json"
        f_dup.write_text(json.dumps({"version": 1, "chunk_id": 0, "results": duplicated}), encoding="utf-8")
        with self.assertRaises(ValueError):
            accept(run_dir, f_dup)

    def test_accept_multiple_ok_for_same_key_fails(self):
        """More than 1 OK for the same destination key fails before assemble."""
        rows = [
            {"pasta": "s", "arquivo": "CC_1a.ppt", "hinario": "CC", "numero": "1", "titulo": "H1", "letra": "L1\nVerso A", "status": "PENDENTE", "observacao": "", "creditos": ""},
            {"pasta": "s", "arquivo": "CC_1b.ppt", "hinario": "CC", "numero": "1", "titulo": "H1", "letra": "L1\nVerso B", "status": "PENDENTE", "observacao": "", "creditos": ""},
        ]
        csv_path = self.root / "sources_dup_key.csv"
        write_test_csv(csv_path, rows)
        run_dir = self.root / "run_dup_key"
        prepare(csv_path, self.db_path, run_dir, self.etl_path)

        chunk_info = get_next(run_dir)
        # Marking BOTH sibling sources as OK must fail!
        res_0 = self._build_valid_result(chunk_info["sources"][0])
        res_1 = self._build_valid_result(chunk_info["sources"][1])

        chunk_file = run_dir / "chunk-dup-ok.json"
        chunk_file.write_text(json.dumps({"version": 1, "chunk_id": 0, "results": [res_0, res_1]}), encoding="utf-8")

        with self.assertRaises(ValueError):
            accept(run_dir, chunk_file)

    def test_accept_canonicalizes_credit(self):
        """Empty or ASCII-whitespace-only proposed credit becomes None in receipt."""
        _, run_dir = self._create_sample_campaign(1)
        chunk_info = get_next(run_dir)
        res = self._build_valid_result(chunk_info["sources"][0])
        res["creditos"] = "   \t\n  "  # only ascii ws

        chunk_file = run_dir / "chunk-cred.json"
        chunk_file.write_text(json.dumps({"version": 1, "chunk_id": 0, "results": [res]}), encoding="utf-8")
        accept(run_dir, chunk_file)

        partial_path = run_dir / "partial-0.json"
        self.assertTrue(partial_path.exists())
        partial_data = json.loads(partial_path.read_text(encoding="utf-8"))
        receipt = partial_data["receipts"][0]
        self.assertIsNone(receipt["creditos"])
        # Verify final_sha256 was computed with None
        expected_hash = compute_final_sha256(
            receipt["codigo"],
            receipt["numero"],
            receipt["titulo"],
            receipt["letra"],
            None,
        )
        self.assertEqual(receipt["final_sha256"], expected_hash)

    def test_crash_before_rename_does_not_corrupt(self):
        """A crash before atomic rename does not corrupt run state."""
        _, run_dir = self._create_sample_campaign(5)
        # Create a stale temp file as if write crashed before replace
        stale_tmp = run_dir / "partial-0.tmp"
        stale_tmp.write_text("corrupted partial data", encoding="utf-8")

        # get_next should reconcile cleanly without failing
        chunk_info = get_next(run_dir)
        self.assertEqual(chunk_info["chunk_id"], 0)

    def test_reconciliation_recovers_uncommitted_run_json(self):
        """Reconciliation recovers cursor if partial was published but run.json was uncommitted."""
        _, run_dir = self._create_sample_campaign(15)
        chunk_0_info = get_next(run_dir)
        results = [self._build_valid_result(s) for s in chunk_0_info["sources"]]
        chunk_file = run_dir / "chunk-0.json"
        chunk_file.write_text(json.dumps({"version": 1, "chunk_id": 0, "results": results}), encoding="utf-8")

        accept(run_dir, chunk_file)

        # Simulate crash: revert chunk 0 state to "pending" in run.json
        run_json_path = run_dir / "run.json"
        run_data = json.loads(run_json_path.read_text(encoding="utf-8"))
        run_data["chunks"][0]["state"] = "pending"
        run_json_path.write_text(json.dumps(run_data), encoding="utf-8")

        # Next call should reconcile from partial-0.json, advance cursor to chunk 1, without a new worker
        chunk_next = get_next(run_dir)
        self.assertEqual(chunk_next["chunk_id"], 1)

    def test_assemble_generates_stable_outputs(self):
        """Assemble requires completed chunks, generates sanitized.csv, duvidas.csv, provenance.jsonl, relatorio.md."""
        rows = [
            # 1. OK candidate
            {"pasta": "s", "arquivo": "CC_1.ppt", "hinario": "CC", "numero": "1", "titulo": "H1", "letra": "T1\nV1", "status": "PENDENTE", "observacao": "", "creditos": "Autor"},
            # 2. Doubt candidate
            {"pasta": "s", "arquivo": "CC_2.ppt", "hinario": "CC", "numero": "2", "titulo": "H2", "letra": "T2\nV2", "status": "PENDENTE", "observacao": "", "creditos": ""},
            # 3. Skip candidate
            {"pasta": "s", "arquivo": "CC_3.ppt", "hinario": "CC", "numero": "3", "titulo": "H3", "letra": "T3\nV3", "status": "IGNORAR", "observacao": "descartar", "creditos": ""},
        ]
        csv_path = self.root / "sources_3.csv"
        write_test_csv(csv_path, rows)
        run_dir = self.root / "run_assemble"
        prepare(csv_path, self.db_path, run_dir, self.etl_path)

        chunk_info = get_next(run_dir)
        srcs = chunk_info["sources"]

        # 1. OK
        res_ok = self._build_valid_result(srcs[0])
        # 2. Doubt
        res_doubt = {
            "source_id": srcs[1]["source_id"],
            "source_sha256": srcs[1]["source_sha256"],
            "state": "doubt",
            "codigo": srcs[1]["codigo"],
            "numero": srcs[1]["numero"],
            "titulo": srcs[1]["titulo_fonte"],
            "letra": srcs[1]["raw_text"],
            "creditos": None,
            "observacao": "ambiguidade",
            "reasons": ["texto duvidoso"],
            "identity_decision": {"state": "unresolved", "origin": "none"},
            "operations": [],
            "blocks": [],
        }
        # 3. Skip
        res_skip = {
            "source_id": srcs[2]["source_id"],
            "source_sha256": srcs[2]["source_sha256"],
            "state": "skip",
            "codigo": srcs[2]["codigo"],
            "numero": srcs[2]["numero"],
            "titulo": srcs[2]["titulo_fonte"],
            "letra": srcs[2]["raw_text"],
            "creditos": None,
            "observacao": "status inicial IGNORAR",
            "reasons": ["ignorado"],
            "identity_decision": {"state": "not_applicable", "origin": "none"},
            "operations": [],
            "blocks": [],
        }

        chunk_file = run_dir / "chunk-all.json"
        chunk_file.write_text(json.dumps({"version": 1, "chunk_id": 0, "results": [res_ok, res_doubt, res_skip]}), encoding="utf-8")
        accept(run_dir, chunk_file)

        # Assemble
        assemble(run_dir)

        # Verify sanitized.csv
        sanitized_csv = run_dir / "sanitized.csv"
        self.assertTrue(sanitized_csv.exists())
        with sanitized_csv.open("r", encoding="utf-8") as f:
            rows_sanitized = list(csv.reader(f))
        self.assertEqual(len(rows_sanitized), 2)  # header + 1 OK row
        header = rows_sanitized[0]
        self.assertEqual(
            header,
            ["pasta", "arquivo", "hinario", "numero", "titulo", "letra", "status", "observacao", "creditos", "source_id", "source_sha256"],
        )
        self.assertEqual(rows_sanitized[1][1], "CC_1.ppt")

        # Verify duvidas.csv
        duvidas_csv = run_dir / "duvidas.csv"
        self.assertTrue(duvidas_csv.exists())
        with duvidas_csv.open("r", encoding="utf-8") as f:
            rows_duvidas = list(csv.reader(f))
        self.assertEqual(len(rows_duvidas), 3)  # header + 2 non-OK rows

        # Verify provenance.jsonl
        prov_file = run_dir / "provenance.jsonl"
        self.assertTrue(prov_file.exists())
        prov_lines = [json.loads(ln) for ln in prov_file.read_text(encoding="utf-8").splitlines() if ln.strip()]
        self.assertEqual(len(prov_lines), 3)

        # Verify relatorio.md
        relatorio_file = run_dir / "relatorio.md"
        self.assertTrue(relatorio_file.exists())
        rel_text = relatorio_file.read_text(encoding="utf-8")
        self.assertIn("# Relatório de Sanitização", rel_text)
        self.assertIn("Total de fontes processadas", rel_text)

        # Verify no result files outside run_dir
        for p in self.root.iterdir():
            if p != run_dir and p.name not in ["snapshot.db", "mock_etl", "sources_3.csv"]:
                self.fail(f"unexpected file outside run_dir: {p}")

    def test_modifying_fields_after_normalization_invalidates_receipt(self):
        """Modifying credit, title, key, or lyrics case invalidates HashFinal receipt."""
        codigo, numero, titulo, letra, creditos = "CC", 42, "Antífona", "Santo, Santo!", "Autor"
        valid_hash = compute_final_sha256(codigo, numero, titulo, letra, creditos)

        # 1. Modify credit
        self.assertNotEqual(valid_hash, compute_final_sha256(codigo, numero, titulo, letra, "Outro Autor"))
        # 2. Modify title
        self.assertNotEqual(valid_hash, compute_final_sha256(codigo, numero, "Outro Titulo", letra, creditos))
        # 3. Modify key
        self.assertNotEqual(valid_hash, compute_final_sha256("HCC", numero, titulo, letra, creditos))
        self.assertNotEqual(valid_hash, compute_final_sha256(codigo, 43, titulo, letra, creditos))
        # 4. Modify case
        self.assertNotEqual(valid_hash, compute_final_sha256(codigo, numero, titulo, letra.upper(), creditos))


class TestCampaignIntegrityAndLocks(unittest.TestCase):
    """Tests file integrity verification, advisory lock, and crash resilience."""

    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name)
        self.db_path = self.root / "snapshot.db"
        create_synthetic_snapshot(self.db_path)
        self.etl_path = create_mock_etl_executable(self.root / "mock_etl")

    def tearDown(self):
        self.temp_dir.cleanup()

    def test_double_writer_mutual_exclusion(self):
        """Two concurrent writers fail with exclusive lock."""
        run_dir = self.root / "run_lock"
        run_dir.mkdir(parents=True)

        with CampaignLock(run_dir):
            # Second attempt while lock is held must raise
            with self.assertRaises(RuntimeError):
                with CampaignLock(run_dir):
                    pass

    def test_sigkill_releases_os_lock(self):
        """SIGKILL on locking process immediately releases OS flock."""
        import subprocess
        run_dir = self.root / "run_sigkill"
        run_dir.mkdir(parents=True)
        lock_file = run_dir / ".lock"

        # Spawn child process holding flock
        child_cmd = [
            sys.executable,
            "-c",
            f"import fcntl, sys, time; f = open('{lock_file}', 'a+'); fcntl.flock(f, fcntl.LOCK_EX | fcntl.LOCK_NB); sys.stdout.write('LOCKED\\n'); sys.stdout.flush(); time.sleep(60)",
        ]
        proc = subprocess.Popen(child_cmd, stdout=subprocess.PIPE, text=True)
        try:
            line = proc.stdout.readline()
            self.assertEqual(line.strip(), "LOCKED")

            # Verify parent cannot lock while child holds it
            with self.assertRaises(RuntimeError):
                with CampaignLock(run_dir):
                    pass

            # Send SIGKILL to child
            proc.kill()
            proc.wait()

            # Now parent can lock immediately
            with CampaignLock(run_dir):
                pass
        finally:
            if proc.stdout:
                proc.stdout.close()
            if proc.poll() is None:
                proc.kill()
                proc.wait()

    def test_file_tampering_or_absence_prevents_resumption(self):
        """Tampered or missing CSV, snapshot, ETL, script, or skill files prevent resumption."""
        rows = [{"pasta": "s", "arquivo": "CC_1.ppt", "hinario": "CC", "numero": "1", "titulo": "H1", "letra": "L1", "status": "PENDENTE", "observacao": "", "creditos": ""}]
        csv_path = self.root / "sources.csv"
        write_test_csv(csv_path, rows)
        run_dir = self.root / "run_tamper"
        prepare(csv_path, self.db_path, run_dir, self.etl_path)

        # 1. Tampered CSV
        csv_path.write_text("tampered csv content", encoding="utf-8")
        with self.assertRaises(RuntimeError):
            get_next(run_dir)
        write_test_csv(csv_path, rows)  # restore

        # 2. Tampered snapshot
        self.db_path.write_bytes(b"tampered snapshot")
        with self.assertRaises(RuntimeError):
            get_next(run_dir)
        self.db_path.unlink()
        create_synthetic_snapshot(self.db_path)  # restore

        # 3. Tampered ETL
        self.etl_path.write_bytes(b"tampered etl")
        with self.assertRaises(RuntimeError):
            get_next(run_dir)
        create_mock_etl_executable(self.etl_path)  # restore

        # 4. Missing CSV
        csv_path.unlink()
        with self.assertRaises(RuntimeError):
            get_next(run_dir)
        write_test_csv(csv_path, rows)  # restore

    def test_cli_invocation_with_spaces_in_paths(self):
        """CLI invocation works cleanly with spaces in paths."""
        space_dir = self.root / "dir with spaces"
        space_dir.mkdir(parents=True)
        csv_path = space_dir / "my sources.csv"
        rows = [{"pasta": "s", "arquivo": "CC_1.ppt", "hinario": "CC", "numero": "1", "titulo": "H1", "letra": "T1\nV1", "status": "PENDENTE", "observacao": "", "creditos": "Autor"}]
        write_test_csv(csv_path, rows)
        run_dir = space_dir / "my run output"
        etl_path = create_mock_etl_executable(space_dir / "my mock etl")

        skill_script = SKILL_ROOT / "scripts" / "campaign.py"

        # prepare
        import subprocess
        res = subprocess.run(
            [sys.executable, str(skill_script), "prepare", "--csv", str(csv_path), "--snapshot", str(self.db_path), "--out", str(run_dir), "--etl", str(etl_path)],
            capture_output=True,
            text=True,
        )
        self.assertEqual(res.returncode, 0, msg=res.stderr)

        # next
        res = subprocess.run(
            [sys.executable, str(skill_script), "next", "--out", str(run_dir)],
            capture_output=True,
            text=True,
        )
        self.assertEqual(res.returncode, 0, msg=res.stderr)
        chunk_data = json.loads(res.stdout)
        self.assertEqual(chunk_data["chunk_id"], 0)

        # accept
        res_item = {
            "source_id": chunk_data["sources"][0]["source_id"],
            "source_sha256": chunk_data["sources"][0]["source_sha256"],
            "state": "OK",
            "codigo": "CC",
            "numero": 1,
            "titulo": "H1",
            "letra": "V1",
            "creditos": "Autor",
            "observacao": "",
            "reasons": [],
            "identity_decision": {"state": "confirmed", "origin": "exact", "evidence": ["ok"]},
            "operations": [{"start": 0, "end": 3, "type": "title", "evidence": "cover"}],
            "blocks": [{"start": 3, "end": len(chunk_data["sources"][0]["raw_text"]), "type": "estrofe"}],
        }
        chunk_file = space_dir / "my chunk.json"
        chunk_file.write_text(json.dumps({"version": 1, "chunk_id": 0, "results": [res_item]}), encoding="utf-8")

        res = subprocess.run(
            [sys.executable, str(skill_script), "accept", "--out", str(run_dir), "--chunk", str(chunk_file)],
            capture_output=True,
            text=True,
        )
        self.assertEqual(res.returncode, 0, msg=res.stderr)

        # assemble
        res = subprocess.run(
            [sys.executable, str(skill_script), "assemble", "--out", str(run_dir)],
            capture_output=True,
            text=True,
        )
        self.assertEqual(res.returncode, 0, msg=res.stderr)
        self.assertTrue((run_dir / "sanitized.csv").exists())


if __name__ == "__main__":
    unittest.main()
