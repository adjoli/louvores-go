// Package web provê a interface web (HTML) da aplicação.
//
// Diferente do pacote api (que responde JSON), este pacote renderiza views
// HTML usando o template tipado `templ`. As páginas são servidas como HTML
// completo, já com os dados embutidos no render (sem HTMX).
//
// Arquitetura das rotas:
//   - "/stats"   → página completa de estatísticas (tabela embutida)
//   - "/slides"  → página de geração de slides (?codigo= preenche a grade)
//   - "/web/..." → fluxo de edição/salvamento do hino (GET form + POST PRG)
//   - "/static/" → arquivos estáticos (main.css mantido manualmente)
//
// Os handlers dependem apenas dos serviços (DI), nunca de HTTP interno para
// buscar dados — evitam a sobrecarga de chamar a própria API.
package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/adjoli/louvores-go/internal/models"
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
	version  string
}

// New cria a interface web com o handler da API (montado em main), os
// serviços de hinos e estatísticas, e a versão da aplicação (exibida no
// rodapé das páginas). O handler da API é delegado para "/" — ou seja, toda
// requisição sem rota web específica cai na API REST.
func New(apiHandler http.Handler, hinoSvc *services.HinoService, statsSvc *services.StatsService, version string) *Web {
	return &Web{
		api:      apiHandler,
		hinoSvc:  hinoSvc,
		statsSvc: statsSvc,
		version:  version,
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
	mux.HandleFunc("GET /slides", w.handleSlidesPage)
	mux.HandleFunc("GET /web/hinos/{codigo}/{numero}/editar", w.handleEditarHinoPage)
	mux.HandleFunc("POST /web/hinos/{codigo}/{numero}", w.handleSalvarHino)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("internal/web/static"))))

	return mux
}

// handleStatsPage serve a página completa de estatísticas, com a tabela de
// estatísticas embutida no HTML.
func (w *Web) handleStatsPage(wr http.ResponseWriter, r *http.Request) {
	stats, err := w.statsSvc.ObterStats(r.Context())
	if err != nil {
		slog.Error("buscar estatísticas", "erro", err)
		http.Error(wr, "erro interno", http.StatusInternalServerError)
		return
	}

	if err := templates.StatsPage(stats, w.version).Render(r.Context(), wr); err != nil {
		slog.Error("renderizar página de estatísticas", "erro", err)
	}
}

// handleSlidesPage serve a página de geração de slides com o seletor de
// coletânea. Se a query ?codigo= estiver presente e a coletânea existir, a
// grade de hinos já vem preenchida no render.
func (w *Web) handleSlidesPage(wr http.ResponseWriter, r *http.Request) {
	coletaneas, err := w.hinoSvc.ListarColetaneas(r.Context())
	if err != nil {
		slog.Error("buscar coletâneas", "erro", err)
		http.Error(wr, "erro interno", http.StatusInternalServerError)
		return
	}

	codigo := r.URL.Query().Get("codigo")
	var hinos []models.Hino
	if codigo != "" {
		hinos, err = w.hinoSvc.ListarHinos(r.Context(), codigo)
		if err != nil {
			if errors.Is(err, repository.ErrColetaneaNotFound) {
				http.Error(wr, "coletânea não encontrada", http.StatusNotFound)
				return
			}
			slog.Error("buscar hinos da coletânea", "coletanea", codigo, "erro", err)
			http.Error(wr, "erro interno", http.StatusInternalServerError)
			return
		}
	}

	if err := templates.SlidesPage(coletaneas, hinos, codigo, w.version).Render(r.Context(), wr); err != nil {
		slog.Error("renderizar página de geração de slides", "erro", err)
	}
}

// handleEditarHinoPage serve o formulário de edição de um hino, acessado pelo
// ícone edit.png no card. Número e coletânea vêm da rota (não editáveis).
func (w *Web) handleEditarHinoPage(wr http.ResponseWriter, r *http.Request) {
	codigo := r.PathValue("codigo")
	numero, err := strconv.Atoi(r.PathValue("numero"))
	if err != nil {
		http.Error(wr, "número inválido", http.StatusBadRequest)
		return
	}

	hino, err := w.hinoSvc.ObterHino(r.Context(), codigo, numero)
	if err != nil {
		if errors.Is(err, repository.ErrColetaneaNotFound) || errors.Is(err, repository.ErrHinoNotFound) {
			http.Error(wr, "hino não encontrado", http.StatusNotFound)
			return
		}
		slog.Error("buscar hino para edição", "coletanea", codigo, "numero", numero, "erro", err)
		http.Error(wr, "erro interno", http.StatusInternalServerError)
		return
	}

	if err := templates.EditarHinoPage(*hino, codigo, w.version).Render(r.Context(), wr); err != nil {
		slog.Error("renderizar formulário de edição", "coletanea", codigo, "numero", numero, "erro", err)
	}
}

// handleSalvarHino processa o POST do formulário de edição, aplica a
// conversão Title Case na letra (na camada de serviço) e redireciona de volta
// para a página de geração de slides. Se o hino já foi revisado, a revisão
// não é revertida (regra tratada no serviço).
func (w *Web) handleSalvarHino(wr http.ResponseWriter, r *http.Request) {
	codigo := r.PathValue("codigo")
	numero, err := strconv.Atoi(r.PathValue("numero"))
	if err != nil {
		http.Error(wr, "número inválido", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(wr, "formulário inválido", http.StatusBadRequest)
		return
	}

	upd := services.HinoUpdate{
		Titulo:   r.FormValue("titulo"),
		Letra:    r.FormValue("letra"),
		Creditos: r.FormValue("creditos"),
		Revisado: r.FormValue("revisado") == "on",
	}

	if _, err := w.hinoSvc.AtualizarHino(r.Context(), codigo, numero, upd); err != nil {
		if errors.Is(err, repository.ErrColetaneaNotFound) || errors.Is(err, repository.ErrHinoNotFound) {
			http.Error(wr, "hino não encontrado", http.StatusNotFound)
			return
		}
		slog.Error("salvar alterações do hino", "coletanea", codigo, "numero", numero, "erro", err)
		http.Error(wr, "erro interno", http.StatusInternalServerError)
		return
	}

	slog.Info("hino atualizado", "coletanea", codigo, "numero", numero)

	// Redireciona para a listagem da coletânea editada (PRG: evitar re-submit).
	http.Redirect(wr, r, "/slides", http.StatusSeeOther)
}
