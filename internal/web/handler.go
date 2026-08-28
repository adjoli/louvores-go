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
//   - "/static/"         → arquivos estáticos (CSS gerado pelo Tailwind)
//
// Os handlers dependem apenas do serviço de estatísticas (DI), nunca de HTTP
// interno para buscar dados — evitam a sobrecarga de chamar a própria API.
package web

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/adjoli/louvores-go/internal/services"
	"github.com/adjoli/louvores-go/internal/web/templates"
)

// Web é o container dos handlers da interface web, alimentado por serviços
// via DI, no mesmo estilo do pacote api.
type Web struct {
	api      http.Handler
	statsSvc *services.StatsService
}

// New cria a interface web com o handler da API (montado em main) e o
// serviço de estatísticas. O handler da API é delegado para "/" — ou seja,
// toda requisição sem rota web específica cai na API REST.
func New(apiHandler http.Handler, statsSvc *services.StatsService) *Web {
	return &Web{
		api:      apiHandler,
		statsSvc: statsSvc,
	}
}

// Routes monta e retorna o roteador HTTP combinando API + interface web.
//
// O ServeMux usa correspondência por prefixo mais específico: as rotas web
// (/stats, /web/..., /static/) têm prioridade sobre o "/" que delega para a
// API. Isso permite expor os dois "fronts" no mesmo servidor sem conflito.
func (w *Web) Routes() http.Handler {
	mux := http.NewServeMux()

	// Se não houver handler de API (ex.: uso parcial em testes), a delegação
	// para "/" cai num 404 padrão em vez de pânico.
	if w.api != nil {
		mux.Handle("/", w.api)
	}
	mux.HandleFunc("GET /stats", w.handleStatsPage)
	mux.HandleFunc("GET /web/stats/data", w.handleStatsData)
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
