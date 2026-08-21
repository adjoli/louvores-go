// Package api expõe a API REST da aplicação (somente leitura nesta fase).
//
// Os handlers são finos: interpretam a requisição (parâmetros de rota),
// delegam aos serviços e serializam o resultado como JSON. Erros de
// negócio são mapeados para códigos HTTP: não encontrado → 404,
// parâmetro inválido → 400, demais → 500 com log estruturado.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/adjoli/louvores-go/internal/repository"
	"github.com/adjoli/louvores-go/internal/services"
)

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

// Routes monta e retorna o roteador HTTP com todos os endpoints da API.
// Usa o ServeMux da stdlib (Go 1.22+) com padrões "METHOD /caminho/{param}".
func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/healthz", a.handleHealth)
	mux.HandleFunc("GET /api/coletaneas", a.handleListarColetaneas)
	mux.HandleFunc("GET /api/coletaneas/{codigo}/hinos", a.handleListarHinos)
	mux.HandleFunc("GET /api/coletaneas/{codigo}/hinos/{numero}", a.handleObterHino)
	mux.HandleFunc("GET /api/stats", a.handleStats)

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

// handleStats retorna as estatísticas agregadas por coletânea.
func (a *API) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.statsSvc.ObterStats(r.Context())
	if err != nil {
		respondError(r, w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
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
// "não encontrado" viram 404; qualquer outro erro vira 500 genérico
// (detalhes vão para o log, nunca para o cliente).
func respondError(r *http.Request, w http.ResponseWriter, err error) {
	if errors.Is(err, repository.ErrHinoNotFound) || errors.Is(err, repository.ErrColetaneaNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	slog.Error("requisição falhou", "metodo", r.Method, "path", r.URL.Path, "erro", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{
		"error": "erro interno",
	})
}
