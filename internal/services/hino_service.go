package services

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sort"
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

// HinoUpdate carrega os campos editáveis de um hino enviados pelo formulário
// web. Numeração e coletânea são a chave da rota e não podem ser alterados.
type HinoUpdate struct {
	Titulo   string
	Letra    string
	Creditos string
	Revisado bool
}

// AtualizarHino persiste as alterações de um hino (título, letra, créditos e
// revisão). A letra é convertida para Title Case antes de salvar, evitando
// texto todo em minúsculas ou maiúsculas.
//
// Revisão é irreversível: se o hino já está revisado, o campo Revisado é
// mantido como true mesmo que o formulário o envie desmarcado.
// Retorna repository.ErrColetaneaNotFound ou repository.ErrHinoNotFound.
func (s *HinoService) AtualizarHino(
	ctx context.Context,
	codigoColetanea string,
	numero int,
	upd HinoUpdate,
) (*models.Hino, error) {
	coletanea, err := s.coletaneaRepo.FindByCodigo(ctx, codigoColetanea)
	if err != nil {
		return nil, err
	}

	hino, err := s.hinoRepo.FindByNumero(ctx, coletanea.ID, numero)
	if err != nil {
		return nil, err
	}

	// A letra é normalizada para Title Case antes de persistir.
	upd.Letra = titularLetra(upd.Letra)

	hino.Titulo = upd.Titulo
	hino.Letra = &upd.Letra
	hino.Creditos = &upd.Creditos
	if !hino.Revisado {
		hino.Revisado = upd.Revisado
	}

	if err := s.hinoRepo.Update(ctx, hino); err != nil {
		return nil, err
	}

	return hino, nil
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
	if err != nil {
		return nil, err
	}

	_, data, err := s.geraSlidesHino(ctx, coletanea, hino, templatePath)
	return data, err
}

// ArquivoGerado é um PPTX gerado para o lote, com o nome de saída e o conteúdo.
type ArquivoGerado struct {
	Nome     string
	Conteudo []byte
}

// LoteResultado é o resultado da geração em lote: o ZIP empacotado e a
// contagem de hinos gerados e pulados (não revisados, sem letra ou com erro).
type LoteResultado struct {
	Zip     []byte
	Gerados int
	Pulados int
}

// GerarSlidesColetanea gera os slides de todos os hinos revisados (e com
// letra) de uma coletânea e os empacota em um único ZIP, um PPTX por hino.
//
// Hinos não revisados, sem letra ou sem numeração são pulados e contados em
// Pulados; erros individuais são logados e não interrompem o lote.
func (s *HinoService) GerarSlidesColetanea(
	ctx context.Context,
	codigoColetanea string,
	templatePath string,
) (*LoteResultado, error) {
	coletanea, err := s.coletaneaRepo.FindByCodigo(ctx, codigoColetanea)
	if err != nil {
		return nil, err
	}

	hinos, err := s.hinoRepo.ListByColetanea(ctx, coletanea.ID)
	if err != nil {
		return nil, err
	}

	var arquivos []ArquivoGerado
	pulados := 0

	for i := range hinos {
		hino := &hinos[i]
		if !hino.Revisado || hino.Letra == nil || hino.Numeracao == nil {
			pulados++
			continue
		}

		filename, data, err := s.geraSlidesHino(ctx, coletanea, hino, templatePath)
		if err != nil {
			slog.Error("falha ao gerar slides no lote",
				"coletanea", coletanea.Codigo,
				"numero", *hino.Numeracao,
				"erro", err)
			pulados++
			continue
		}
		arquivos = append(arquivos, ArquivoGerado{Nome: filename, Conteudo: data})
	}

	zipData, err := zipArquivos(arquivos)
	if err != nil {
		return nil, fmt.Errorf("empacotar lote: %w", err)
	}

	return &LoteResultado{
		Zip:     zipData,
		Gerados: len(arquivos),
		Pulados: pulados,
	}, nil
}

// geraSlidesHino gera o PPTX de um hino já carregado com a coletânea,
// retornando o nome do arquivo e o conteúdo. Concentra o nome de saída, o
// parse da letra e a formatação por coletânea, para o path único e o lote
// compartilharem a mesma lógica.
func (s *HinoService) geraSlidesHino(
	ctx context.Context,
	coletanea *models.Coletanea,
	hino *models.Hino,
	templatePath string,
) (string, []byte, error) {
	if !hino.Revisado {
		return "", nil, ErrHinoNotReviewed
	}
	if hino.Numeracao == nil {
		return "", nil, fmt.Errorf("hino sem numeração")
	}

	letra := ""
	if hino.Letra != nil {
		letra = *hino.Letra
	}
	seq := processors.ProcessarHino(letra)

	titulo, subtitulo, tituloSlides := textosSlides(*coletanea, *hino)
	data, err := ppt.GenerateSlides(titulo, subtitulo, tituloSlides, seq, templatePath)
	if err != nil {
		return "", nil, err
	}

	return nomeArquivoSlides(coletanea.Codigo, *hino.Numeracao, hino.Titulo), data, nil
}

// nomeArquivoSlides monta o nome de saída de um PPTX: {CODIGO}-{NUM:03d}-{TITULO}.pptx
// (título em maiúsculas, espaços viram sublinhado).
func nomeArquivoSlides(codigo string, numero int, titulo string) string {
	return fmt.Sprintf("%s-%03d-%s.pptx",
		codigo,
		numero,
		strings.ToUpper(strings.ReplaceAll(titulo, " ", "_")),
	)
}

// zipArquivos empacota os arquivos em um ZIP em memória, ordenados pelo nome
// para uma saída determinística.
func zipArquivos(arquivos []ArquivoGerado) ([]byte, error) {
	sort.Slice(arquivos, func(i, j int) bool { return arquivos[i].Nome < arquivos[j].Nome })

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, a := range arquivos {
		w, err := zw.Create(a.Nome)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(a.Conteudo); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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

// titularLetra converte a letra do hino para Title Case antes de salvar,
// evitando que o texto fique todo em minúsculas ou todo em maiúsculas.
//
// A conversão é aplicada palavra a palavra apenas quando a linha está
// uniformemente em minúsculas ou em maiúsculas; texto já com caixa mista é
// preservado (não estraga nomes já corretamente capitalizados). A indentação
// inicial é mantida, pois é usada para detectar refrões. Linhas em branco
// (separadores de blocos) são preservadas.
func titularLetra(s string) string {
	// Normaliza quebras de linha para LF antes de processar: o textarea do
	// formulário pode enviar CRLF, e o "\r" restante contaminaria o Title Case
	// e a separação de blocos (linha em branco extra nos slides).
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if trimmed == strings.ToLower(trimmed) || trimmed == strings.ToUpper(trimmed) {
			lines[i] = titularLinha(line)
		}
	}
	return strings.Join(lines, "\n")
}

// titularLinha aplica Title Case a uma linha, preservando a indentação
// inicial (espaços/tabs usados para marcar refrões).
func titularLinha(line string) string {
	// Separa a indentação da primeira palavra.
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]

	words := strings.Fields(trimmed)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return indent + strings.Join(words, " ")
}
