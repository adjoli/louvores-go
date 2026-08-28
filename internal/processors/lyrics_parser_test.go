package processors

import (
	"testing"

	"github.com/adjoli/louvores-go/internal/domain"
)

func TestProcessarHinoEstrofe(t *testing.T) {
	seq := ProcessarHino("Linha um\nLinha dois\n\nLinha três")

	want := []domain.ParteHino{
		{Txt: "Linha um\nLinha dois", Numero: 1, Tipo: domain.TipoParteEstrofe},
		{Txt: "Linha três", Numero: 2, Tipo: domain.TipoParteEstrofe},
	}
	assertPartes(t, seq, want)
}

func TestProcessarHinoRefraoIndentado(t *testing.T) {
	seq := ProcessarHino("Estrofe\n\n    Refrão um\n    Refrão dois")

	want := []domain.ParteHino{
		{Txt: "Estrofe", Numero: 1, Tipo: domain.TipoParteEstrofe},
		{Txt: "Refrão um\nRefrão dois", Numero: 2, Tipo: domain.TipoParteRefrao},
	}
	assertPartes(t, seq, want)
}

func TestProcessarHinoRefraoComTab(t *testing.T) {
	seq := ProcessarHino("\tRefrão com tab\n\tsegunda linha")

	want := []domain.ParteHino{
		{Txt: "Refrão com tab\nsegunda linha", Numero: 1, Tipo: domain.TipoParteRefrao},
	}
	assertPartes(t, seq, want)
}

// TestProcessarHinoCRLF garante que quebras de linha CRLF (textarea do
// formulário) são normalizadas para LF, evitando linhas em branco extras.
func TestProcessarHinoCRLF(t *testing.T) {
	texto := "Estrofe um\r\n\r\n    Refrão santo\r\n    Segunda linha"
	seq := ProcessarHino(texto)

	want := []domain.ParteHino{
		{Txt: "Estrofe um", Numero: 1, Tipo: domain.TipoParteEstrofe},
		{Txt: "Refrão santo\nSegunda linha", Numero: 2, Tipo: domain.TipoParteRefrao},
	}
	assertPartes(t, seq, want)
}

// TestNormalizeNewlines cobre os três formatos de quebra de linha.
func TestNormalizeNewlines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "lf", in: "a\nb", want: "a\nb"},
		{name: "crlf", in: "a\r\nb", want: "a\nb"},
		{name: "cr", in: "a\rb", want: "a\nb"},
		{name: "misturado", in: "a\r\nb\rc\n\nd", want: "a\nb\nc\n\nd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeNewlines(tt.in); got != tt.want {
				t.Errorf("normalizeNewlines(%q) = %q, esperado %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestProcessarHinoMesclado(t *testing.T) {
	texto := "Estrofe um\n\n    Refrão\n    Refrão 2\n\nEstrofe dois\n\n\nEstrofe três"
	seq := ProcessarHino(texto)

	want := []domain.ParteHino{
		{Txt: "Estrofe um", Numero: 1, Tipo: domain.TipoParteEstrofe},
		{Txt: "Refrão\nRefrão 2", Numero: 2, Tipo: domain.TipoParteRefrao},
		{Txt: "Estrofe dois", Numero: 3, Tipo: domain.TipoParteEstrofe},
		{Txt: "Estrofe três", Numero: 4, Tipo: domain.TipoParteEstrofe},
	}
	assertPartes(t, seq, want)
}

func TestProcessarHinoVazio(t *testing.T) {
	if seq := ProcessarHino(""); len(seq.Partes) != 0 {
		t.Fatalf("esperado nenhuma parte, obtido %d", len(seq.Partes))
	}
	if seq := ProcessarHino("   \n\n  "); len(seq.Partes) != 0 {
		t.Fatalf("esperado nenhuma parte para texto em branco, obtido %d", len(seq.Partes))
	}
}

func TestRodape(t *testing.T) {
	p := domain.ParteHino{Numero: 2}
	if got, want := p.Rodape(5), "2/5"; got != want {
		t.Fatalf("rodape = %q, esperado %q", got, want)
	}
}

func assertPartes(t *testing.T, seq domain.SequenciaHino, want []domain.ParteHino) {
	t.Helper()
	if len(seq.Partes) != len(want) {
		t.Fatalf("len(partes) = %d, esperado %d\n%+v", len(seq.Partes), len(want), seq.Partes)
	}
	for i, w := range want {
		got := seq.Partes[i]
		if got.Txt != w.Txt || got.Numero != w.Numero || got.Tipo != w.Tipo {
			t.Errorf("parte[%d] = %+v, esperado %+v", i, got, w)
		}
	}
}
