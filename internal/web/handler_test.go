package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/models"
	"github.com/adjoli/louvores-go/internal/repository"
	"github.com/adjoli/louvores-go/internal/services"
)

// setupWeb constrói a interface web com um banco em memória semeado com uma
// coletânea e três hinos cobrindo os três estados de letra/revisão:
//   - número 1: sem letra (Letra nil)
//   - número 2: com letra, não revisado
//   - número 3: com letra, revisado
func setupWeb(t *testing.T) *Web {
	t.Helper()

	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := database.Migrate(conn); err != nil {
		t.Fatalf("migrar: %v", err)
	}

	ctx := context.Background()
	coletaneaRepo := repository.NewSQLiteColetaneaRepository(conn)
	coletanea := &models.Coletanea{Codigo: "CC", Titulo: "Cantor Cristão"}
	if err := coletaneaRepo.Create(ctx, coletanea); err != nil {
		t.Fatalf("criar coletânea: %v", err)
	}

	letra := "Estrofe um\n\n    Refrão"
	hinoRepo := repository.NewSQLiteHinoRepository(conn)

	semear := func(numero int, titulo string, letra *string, revisado bool) {
		t.Helper()
		if err := hinoRepo.Create(ctx, &models.Hino{
			ColetaneaID: coletanea.ID,
			Numeracao:   &numero,
			Titulo:      titulo,
			Letra:       letra,
			Revisado:    revisado,
		}); err != nil {
			t.Fatalf("criar hino %d: %v", numero, err)
		}
	}

	semear(1, "Sem Letra", nil, false)
	semear(2, "Com Letra Não Revisado", &letra, false)
	semear(3, "Revisado", &letra, true)

	hinoSvc := services.NewHinoService(hinoRepo, coletaneaRepo, "data/templates/default.pptx")
	statsSvc := services.NewStatsService(hinoRepo)

	// O handler da API é opcional para os testes web; passamos nil, que o
	// Routes() trata com delegação desativada.
	return New(nil, hinoSvc, statsSvc)
}

// TestStatsPage serve a página /stats com status 200 e conteúdo HTML.
func TestStatsPage(t *testing.T) {
	w := setupWeb(t)

	req := httptest.NewRequest("GET", "/stats", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, esperado 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Louvores") {
		t.Error("página não contém o título base 'Louvores'")
	}
	if !strings.Contains(body, `hx-get="/web/stats/data"`) {
		t.Error("página não declara o placeholder HTMX para /web/stats/data")
	}
}

// TestStatsData serve o fragmento /web/stats/data com a tabela de estatísticas.
func TestStatsData(t *testing.T) {
	w := setupWeb(t)

	req := httptest.NewRequest("GET", "/web/stats/data", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, esperado 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "CC") || !strings.Contains(body, "Cantor Cristão") {
		t.Errorf("fragmento não contém os dados da coletânea semeada: %s", body)
	}
	if !strings.Contains(body, "<table") {
		t.Error("fragmento não contém a tabela de estatísticas")
	}
}

// TestStatsDataVazio renderiza a mensagem de vazio quando não há coletâneas.
func TestStatsDataVazio(t *testing.T) {
	// Banco em memória vazio, apenas com schema migrado.
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := database.Migrate(conn); err != nil {
		t.Fatalf("migrar: %v", err)
	}

	hinoRepo := repository.NewSQLiteHinoRepository(conn)
	coletaneaRepo := repository.NewSQLiteColetaneaRepository(conn)
	hinoSvc := services.NewHinoService(hinoRepo, coletaneaRepo, "")
	statsSvc := services.NewStatsService(hinoRepo)
	w := New(nil, hinoSvc, statsSvc)

	req := httptest.NewRequest("GET", "/web/stats/data", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, esperado 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Nenhuma coletânea") {
		t.Errorf("esperado mensagem de vazio, obtido: %s", rec.Body.String())
	}
}

