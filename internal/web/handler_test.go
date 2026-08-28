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
// coletânea e um hino, reutilizando o mesmo padrão de DI dos testes de serviço.
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

	num := 42
	letra := "Estrofe um\n\n    Refrão"
	hinoRepo := repository.NewSQLiteHinoRepository(conn)
	if err := hinoRepo.Create(ctx, &models.Hino{
		ColetaneaID: coletanea.ID,
		Numeracao:   &num,
		Titulo:      "Grandioso Pai",
		Letra:       &letra,
	}); err != nil {
		t.Fatalf("criar hino: %v", err)
	}

	statsSvc := services.NewStatsService(hinoRepo)

	// O handler da API é opcional para os testes web; passamos um mux vazio
	// que nunca será acionado nas rotas web testadas.
	return New(nil, statsSvc)
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
	statsSvc := services.NewStatsService(hinoRepo)
	w := New(nil, statsSvc)

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
