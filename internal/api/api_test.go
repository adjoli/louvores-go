package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/models"
	"github.com/adjoli/louvores-go/internal/repository"
	"github.com/adjoli/louvores-go/internal/services"
)

// newTestHandler monta um handler HTTP completo sobre um banco em memória
// com a coletânea "CC" e os hinos 42 (com letra) e 43 (sem letra).
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := database.Migrate(conn); err != nil {
		t.Fatalf("migrar: %v", err)
	}
	conn.SetMaxOpenConns(1)

	ctx := context.Background()
	coletaneaRepo := repository.NewColetaneaRepository(conn)
	coletanea := &models.Coletanea{Codigo: "CC", Titulo: "Cantor Cristão"}
	if err := coletaneaRepo.Create(ctx, coletanea); err != nil {
		t.Fatalf("criar coletânea: %v", err)
	}

	hinoRepo := repository.NewHinoRepository(conn)
	num := 42
	letra := "Estrofe um\n\n    Refrão"
	if err := hinoRepo.Create(ctx, &models.Hino{
		ColetaneaID: coletanea.ID,
		Numeracao:   &num,
		Titulo:      "Grandioso Pai",
		Letra:       &letra,
	}); err != nil {
		t.Fatalf("criar hino 42: %v", err)
	}

	semLetra := 43
	if err := hinoRepo.Create(ctx, &models.Hino{
		ColetaneaID: coletanea.ID,
		Numeracao:   &semLetra,
		Titulo:      "Sem Letra",
	}); err != nil {
		t.Fatalf("criar hino 43: %v", err)
	}

	hinoSvc := services.NewHinoService(hinoRepo, coletaneaRepo)
	statsSvc := services.NewStatsService(hinoRepo)

	return New(hinoSvc, statsSvc).Routes()
}

func doRequest(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	handler := newTestHandler(t)

	res := doRequest(handler, http.MethodGet, "/api/healthz")

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	var body map[string]string
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestListarColetaneasEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	res := doRequest(handler, http.MethodGet, "/api/coletaneas")

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	var coletaneas []models.Coletanea
	if err := json.Unmarshal(res.Body.Bytes(), &coletaneas); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	if len(coletaneas) != 1 || coletaneas[0].Codigo != "CC" {
		t.Errorf("coletaneas = %+v", coletaneas)
	}
}

func TestListarHinosEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	res := doRequest(handler, http.MethodGet, "/api/coletaneas/CC/hinos")

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	var hinos []models.Hino
	if err := json.Unmarshal(res.Body.Bytes(), &hinos); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	if len(hinos) != 2 {
		t.Fatalf("len = %d, want 2", len(hinos))
	}
	if hinos[0].Titulo != "Grandioso Pai" || hinos[1].Titulo != "Sem Letra" {
		t.Errorf("hinos fora de ordem: %+v", hinos)
	}
}

func TestListarHinosColetaneaInexistenteEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	res := doRequest(handler, http.MethodGet, "/api/coletaneas/XX/hinos")

	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusNotFound)
	}
}

func TestObterHinoEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	res := doRequest(handler, http.MethodGet, "/api/coletaneas/CC/hinos/42")

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	var hino models.Hino
	if err := json.Unmarshal(res.Body.Bytes(), &hino); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	if hino.Titulo != "Grandioso Pai" {
		t.Errorf("titulo = %q, want %q", hino.Titulo, "Grandioso Pai")
	}
}

func TestObterHinoNaoEncontradoEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	res := doRequest(handler, http.MethodGet, "/api/coletaneas/CC/hinos/999")

	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusNotFound)
	}
}

func TestObterHinoNumeroInvalidoEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	res := doRequest(handler, http.MethodGet, "/api/coletaneas/CC/hinos/abc")

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
}

func TestStatsEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	res := doRequest(handler, http.MethodGet, "/api/stats")

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	var stats []services.ColetaneaStats
	if err := json.Unmarshal(res.Body.Bytes(), &stats); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("len = %d, want 1", len(stats))
	}
	s := stats[0]
	if s.Codigo != "CC" || s.Total != 2 || s.ComLetra != 1 || s.Revisados != 0 {
		t.Errorf("stats = %+v", s)
	}
	if s.Percentual != 50 {
		t.Errorf("percentual = %v, want 50", s.Percentual)
	}
}