// TestSlidesPage serve a página /slides com status 200, o seletor HTMX e as
// opções de coletânea.
func TestSlidesPage(t *testing.T) {
	w := setupWeb(t)

	req := httptest.NewRequest("GET", "/slides", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, esperado 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Geração de Slides") {
		t.Error("página não contém o título 'Geração de Slides'")
	}
	if !strings.Contains(body, `hx-get="/web/slides/hinos"`) {
		t.Error("seletor não dispara HTMX para /web/slides/hinos")
	}
	if !strings.Contains(body, "Cantor Cristão") {
		t.Error("seletor não contém a opção da coletânea semeada")
	}
}

// TestSlidesHinos serve o fragmento /web/slides/hinos com a grade de cards,
// verificando numeração zero-padded, cores por estado e o link de geração
// apenas para o hino revisado.
func TestSlidesHinos(t *testing.T) {
	w := setupWeb(t)

	req := httptest.NewRequest("GET", "/web/slides/hinos?codigo=CC", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, esperado 200", rec.Code)
	}
	body := rec.Body.String()

	for _, titulo := range []string{"Sem Letra", "Com Letra Não Revisado", "Revisado"} {
		if !strings.Contains(body, titulo) {
			t.Errorf("grade não contém o hino '%s'", titulo)
		}
	}

	// Numeração com três dígitos e zeros à esquerda.
	if !strings.Contains(body, "001") || !strings.Contains(body, "002") || !strings.Contains(body, "003") {
		t.Errorf("numeração não exibe três dígitos com zeros à esquerda: %s", body)
	}

	// Cores de fundo conforme o estado do hino.
	if !strings.Contains(body, "#FFB7B2") { // sem letra
		t.Error("card sem letra não usa a cor #FFB7B2")
	}
	if !strings.Contains(body, "#FFF5BA") { // letra não revisada
		t.Error("card com letra não revisada não usa a cor #FFF5BA")
	}
	if !strings.Contains(body, "#B5EAD7") { // revisado
		t.Error("card revisado não usa a cor #B5EAD7")
	}

	// Link de geração de slide presente (hino 3, revisado) e correto, na forma
	// de ícone do PowerPoint (ppt.png).
	if !strings.Contains(body, `/api/coletaneas/CC/hinos/3/slides`) {
		t.Error("hino revisado não expõe o link de geração de slide")
	}
	if !strings.Contains(body, `aria-label="Gerar slide"`) || !strings.Contains(body, `src="/static/ppt.png"`) {
		t.Error("link de geração de slide não é exibido como ícone do PowerPoint (ppt.png)")
	}
	if strings.Contains(body, `/api/coletaneas/CC/hinos/1/slides`) || strings.Contains(body, `/api/coletaneas/CC/hinos/2/slides`) {
		t.Error("hino não revisado/sem letra não deve ter link de geração de slide")
	}

	// Ícone de edição presente em todos os cards (funcionalidade futura).
	if !strings.Contains(body, `aria-label="Editar hino"`) || !strings.Contains(body, `src="/static/edit.png"`) {
		t.Error("card não exibe o ícone de edição de hino (edit.png)")
	}
	if !strings.Contains(body, `mt-auto flex items-center justify-center`) {
		t.Error("ícones não estão agrupados na linha do rodapé do card")
	}
}

// TestSlidesHinosSemCodigo retorna 400 quando o parâmetro codigo falta.
func TestSlidesHinosSemCodigo(t *testing.T) {
	w := setupWeb(t)

	req := httptest.NewRequest("GET", "/web/slides/hinos", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, esperado 400", rec.Code)
	}
}

// TestSlidesHinosColetaneaInexistente retorna 404 para código desconhecido.
func TestSlidesHinosColetaneaInexistente(t *testing.T) {
	w := setupWeb(t)

	req := httptest.NewRequest("GET", "/web/slides/hinos?codigo=ZZ", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, esperado 404", rec.Code)
	}
}
