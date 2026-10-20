package services

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCapitalizacaoUnicode(t *testing.T) {
	t.Run("TitularLetra", func(t *testing.T) {
		tests := []struct {
			name string
			in   string
			want string
		}{
			{
				name: "acentos unicode em maiúsculas",
				in:   "Ó À É ÁGUA",
				want: "Ó À É Água",
			},
			{
				name: "pontuação inicial e aspas",
				in:   `"VINDE “COM (BIS)`,
				want: `"Vinde “Com (Bis)`,
			},
			{
				name: "indentação CRLF e preservação de caixa mista",
				in:   "    Ó VEM\r\n\r\nJá em Caixa Mista",
				want: "    Ó Vem\n\nJá em Caixa Mista",
			},
			{
				name: "recuo com tabs intactos",
				in:   "\t\tÓ VEM SENHOR",
				want: "\t\tÓ Vem Senhor",
			},
			{
				name: "texto misto já capitalizado preservado",
				in:   "Grande É a Esperança, Ó Irmãos\n    Refrão Santo",
				want: "Grande É a Esperança, Ó Irmãos\n    Refrão Santo",
			},
			{
				name: "palavras inteiramente em minúsculas com acentos",
				in:   "ó à é água viva",
				want: "Ó À É Água Viva",
			},
			{
				name: "pontuação diversa e parênteses",
				in:   `[CORO] (REFRÃO) {FINAL} ‘AMÉM’`,
				want: `[Coro] (Refrão) {Final} ‘Amém’`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := TitularLetra(tt.in)
				if got != tt.want {
					t.Errorf("TitularLetra(%q) = %q, esperado %q", tt.in, got, tt.want)
				}
				if !utf8.ValidString(got) {
					t.Errorf("resultado não é UTF-8 válido: %q", got)
				}
				if strings.ContainsRune(got, '\uFFFD') {
					t.Errorf("resultado contém U+FFFD: %q", got)
				}
				// Idempotência
				gotAgain := TitularLetra(got)
				if gotAgain != got {
					t.Errorf("TitularLetra não é idempotente: TitularLetra(%q) = %q", got, gotAgain)
				}
			})
		}
	})

	t.Run("titleCase", func(t *testing.T) {
		tests := []struct {
			name string
			in   string
			want string
		}{
			{
				name: "todas em maiúsculas com acentos",
				in:   "ÁGUA E LUZ",
				want: "Água E Luz",
			},
			{
				name: "acentos isolados",
				in:   "Ó À É",
				want: "Ó À É",
			},
			{
				name: "pontuação inicial",
				in:   `“ÁGUA E LUZ”`,
				want: `“Água E Luz”`,
			},
			{
				name: "caixa mista preservada",
				in:   "Hinário para o Culto Cristão",
				want: "Hinário para o Culto Cristão",
			},
			{
				name: "string vazia",
				in:   "",
				want: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := titleCase(tt.in)
				if got != tt.want {
					t.Errorf("titleCase(%q) = %q, esperado %q", tt.in, got, tt.want)
				}
				if !utf8.ValidString(got) {
					t.Errorf("resultado não é UTF-8 válido: %q", got)
				}
				if strings.ContainsRune(got, '\uFFFD') {
					t.Errorf("resultado contém U+FFFD: %q", got)
				}
				// Idempotência
				gotAgain := titleCase(got)
				if gotAgain != got {
					t.Errorf("titleCase não é idempotente: titleCase(%q) = %q", got, gotAgain)
				}
			})
		}
	})
}

func TestCapitalizarPalavra(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "string vazia", in: "", want: ""},
		{name: "palavra simples maiúscula", in: "AMOR", want: "Amor"},
		{name: "palavra simples minúscula", in: "amor", want: "Amor"},
		{name: "palavra acentuada", in: "ÁGUA", want: "Água"},
		{name: "letra acentuada única maiúscula", in: "Ó", want: "Ó"},
		{name: "letra acentuada única minúscula", in: "ó", want: "Ó"},
		{name: "aspas retas iniciais", in: `"VINDE`, want: `"Vinde`},
		{name: "aspas curvas iniciais", in: "“COM", want: "“Com"},
		{name: "parênteses iniciais", in: "(BIS)", want: "(Bis)"},
		{name: "apóstrofo inicial", in: "'PRA", want: "'Pra"},
		{name: "múltipla pontuação inicial", in: "([“Ó”])", want: "([“Ó”])"},
		{name: "sem letras apenas símbolos", in: "---", want: "---"},
		{name: "apenas números", in: "123", want: "123"},
		{name: "apenas pontuação", in: "“", want: "“"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capitalizarPalavra(tt.in)
			if got != tt.want {
				t.Errorf("capitalizarPalavra(%q) = %q, esperado %q", tt.in, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("resultado não é UTF-8 válido: %q", got)
			}
			if strings.ContainsRune(got, '\uFFFD') {
				t.Errorf("resultado contém U+FFFD: %q", got)
			}
		})
	}
}
