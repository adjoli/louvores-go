// Package web provê a interface web (HTML) da aplicação.
//
// Diferente do pacote api (que responde JSON), este pacote renderiza views
// HTML usando o framework de templates tipados `templ` combinado com HTMX
// para atualização parcial de conteúdo sem recarregar a página.
//
// Arquitetura das rotas:
//   - "/stats"           → página completa (shell + placeholder HTMX)
//   - "/web/stats/data"  → fragmento HTML com a tabela de estatísticas,
//     consumido pelo HTMX via hx-get/hx-trigger="load"
//   - "/slides"          → página de geração de slides (shell + seletor HTMX)
//   - "/web/slides/hinos"→ fragmento HTML com a grade de cards dos hinos,
//     consumido pelo HTMX via hx-get/hx-trigger="change"
//   - "/static/"         → arquivos estáticos (CSS gerado pelo Tailwind)
//
// Os handlers dependem apenas dos serviços (DI), nunca de HTTP interno para
// buscar dados — evitam a sobrecarga de chamar a própria API.
package web

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/adjoli/louvores-go/internal/repository"
	"github.com/adjoli/louvores-go/internal/services"
	"github.com/adjoli/louvores-go/internal/web/templates"
)

// Web é o container dos handlers da interface web, alimentado por serviços
// via DI, no mesmo estilo do pacote api.
type Web struct {
	api      http.Handler
	hinoSvc  *services.HinoService
	statsSvc *services.StatsService
}

// New cria a interface web com o handler da API (montado em main) e os
// serviços de hinos e estatísticas. O handler da API é delegado para "/" —
// ou seja, toda requisição sem rota web específica cai na API REST.
func New(apiHandler http.Handler, hinoSvc *services.HinoService, statsSvc *services.StatsService) *Web {
	return &Web{
		api:      apiHandler,
		hinoSvc:  hinoSvc,
		statsSvc: statsSvc,
	}
}

// Routes monta e retorna o roteador HTTP combinando API + interface web.
//
// O ServeMux usa correspondência por prefixo mais específico: as rotas web
// (/stats, /slides, /web/..., /static/) têm prioridade sobre o "/" que delega
// para a API. Isso permite expor os dois "fronts" no mesmo servidor sem conflito.
func (w *Web) Routes() http.Handler {
	mux := http.NewServeMux()

	// Se não houver handler de API (ex.: uso parcial em testes), a delegação
	// para "/" cai num 404 padrão em vez de pânico.
	if w.api != nil {
		mux.Handle("/", w.api)
	}
	mux.HandleFunc("GET /stats", w.handleStatsPage)
	mux.HandleFunc("GET /web/stats/data", w.handleStatsData)
	mux.HandleFunc("GET /slides", w.handleSlidesPage)
	mux.HandleFunc("GET /web/slides/hinos", w.handleSlidesHinos)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("internal/web/static"))))

	return mux
}

// handleStatsPage serve a página completa de estatísticas. A página contém
// apenas o placeholder; os dados são carregados assíncronamente pelo HTMX.
func (w *Web) handleStatsPage(wr http.ResponseWriter, r *http.Request) {
	err := templates.StatsPage().Render(r.Context(), wr)
	if err != nil {
		slog.Error("renderizar página de estatísticas", "erro", err)
	}
}

// handleStatsData serve o fragmento HTML com a tabela de estatísticas.
// É o endpoint consumido pelo HTMX (hx-get) para preencher o placeholder.
func (w *Web) handleStatsData(wr http.ResponseWriter, r *http.Request) {
	stats, err := w.statsSvc.ObterStats(r.Context())
	if err != nil {
		slog.Error("buscar estatísticas", "erro", err)
		http.Error(wr, "erro interno", http.StatusInternalServerError)
		return
	}

	var comp templ.Component
	if len(stats) == 0 {
		comp = templates.StatsVazio()
	} else {
		comp = templates.StatsTable(stats)
	}

	if err := comp.Render(r.Context(), wr); err != nil {
		slog.Error("renderizar fragmento de estatísticas", "erro", err)
	}
}

// handleSlidesPage serve a página de geração de slides com o seletor de
// coletâneas. Os hinos são carregados assíncronamente pelo HTMX.
func (w *Web) handleSlidesPage(wr http.ResponseWriter, r *http.Request) {
	coletaneas, err := w.hinoSvc.ListarColetaneas(r.Context())
	if err != nil {
		slog.Error("buscar coletâneas", "erro", err)
		http.Error(wr, "erro interno", http.StatusInternalServerError)
		return
	}

	if err := templates.SlidesPage(coletaneas).Render(r.Context(), wr); err != nil {
		slog.Error("renderizar página de geração de slides", "erro", err)
	}
}

// handleSlidesHinos serve o fragmento HTML com a grade de cards dos hinos da
// coletânea informada via query (?codigo=CC). É o endpoint consumido pelo
// HTMX para preencher o container #hinos ao trocar a coletânea.
func (w *Web) handleSlidesHinos(wr http.ResponseWriter, r *http.Request) {
	codigo := r.URL.Query().Get("codigo")
	if codigo == "" {
		http.Error(wr, "parâmetro codigo é obrigatório", http.StatusBadRequest)
		return
	}

	hinos, err := w.hinoSvc.ListarHinos(r.Context(), codigo)
	if err != nil {
		if errors.Is(err, repository.ErrColetaneaNotFound) {
			http.Error(wr, "coletânea não encontrada", http.StatusNotFound)
			return
		}
		slog.Error("buscar hinos da coletânea", "coletanea", codigo, "erro", err)
		http.Error(wr, "erro interno", http.StatusInternalServerError)
		return
	}

	if err := templates.HinosGrid(hinos, codigo).Render(r.Context(), wr); err != nil {
		slog.Error("renderizar grade de hinos", "coletanea", codigo, "erro", err)
	}
}
