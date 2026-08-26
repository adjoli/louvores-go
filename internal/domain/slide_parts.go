package domain

import "fmt"

type TipoParte string

const (
	TipoParteEstrofe TipoParte = "estrofe"
	TipoParteRefrao  TipoParte = "refrao"
)

type ParteHino struct {
	Txt    string
	Numero int
	Tipo   TipoParte
}

func (p ParteHino) Rodape(numPartes int) string {
	return fmt.Sprintf("%d/%d", p.Numero, numPartes)
}

type SequenciaHino struct {
	Partes []ParteHino
}
