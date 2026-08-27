package ppt

// Índices dos layouts do template default.pptx (baseados na estrutura real do
// arquivo). O gerador depende destes valores: NÃO alterar sem ajustar o template.
//
//	1 = TÍTULO  — ctrTitle + subTitle
//	2 = ESTROFE — title + body (idx=1) + rodapé (body sz=quarter, idx=10)
//	3 = REFRAO  — title + body (idx=1) + rodapé (body sz=quarter, idx=10)
const (
	LayoutTitulo  = 1
	LayoutEstrofe = 2
	LayoutRefrao  = 3
)
