// Package api expõe a API REST da aplicação (somente leitura nesta fase).
//
// Os handlers são finos: interpretam a requisição (parâmetros de rota),
// delegam aos serviços e serializam o resultado como JSON. Erros de
// negócio são mapeados para códigos HTTP: não encontrado → 404,
// parâmetro inválido → 400, demais → 500 com log estruturado.
package api

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/adjoli/louvores-go/internal/repository"
	"github.com/adjoli/louvores-go/internal/services"
)

// openapiSpec contém a especificação OpenAPI da API, embutida no binário e
// servida crua em /api/openapi.yaml.
//
//go:embed openapi.yaml
var openapiSpec []byte

// docsHTML é a página do Swagger UI que carrega os assets via CDN e consome
// a especificação embutida.
//
//go:embed docs.html
var docsHTML []byte

// API é o container dos handlers REST, alimentado pelos serviços via DI.
type API struct {
	hinoSvc  *services.HinoService
	statsSvc *services.StatsService
}

// New cria a API com os serviços fornecidos.
func New(hinoSvc *services.HinoService, statsSvc *services.StatsService) *API {
	return &API{
		hinoSvc:  hinoSvc,
		statsSvc: statsSvc,
	}
}

// rota associa um padrão "METHOD /caminho/{param}" ao seu handler.
type rota struct {
	padrao  string
	handler func(http.ResponseWriter, *http.Request)
}

// rotas é a fonte única das rotas registradas — consumida por Routes() para
// montar o mux e pelos testes de paridade contra a spec OpenAPI.
func (a *API) rotas() []rota {
	return []rota{
		{padrao: "GET /api/healthz", handler: a.handleHealth},
		{padrao: "GET /api/coletaneas", handler: a.handleListarColetaneas},
		{padrao: "GET /api/coletaneas/{codigo}/hinos", handler: a.handleListarHinos},
		{padrao: "GET /api/coletaneas/{codigo}/hinos/{numero}", handler: a.handleObterHino},
		{padrao: "GET /api/coletaneas/{codigo}/hinos/{numero}/slides", handler: a.handleGerarSlides},
		{padrao: "GET /api/stats", handler: a.handleStats},
		{padrao: "GET /api/openapi.yaml", handler: a.handleOpenAPISpec},
		{padrao: "GET /api/docs", handler: a.handleDocs},
	}
}

// Routes monta e retorna o roteador HTTP com todos os endpoints da API.
// Usa o ServeMux da stdlib (Go 1.22+) com padrões "METHOD /caminho/{param}".
func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()

	for _, rota := range a.rotas() {
		mux.HandleFunc(rota.padrao, rota.handler)
	}

	return mux
}

// handleHealth responde liveness do serviço.
func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleListarColetaneas retorna todas as coletâneas cadastradas.
func (a *API) handleListarColetaneas(w http.ResponseWriter, r *http.Request) {
	coletaneas, err := a.hinoSvc.ListarColetaneas(r.Context())
	if err != nil {
		respondError(r, w, err)
		return
	}
	writeJSON(w, http.StatusOK, coletaneas)
}

// handleListarHinos retorna os hinos de uma coletânea, ordenados pela
// numeração. O código vem da rota: /api/coletaneas/{codigo}/hinos.
func (a *API) handleListarHinos(w http.ResponseWriter, r *http.Request) {
	codigo := r.PathValue("codigo")

	hinos, err := a.hinoSvc.ListarHinos(r.Context(), codigo)
	if err != nil {
		respondError(r, w, err)
		return
	}
	writeJSON(w, http.StatusOK, hinos)
}

// handleObterHino retorna os detalhes de um hino identificado pela chave de
// negócio código + número: /api/coletaneas/{codigo}/hinos/{numero}.
func (a *API) handleObterHino(w http.ResponseWriter, r *http.Request) {
	codigo := r.PathValue("codigo")

	numero, err := strconv.Atoi(r.PathValue("numero"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "número inválido",
		})
		return
	}

	hino, err := a.hinoSvc.ObterHino(r.Context(), codigo, numero)
	if err != nil {
		respondError(r, w, err)
		return
	}
	writeJSON(w, http.StatusOK, hino)
}

// handleGerarSlides gera e retorna o arquivo PPTX com os slides do hino.
// Apenas hinos revisados podem ter slides gerados.
// Retorna o arquivo PPTX como download (application/vnd.openxmlformats-officedocument.presentationml.presentation).
func (a *API) handleGerarSlides(w http.ResponseWriter, r *http.Request) {
	codigo := r.PathValue("codigo")

	numero, err := strconv.Atoi(r.PathValue("numero"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "número inválido",
		})
		return
	}

	// Obter o hino para pegar o título para o nome do arquivo
	hino, err := a.hinoSvc.ObterHino(r.Context(), codigo, numero)
	if err != nil {
		respondError(r, w, err)
		return
	}

	// Gerar slides (valida se está revisado internamente)
	pptxBytes, err := a.hinoSvc.GerarSlides(r.Context(), codigo, numero, a.hinoSvc.TemplatePath())
	if err != nil {
		if errors.Is(err, services.ErrHinoNotReviewed) {
			slog.Warn("gerar slides bloqueado: hino não revisado",
				"coletanea", codigo, "numero", numero)
		}
		respondError(r, w, err)
		return
	}

	// Nome do arquivo: {CODIGO}-{NUM:03d}-{TITULO}.pptx (título em maiúsculas)
	filename := fmt.Sprintf("%s-%03d-%s.pptx",
		codigo,
		numero,
		strings.ToUpper(strings.ReplaceAll(hino.Titulo, " ", "_")),
	)

	slog.Info("slides gerados",
		"coletanea", codigo,
		"numero", numero,
		"arquivo", filename,
		"tamanho", len(pptxBytes),
	)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(pptxBytes)))
	w.WriteHeader(http.StatusOK)
	w.Write(pptxBytes)
}

// handleStats retorna as estatísticas agregadas por coletânea.
func (a *API) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.statsSvc.ObterStats(r.Context())
	if err != nil {
		respondError(r, w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleOpenAPISpec serve a especificação OpenAPI embutida, em YAML cru.
func (a *API) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(openapiSpec); err != nil {
		slog.Error("servir especificação OpenAPI", "erro", err)
	}
}

// handleDocs serve a página interativa do Swagger UI (assets via CDN),
// configurada para consumir /api/openapi.yaml.
func (a *API) handleDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(docsHTML); err != nil {
		slog.Error("servir documentação HTML", "erro", err)
	}
}

// writeJSON serializa v como resposta JSON com o status informado.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("serializar resposta JSON", "erro", err)
	}
}

// respondError mapeia erros para respostas HTTP: erros sentinela de
// "não encontrado" viram 404; hino não revisado vira 409;
// qualquer outro erro vira 500 genérico (detalhes vão para o log, nunca para o cliente).
func respondError(r *http.Request, w http.ResponseWriter, err error) {
	if errors.Is(err, repository.ErrHinoNotFound) || errors.Is(err, repository.ErrColetaneaNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if errors.Is(err, services.ErrHinoNotReviewed) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	slog.Error("requisição falhou", "metodo", r.Method, "path", r.URL.Path, "erro", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{
		"error": "erro interno",
	})
}
