package processors

import (
	"regexp"
	"strings"

	"github.com/adjoli/louvores-go/internal/domain"
)

// blocoRegex separa a letra em blocos lógicos.
// O padrão \n\s*\n significa: quebra de linha, seguida de zero ou mais
// espaços/tabs (linhas "vazias" com whitespace), seguida de outra quebra de linha.
// Isso captura tanto parágrafos separados por linha em branco pura ("\n\n")
// quanto por linhas com espaços/tabs ("\n  \n", "\n\t\n").
// O Split com -1 preserva todos os blocos (incluindo possíveis vazios no final,
// embora strings.Trim remova os das pontas antes do split).
var blocoRegex = regexp.MustCompile(`\n\s*\n`)

// ProcessarHino divide o texto bruto de um hino em uma sequência de partes
// (estrofes e refrões), cada uma com seu número de slide sequencial.
//
// Regras de parsing:
//   - A letra é dividida em blocos por linhas em branco (regex blocoRegex).
//   - Linhas vazias ou só com whitespace dentro de um bloco são ignoradas.
//   - Um bloco é classificado como REFRÃO se TODAS as suas linhas começam
//     com espaço (' ') OU tab ('\t'). Qualquer linha sem indentação torna
//     o bloco uma ESTROFE.
//   - Para refrões, a indentação inicial é removida de cada linha (TrimLeft
//     em " \t"), preservando quebras de linha internas.
//   - Estrofes mantêm o texto original (sem remoção de indentação).
//   - Cada parte recebe Numero sequencial (1-based) para ordenação dos slides.
//
// Retorna domain.SequenciaHino contendo todas as partes em ordem.
func ProcessarHino(texto string) domain.SequenciaHino {
	// Normaliza quebras de linha: formulários (textarea) e importações podem
	// trazer CRLF ("\r\n") ou CR ("\r") isolado. Sem essa normalização, cada
	// linha ficaria terminando em "\r", o que gera uma linha em branco extra
	// ao renderizar os slides. A ordem importa: primeiro "\r\n", depois "\r"
	// isolado, para não duplicar.
	texto = normalizeNewlines(texto)

	// Remove quebras de linha das pontas para evitar blocos vazios no split
	blocos := blocoRegex.Split(strings.Trim(texto, "\n"), -1)

	seq := domain.SequenciaHino{}
	numeroSlide := 1

	for _, bloco := range blocos {
		// Filtra linhas que não são apenas whitespace dentro do bloco
		linhas := linhasNaoVazias(bloco)
		if len(linhas) == 0 {
			continue
		}

		tipo := domain.TipoParteEstrofe
		conteudo := strings.Join(linhas, "\n")
		if ehRefrao(linhas) {
			tipo = domain.TipoParteRefrao
			// Refrão: remove indentação inicial (espaço/tab) de cada linha
			conteudo = joinLTrim(linhas)
		}

		seq.Partes = append(seq.Partes, domain.ParteHino{
			Txt:    conteudo,
			Numero: numeroSlide,
			Tipo:   tipo,
		})
		numeroSlide++
	}

	return seq
}

// ehRefrao verifica se um bloco é um refrão.
// Regra: TODAS as linhas devem começar com espaço (' ') OU tab ('\t').
// Se pelo menos uma linha não tem indentação, o bloco é estrofe.
// Esta é uma verificação estrita (AND sobre todas as linhas).
func ehRefrao(linhas []string) bool {
	for _, linha := range linhas {
		if !strings.HasPrefix(linha, " ") && !strings.HasPrefix(linha, "\t") {
			return false
		}
	}
	return true
}

// linhasNaoVazias divide um bloco em linhas e descarta as que contêm
// apenas whitespace (espaços, tabs, quebras de linha).
// Mantém a indentação original das linhas não vazias (não faz Trim).
func linhasNaoVazias(bloco string) []string {
	var out []string
	for _, linha := range strings.Split(bloco, "\n") {
		if strings.TrimSpace(linha) != "" {
			out = append(out, linha)
		}
	}
	return out
}

// joinLTrim remove a indentação inicial (espaços e tabs) de cada linha
// e as junta com '\n'.
// Usa TrimLeft com cutset " \t" para remover APENAS leading whitespace,
// preservando qualquer indentação interna das linhas.
// Exemplo: "  linha 1\n\tlinha 2" -> "linha 1\nlinha 2"
func joinLTrim(linhas []string) string {
	limpas := make([]string, 0, len(linhas))
	for _, linha := range linhas {
		limpas = append(limpas, strings.TrimLeft(linha, " \t"))
	}
	return strings.Join(limpas, "\n")
}

// normalizeNewlines converte todas as quebras de linha para "\n" (LF).
// Entradas comuns são CRLF ("\r\n", do Windows/textarea) e CR isolado ("\r",
// macOS antigo). A ordem de substituição importa: tratar "\r\n" antes do "\r"
// isolado evita criar "\n\n" a partir de um único "\r\n".
func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}
