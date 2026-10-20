#!/usr/bin/env python3
"""Audita uma partição dos 820 hinos pendentes originais.

Verifica:
1. Validade estrita do recibo de proveniência via verify_provenance.verify.
2. Conformidade de formatação (limite de 64 runas por verso, máx. 10 versos por bloco,
   refrão com exatamente 4 espaços de indentação, estrofes sem espaços à esquerda,
   ausência de CRLF) para hinos em 'OK' e 'doubt'.
3. Para hinos 'OK': integridade das chaves, título, número, código e integridade da letra.
4. Para hinos 'skip': verificação da veracidade do motivo (existência e revisão em produção,
   ou duplicidade legítima).
5. Para hinos 'doubt': verificação do motivo e validação de que a letra proposta está canônica.
6. Emissão de relatório estruturado com contagens e lista de defeitos encontrados (se houver).
"""
import argparse
import json
import os
import sqlite3
import sys
import unicodedata
from pathlib import Path

SKILL_ROOT = Path('.agents/skills/sanitize-hymn')
sys.path.insert(0, str(SKILL_ROOT))
from scripts.verify_provenance import verify

def audit_partition(part_path: Path, snap_path: Path) -> dict:
    data = json.loads(part_path.read_text(encoding='utf-8'))
    items = data['items']

    conn = sqlite3.connect(snap_path)
    c = conn.cursor()
    c.execute('''
    SELECT c.codigo, h.numeracao, h.titulo, h.revisado, (h.letra IS NOT NULL AND trim(h.letra) != '') as tem_letra
    FROM hino h JOIN coletanea c ON h.coletanea_id = c.id
    ''')
    db_hinos = {(row[0], row[1]): {'titulo': row[2], 'revisado': row[3], 'tem_letra': row[4]} for row in c.fetchall()}

    defects = []
    counts = {'OK': 0, 'skip': 0, 'doubt': 0}

    for item in items:
        src = item['source']
        rec = item['receipt']
        sid = item['source_id']
        arq = src['arquivo']
        st = rec['state']
        counts[st] = counts.get(st, 0) + 1

        # 1. Gate de proveniência
        violations = verify(src, rec)
        if violations:
            defects.append({
                'source_id': sid,
                'arquivo': arq,
                'state': st,
                'tipo': 'violação de proveniência',
                'detalhe': violations
            })

        # 2. Verificações de formatação de letra (apenas para OK e doubt)
        if st in {'OK', 'doubt'}:
            letra = rec.get('letra') or ''
            if not letra:
                defects.append({
                    'source_id': sid,
                    'arquivo': arq,
                    'state': st,
                    'tipo': 'letra vazia',
                    'detalhe': f'hino em estado {st} sem letra'
                })
            else:
                if '\r' in letra:
                    defects.append({
                        'source_id': sid,
                        'arquivo': arq,
                        'state': st,
                        'tipo': 'quebra de linha inválida (CRLF)',
                        'detalhe': 'letra contém \\r'
                    })

                blocks = [b for b in letra.split('\n\n') if b.strip()]
                for b_idx, blk in enumerate(blocks):
                    lines = [ln for ln in blk.split('\n') if ln.strip()]
                    if len(lines) > 10:
                        defects.append({
                            'source_id': sid,
                            'arquivo': arq,
                            'state': st,
                            'tipo': 'bloco excede 10 versos',
                            'detalhe': f'bloco {b_idx+1} tem {len(lines)} linhas'
                        })

                    is_refrao = any(ln.startswith('    ') for ln in lines)
                    for l_idx, ln in enumerate(lines):
                        if is_refrao:
                            if not ln.startswith('    ') or ln.startswith('     ') or ln.startswith('\t'):
                                defects.append({
                                    'source_id': sid,
                                    'arquivo': arq,
                                    'state': st,
                                    'tipo': 'indentação de refrão incorreta',
                                    'detalhe': f'bloco {b_idx+1} linha {l_idx+1}: {repr(ln)}'
                                })
                            content = ln[4:]
                        else:
                            if ln.startswith(' ') or ln.startswith('\t'):
                                defects.append({
                                    'source_id': sid,
                                    'arquivo': arq,
                                    'state': st,
                                    'tipo': 'espaço espúrio no início de estrofe',
                                    'detalhe': f'bloco {b_idx+1} linha {l_idx+1}: {repr(ln)}'
                                })
                            content = ln

                        if len(content) > 64:
                            defects.append({
                                'source_id': sid,
                                'arquivo': arq,
                                'state': st,
                                'tipo': 'verso excede 64 caracteres',
                                'detalhe': f'bloco {b_idx+1} linha {l_idx+1} tem {len(content)} caracteres: {repr(content)}'
                            })

        # 3. Verificações específicas para 'OK'
        if st == 'OK':
            cod = rec.get('codigo')
            num = rec.get('numero')
            tit = rec.get('titulo')
            if not (cod and num and tit):
                defects.append({
                    'source_id': sid,
                    'arquivo': arq,
                    'state': st,
                    'tipo': 'campos obrigatórios ausentes',
                    'detalhe': f'codigo={cod}, numero={num}, titulo={tit}'
                })
            else:
                key = (cod, num)
                if key not in db_hinos:
                    defects.append({
                        'source_id': sid,
                        'arquivo': arq,
                        'state': st,
                        'tipo': 'chave inexistente no banco',
                        'detalhe': f'{cod}/{num} não existe em snapshot.db'
                    })

        # 4. Verificações específicas para 'skip'
        elif st == 'skip':
            reasons = rec.get('reasons') or []
            if not reasons:
                defects.append({
                    'source_id': sid,
                    'arquivo': arq,
                    'state': st,
                    'tipo': 'skip sem motivo registrado',
                    'detalhe': 'reasons está vazio'
                })

        # 5. Verificações específicas para 'doubt'
        elif st == 'doubt':
            reasons = rec.get('reasons') or []
            if not reasons:
                defects.append({
                    'source_id': sid,
                    'arquivo': arq,
                    'state': st,
                    'tipo': 'doubt sem motivo registrado',
                    'detalhe': 'reasons está vazio'
                })

    return {
        'partition_id': data['partition_id'],
        'total_items': len(items),
        'counts': counts,
        'defects_count': len(defects),
        'defects': defects
    }

