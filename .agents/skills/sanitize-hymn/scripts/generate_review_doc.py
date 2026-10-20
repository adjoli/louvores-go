#!/usr/bin/env python3
"""Gera artefatos de revisão manual para hinos em dúvida (ex.: corinhos ou avulsos sem chave oficial).

Converte a lista de itens pendentes/dúvidas em um documento interativo em Markdown
e uma planilha CSV de decisão para o operador.
Não depende de diretórios locais de corpus nem caminhos fixos do sistema.
"""
import argparse
import csv
import sys
from pathlib import Path

def main():
    parser = argparse.ArgumentParser(description="Gera documento de revisão manual e CSV de decisão para hinos em dúvida")
    parser.add_argument("--csv", type=Path, required=True, help="Caminho do arquivo CSV de entrada (ex.: duvidas.csv)")
    parser.add_argument("--md", type=Path, required=True, help="Caminho do arquivo Markdown de saída")
    parser.add_argument("--out-csv", type=Path, required=True, help="Caminho da planilha CSV de decisão de saída")
    args = parser.parse_args()

    if not args.csv.is_file():
        print(f"Erro: arquivo de entrada {args.csv} não encontrado.", file=sys.stderr)
        sys.exit(1)

    with args.csv.open(encoding="utf-8") as f:
        rows = list(csv.DictReader(f))

    if not rows:
        print(f"Aviso: nenhum registro encontrado em {args.csv}.")

    items = []
    for idx, r in enumerate(rows, 1):
        arq = r.get("arquivo") or r.get("nome_arquivo") or f"item_{idx}"
        pasta = r.get("pasta") or r.get("pasta_origem") or ""
        tit = r.get("titulo") or r.get("titulo_proposto") or Path(arq).stem
        letra = r.get("letra") or r.get("letra_formatada") or r.get("raw_text") or ""
        motivo = r.get("reasons") or r.get("observacao") or r.get("diagnostico") or "Avaliação pendente"

        items.append({
            "ordem": idx,
            "pasta": pasta,
            "arquivo": arq,
            "titulo": tit,
            "motivo": motivo,
            "letra": letra,
        })

    # 1. Gerar CSV de decisão
    args.out_csv.parent.mkdir(parents=True, exist_ok=True)
    header = [
        "ordem",
        "pasta",
        "arquivo",
        "titulo_proposto",
        "motivo",
        "decisao_sugerida",
        "decisao_operador",
        "letra",
    ]
    with args.out_csv.open("w", encoding="utf-8", newline="") as f:
        w = csv.writer(f)
        w.writerow(header)
        for it in items:
            sugestao = "DESCARTAR" if any(w in it["motivo"].lower() for w in ["descarte", "duplicata", "ignorar"]) else "MANTER"
            w.writerow([
                it["ordem"],
                it["pasta"],
                it["arquivo"],
                it["titulo"],
                it["motivo"],
                sugestao,
                "",  # Preenchimento pelo operador
                it["letra"],
            ])

    # 2. Gerar Markdown de revisão
    args.md.parent.mkdir(parents=True, exist_ok=True)
    md_lines = [
        "# Revisão Manual de Hinos e Cânticos em Dúvida",
        "",
        f"Este documento reúne os **{len(items)} itens** retidos em dúvida para conferência humana pelo operador.",
        "",
        "---",
        "",
        "## Índice para Revisão",
        "",
    ]

    for it in items:
        md_lines.append(f"{it['ordem']}. **{it['titulo']}** (`{it['arquivo']}`) — *Motivo*: {it['motivo']}")

    md_lines.append("")
    md_lines.append("---")
    md_lines.append("")

    for it in items:
        sugestao = "Descartar" if any(w in it["motivo"].lower() for w in ["descarte", "duplicata", "ignorar"]) else "Manter"
        default_manter = "x" if sugestao == "Manter" else " "
        default_descartar = "x" if sugestao == "Descartar" else " "

        md_lines.extend([
            f"### {it['ordem']}. {it['titulo']}",
            "",
            f"- **Arquivo**: `{it['arquivo']}`",
            f"- **Pasta**: `{it['pasta']}`" if it["pasta"] else "",
            f"- **Motivo da Dúvida**: {it['motivo']}",
            f"- **Sugestão**: `{sugestao}`",
            f"- **Decisão**: [{default_manter}] Manter e cadastrar  |  [{default_descartar}] Descartar",
            "- **Hinário / Numeração**: _______________________________",
            "",
            "```text",
            it["letra"],
            "```",
            "",
            "---",
            "",
        ])

    args.md.write_text("\n".join(line for line in md_lines if line != ""), encoding="utf-8")
    print(f"Sucesso: gerados {args.out_csv} e {args.md} com {len(items)} itens.")

if __name__ == "__main__":
    main()
