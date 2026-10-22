package services

import (
	"strings"
	"unicode"
)

// capitalizarPalavra capitaliza a primeira letra de uma palavra (considerando
// caracteres Unicode e pontuação inicial), convertendo as demais letras para
// minúsculas. Pontuação ou símbolos que precedem a primeira letra são preservados.
func capitalizarPalavra(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(strings.ToLower(s))
	for i, r := range runes {
		if unicode.IsLetter(r) {
			runes[i] = unicode.ToUpper(r)
			break
		}
	}
	return string(runes)
}