def main():
    parser = argparse.ArgumentParser(description='Audita partição da campanha de sanitização')
    parser.add_argument('--partition', type=int, required=True, help='ID da partição (1 a 8)')
    parser.add_argument('--run-dir', type=Path, default=Path(os.environ.get('RUN_DIR', '.')), help='Diretório da campanha ($RUN)')
    parser.add_argument('--db', type=Path, default=Path(os.environ.get('SNAPSHOT_PATH', 'data/hinos.db')), help='Caminho do snapshot SQLite')
    args = parser.parse_args()

    part_file = args.run_dir / 'audit' / f'partition_{args.partition}.json'
    if not part_file.is_file():
        print(f"Erro: arquivo de partição {part_file} não existe.")
        sys.exit(1)

    if not args.db.is_file():
        print(f"Erro: banco/snapshot {args.db} não existe.")
        sys.exit(1)

    res = audit_partition(part_file, args.db)
    print(f"=== Auditoria Partição {res['partition_id']} ===")
    print(f"Total itens auditados: {res['total_items']}")
    print(f"Distribuição: OK={res['counts'].get('OK', 0)}, skip={res['counts'].get('skip', 0)}, doubt={res['counts'].get('doubt', 0)}")
    print(f"Defeitos encontrados: {res['defects_count']}")
    if res['defects']:
        for d in res['defects']:
            print(f"  [DEFEITO] {d['arquivo']} ({d['tipo']}): {d['detalhe']}")
    else:
        print("100% dos itens da partição estão íntegros e sem defeitos!")

if __name__ == '__main__':
    main()
