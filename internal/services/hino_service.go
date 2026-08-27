package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/adjoli/louvores-go/internal/models"
	"github.com/adjoli/louvores-go/internal/ppt"
	"github.com/adjoli/louvores-go/internal/processors"
)

// ErrHinoNotReviewed é retornado quando se tenta gerar slides para um hino não revisado.
var ErrHinoNotReviewed = &hinoNotReviewedError{}

type hinoNotReviewedError struct{}

func (e *hinoNotReviewedError) Error() string {
	return "hino não revisado: slides só podem ser gerados para hinos revisados"
}

// HinoService concentra as operações de leitura sobre coletâneas e hinos.
//
// Ele resolve a navegação pela chave de negócio (código da coletânea +
// numeração do hino, ex.: CC/42), traduzindo-a para os IDs internos usados
// pelo repositório. Os erros sentinela do repositório
// (ErrColetaneaNotFound, ErrHinoNotFound) propagam
// sem alteração para que a camada superior os mapeie para respostas HTTP.
type HinoService struct {
	hinoRepo      HinoRepository
	coletaneaRepo ColetaneaRepository
	templatePath  string
}

// NewHinoService cria um novo HinoService com os repositórios fornecidos.
func NewHinoService(
	hinoRepo HinoRepository,
	coletaneaRepo ColetaneaRepository,
	templatePath string,
) *HinoService {
	return &HinoService{
		hinoRepo:      hinoRepo,
		coletaneaRepo: coletaneaRepo,
		templatePath:  templatePath,
	}
}

// TemplatePath retorna o caminho do template PPTX.
func (s *HinoService) TemplatePath() string {
	return s.templatePath
}

// ListarColetaneas retorna todas as coletâneas cadastradas.
func (s *HinoService) ListarColetaneas(
	ctx context.Context,
) ([]models.Coletanea, error) {
	return s.coletaneaRepo.List(ctx)
}

// ListarHinos retorna os hinos da coletânea identificada pelo código curto
// (ex.: "CC"), ordenados pela numeração.
// Retorna repository.ErrColetaneaNotFound se o código não existir.
func (s *HinoService) ListarHinos(
	ctx context.Context,
	codigoColetanea string,
) ([]models.Hino, error) {
	coletanea, err := s.coletaneaRepo.FindByCodigo(ctx, codigoColetanea)
	if err != nil {
		return nil, err
	}

	return s.hinoRepo.ListByColetanea(ctx, coletanea.ID)
}

// ObterHino retorna o hino identificado pela combinação código da coletânea
// + numeração (ex.: CC/42).
// Retorna repository.ErrColetaneaNotFound ou repository.ErrHinoNotFound.
func (s *HinoService) ObterHino(
	ctx context.Context,
	codigoColetanea string,
	numero int,
) (*models.Hino, error) {
	coletanea, err := s.coletaneaRepo.FindByCodigo(ctx, codigoColetanea)
	if err != nil {
		return nil, err
	}

	return s.hinoRepo.FindByNumero(ctx, coletanea.ID, numero)
}

// GerarSlides gera o arquivo PPTX com os slides do hino.
// Apenas hinos revisados (Revisado=true) podem ter slides gerados.
// Retorna ErrHinoNotReviewed se o hino não for revisado.
// Retorna repository.ErrColetaneaNotFound ou repository.ErrHinoNotFound se não encontrado.
func (s *HinoService) GerarSlides(
	ctx context.Context,
	codigoColetanea string,
	numero int,
	templatePath string,
) ([]byte, error) {
	coletanea, err := s.coletaneaRepo.FindByCodigo(ctx, codigoColetanea)
	if err != nil {
		return nil, err
	}

	hino, err := s.hinoRepo.FindByNumero(ctx, coletanea.ID, numero)
	if !hino.Revisado {
		return nil, ErrHinoNotReviewed
	}

	letra := ""
	if hino.Letra != nil {
		letra = *hino.Letra
	}
	seq := processors.ProcessarHino(letra)

	titulo, subtitulo, tituloSlides := textosSlides(*coletanea, *hino)
	return ppt.GenerateSlides(titulo, subtitulo, tituloSlides, seq, templatePath)
}

// codigoCorinhos é o código curto da coletânea "Corinhos". Para essa coletânea
// o subtítulo do slide de título fica vazio e os slides de conteúdo mantêm o
// título original (sem prefixo da coletânea).
const codigoCorinhos = "COR"

// textosSlides define o que é exibido nos slides: o título do primeiro slide,
// o texto abaixo dele (subtítulo) e o título dos slides de conteúdo.
//
//   - Para coletâneas que não são Corinhos, o subtítulo é "Nome da Coletânea - Número"
//     e os slides de conteúdo levam o prefixo "NÚMERO{CÓDIGO} - Título".
//   - Para Corinhos, o subtítulo fica vazio e os slides de conteúdo mantêm o
//     título original, sem prefixo.
func textosSlides(c models.Coletanea, h models.Hino) (titulo, subtitulo, tituloSlides string) {
	numero := 0
	if h.Numeracao != nil {
		numero = *h.Numeracao
	}

	subtitulo = ""
	tituloSlides = h.Titulo
	if c.Codigo != codigoCorinhos {
		subtitulo = fmt.Sprintf("%s - %d", titleCase(c.Titulo), numero)
		tituloSlides = fmt.Sprintf("%d%s - %s", numero, c.Codigo, h.Titulo)
	}
	return h.Titulo, subtitulo, tituloSlides
}

// titleCase capitaliza a primeira letra de cada palavra, deixando o restante
// em minúsculas. Para evitar estragar nomes já corretamente capitalizados
// (ex.: "Hinário para o Culto Cristão"), a conversão só é aplicada quando o
// texto está totalmente em maiúsculas.
func titleCase(s string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}

	// Se houver alguma minúscula, o texto já tem caixa definida: preserva.
	if s != strings.ToUpper(s) {
		return s
	}

	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}
