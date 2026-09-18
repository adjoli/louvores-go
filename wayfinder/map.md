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
- Contexto do chat (18/09/2026): Adão revisa semanalmente o que tem letra e copia/cola o que não tem a partir dos slides antigos; IA anterior gerou letra mal formatada e não confiável; faltam ~300 letras, sobretudo HL/HCC/corinhos; mutirão de revisão com Jônatas/Gabriel/louvor comparando com o hinário.

## Decisions so far

<!-- índice: uma linha por ticket fechado; detalhe vive no ticket -->

- (nenhuma ainda — mapa recém-cartografado)

## Not yet specified

<!-- fog: dentro do escopo, mas ainda sem nitidez para virar ticket -->

- Volume total real no Drive/NOVA IDENTIDADE (quantos PPT/PPT por coletânea, duplicatas, quantos já têm letra no banco vs erro de extração vs nunca cantados) — depende do [Inventário do corpus legado](tickets/01-inventario-corpus.md).
- Se LLM entra em alguma etapa (ex. só classificar refrão candidato, nunca gerar letra do zero) e com qual guardrail — amadurece após [Heurística de refrão (detecção)](tickets/04-heuristica-refrao.md) + [Junção, armazenamento e papel do LLM](tickets/09-juncao-armazenamento-llm.md).
- Quem revisa o quê e em qual UI (planilha, arquivo local temporário, página web temporária) — amadurece após [Formato intermediário de revisão](tickets/05-formato-revisao.md).
- Comando final de importação (`sql` em batch, `csv`, `go run ./cmd/importar`?) e política upsert (pular existentes com letra? sobrescrever só `revisado=false`?) — amadurece após [Desenho da pipeline ETL](tickets/06-desenho-pipeline.md).

## Out of scope

- Refatoração grande para `hinos/<hinario>/*.md` + git como fonte da verdade com refrão não repetido (sugestão overkill do Mateus em 18/09 — reconhecida como overkill no próprio chat; requer refazer lógica de slides, auth/git para todos, override de última hora). Mantém-se o banco como fonte.
- Reescrever a geração de PPTX, trocar de template, ou mudar layouts 1/2/3 do `default.pptx`.
- Busca por palavra-chave/tema para o louvor (feature desejada, esforço separado).
- Migração Render+Turso → VPS, backup automático, ou versionamento das letras em código.
- Direitos autorais/repositório privado (só volta se aDestination for redesenhada para versionar letras em git).
