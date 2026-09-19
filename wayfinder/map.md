# Mapa — ETL de importação dos hinos legados (PPT/PPTX → SQLite prod)

> Tracker local (`.md`). Nomes são a identidade — nunca referencie por número nu.
> Evidências em `wayfinder/evidencia/` (10 PPTs + 2 exports do WhatsApp).

## Destination

Uma **pipeline ETL reproduzível** que extrai as letras dos PPTs/PPTs legados,
normaliza para o formato esperado pelo `louvores-go`, passa por uma **etapa
intermediária de revisão humana**, e gera um **artefato pronto para importação
em lote** no SQLite/Turso de produção — sem alterar a lógica de geração de
slides. O mapa termina quando o caminho está decidido (formato, heurísticas,
ferramentas, importação segura), pronto para execução.

## Notes

- Domínio: `louvores-go` (Go ≥ 1.25, `net/http`, `templ`, SQLite `modernc.org/sqlite` / Turso libSQL, PPTX OOXML próprio). Não alterar `~/Projetos/louvores` (Python original).
- Skills que toda sessão deve consultar: `grilling` + `domain-modeling` (decisões), `research` (tickets AFK), `prototype` (desenho da pipeline).
- Glossário canônico (de `internal/`): **coletânea** (código curto CC/HL/VM/HCC/COR), **hino** (chave `CODIGO/numero`, ex. CC/42), **letra** (texto bruto com blocos separados por linha em branco `\n\s*\n`), **estrofe** (bloco sem indentação) vs **refrão** (todas as linhas com espaço/tab inicial, indentação removida), **parte/slide** (`domain.SequenciaHino`, `Numero` 1-based, rodapé `N/total`), **revisado** (bool irreversível — só gera slide se `true`), **Title Case** (`titularLetra`, preserva indentação e caixa mista, normaliza CRLF→LF), **template** único `default.pptx` (layouts 1=TITULO, 2=ESTROFE, 3=REFRAO — não alterar).
- Preferências standing: importação é **em lote idempotente** (não edição manual no SQLite); backup manual do Turso free (só 24h) antes de qualquer escrita; `data/hinos.db`, `output/`, `logs/`, `.env`, dumps fora do git; arquivos `_templ.go` commitados (irrelevante aqui); fonte da verdade continua o **banco** (não migrar para `hinos/*.md` neste esforço).
- Decisão assentada (Mateus, 09/2026): a ETL é uma **camada separada e simples** (CLI ou script que processa uma pasta inteira de `pptx`/`ppt` e entrega o artefato pronto para importar) — **zero mudanças no codebase atual de geração de slides** (`internal/ppt`, `processors`, `services`, template). A ETL só lê slides legados e escreve o formato de revisão/importação.
- Contexto do chat (18/09/2026): Adão revisa semanalmente o que tem letra e copia/cola o que não tem a partir dos slides antigos; IA anterior gerou letra mal formatada e não confiável; estimava-se ~300,
  mas o banco local tem **1187 sem letra** (HCC 429, VM 400, CC 151,
  COR 141, HL 66); mutirão de revisão com Jônatas/Gabriel/louvor comparando com o hinário.
- Perfil dos revisores (Mateus, 09/2026): **não usam git/GitHub nem terminal** — o artefato de revisão tem que ser planilha (ou equivalente familiar); CLI/TUI é **ferramenta exclusiva do operador** (Mateus/Adão).

## Decisions so far

<!-- índice: uma linha por ticket fechado; detalhe vive no ticket -->

- [Inventário do corpus legado](tickets/01-inventario-corpus.md): 2282 slides catalogados em `corpus.csv`, 695/1187 chaves sem letra com fonte, mtime mais novo vence, nada descartado
- [Formato esperado pelo banco](tickets/02-formato-esperado.md): contrato verificado no código (incl. unitário vs lote com letra nil)
- [Padrões reais dos PPTs legados](tickets/03-padroes-ppts.md): raio-X das 8 amostras + resumo transversal
- [Extração dos .ppt binários (OLE)](tickets/08-extracao-ppt-ole.md): LibreOffice headless, 650/650 convertidos, texto em `corpus/txt/`, update levado ao Drive via `revisao-ole-update.csv`

## Not yet specified

<!-- fog: vazio — tudo que era névoa graduou em ticket (09/2026, 3 reviews).
     Se nova névoa surgir ao resolver a fronteira, registra aqui. -->

## Out of scope

- Refatoração grande para `hinos/<hinario>/*.md` + git como fonte da verdade com refrão não repetido (sugestão overkill do Mateus em 18/09 — reconhecida como overkill no próprio chat; requer refazer lógica de slides, auth/git para todos, override de última hora). Mantém-se o banco como fonte.
- Reescrever a geração de PPTX, trocar de template, ou mudar layouts 1/2/3 do `default.pptx`.
- Busca por palavra-chave/tema para o louvor (feature desejada, esforço separado).
- Migração Render+Turso → VPS, backup automático, ou versionamento das letras em código.
- Direitos autorais/repositório privado (só volta se aDestination for redesenhada para versionar letras em git).
