package models

// Os tipos deste arquivo espelham as tabelas do banco e formam o modelo
// persistente do domínio (entidades). Campos opcionais usam ponteiro/nil
// para expressar ausência real no banco — ex.: Hino.Letra nil = "hino sem
// letra", que é uma condição de negócio verificada pela camada de serviços.
//
// As tags json definem o contrato público da API (snake_case): campos
// ponteiro sem valor são serializados como null explícito, nunca omitidos.

// Coletanea é uma coleção de hinos (ex.: um hinário), identificada por um
// código curto usado em comandos e no nome dos arquivos gerados.
type Coletanea struct {
	ID     int64  `json:"id"`
	Codigo string `json:"codigo"`
	Titulo string `json:"titulo"`
}

// Hino é um cântico pertencente a uma coletânea.
//
// Numeracao, Letra e Creditos são opcionais (ponteiro). ColetaneaID referencia
// a coletânea pai; a navegação pública combina o código dela com a numeração
// (ex.: CC/42) — este ID interno nunca aparece nas rotas. Revisado indica que
// a letra passou por revisão humana e pode ser usada para gerar slides.
type Hino struct {
	ID          int64   `json:"id"`
	ColetaneaID int64   `json:"coletanea_id"`
	Numeracao   *int    `json:"numeracao"`
	Titulo      string  `json:"titulo"`
	Letra       *string `json:"letra"`
	Creditos    *string `json:"creditos"`
	Revisado    bool    `json:"revisado"`
}
