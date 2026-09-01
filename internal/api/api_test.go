package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

	ctx := context.Background()
	coletaneaRepo := repository.NewSQLiteColetaneaRepository(conn)
	coletanea := &models.Coletanea{Codigo: "CC", Titulo: "Cantor Cristão"}
	if err := coletaneaRepo.Create(ctx, coletanea); err != nil {
		t.Fatalf("criar coletânea: %v", err)
	}

	hinoRepo := repository.NewSQLiteHinoRepository(conn)
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

	hinoSvc := services.NewHinoService(hinoRepo, coletaneaRepo, "")
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
	if s.PercentualRevisados != 0 {
		t.Errorf("percentual_revisados = %v, want 0", s.PercentualRevisados)
	}
}

func TestGerarSlidesLoteEndpoint(t *testing.T) {
	templatePath := filepath.Join("..", "..", "data", "templates", "default.pptx")
	if _, err := os.Stat(templatePath); err != nil {
		t.Skipf("template não encontrado: %v", err)
	}

	conn, err := database.Open(":memory:")
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := database.Migrate(conn); err != nil {
		t.Fatalf("migrar: %v", err)
	}

	ctx := context.Background()
	cr := repository.NewSQLiteColetaneaRepository(conn)
	coletanea := &models.Coletanea{Codigo: "CC", Titulo: "Cantor Cristão"}
	if err := cr.Create(ctx, coletanea); err != nil {
		t.Fatalf("criar coletânea: %v", err)
	}

	hr := repository.NewSQLiteHinoRepository(conn)
	num := 42
	letra := "Estrofe um\n\n    Refrão"
	if err := hr.Create(ctx, &models.Hino{
		ColetaneaID: coletanea.ID, Numeracao: &num,
		Titulo: "Grandioso Pai", Letra: &letra, Revisado: true,
	}); err != nil {
		t.Fatalf("criar hino: %v", err)
	}

	hinoSvc := services.NewHinoService(hr, cr, templatePath)
	handler := New(hinoSvc, services.NewStatsService(hr)).Routes()

	res := doRequest(handler, http.MethodPost, "/api/coletaneas/CC/slides/lote")

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if ct := res.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("content-type = %q, want application/zip", ct)
	}
	if cd := res.Header().Get("Content-Disposition"); !strings.Contains(cd, "CC-slides.zip") {
		t.Errorf("content-disposition = %q, want CC-slides.zip", cd)
	}

	zr, err := zip.NewReader(bytes.NewReader(res.Body.Bytes()), int64(res.Body.Len()))
	if err != nil {
		t.Fatalf("abrir zip: %v", err)
	}
	if len(zr.File) != 1 {
		t.Fatalf("arquivos no zip = %d, want 1", len(zr.File))
	}
	if zr.File[0].Name != "CC-042-GRANDIOSO_PAI.pptx" {
		t.Errorf("nome do arquivo = %q", zr.File[0].Name)
	}
}

func TestGerarSlidesLoteColetaneaInexistenteEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	res := doRequest(handler, http.MethodPost, "/api/coletaneas/XX/slides/lote")

	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusNotFound)
	}
}

func TestOpenAPISpecEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	res := doRequest(handler, http.MethodGet, "/api/openapi.yaml")

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if ct := res.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("content-type = %q, want application/yaml", ct)
	}
	if !strings.Contains(res.Body.String(), "/api/coletaneas/{codigo}/hinos") {
		t.Error("spec servida não contém os paths esperados")
	}
}

func TestDocsEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	res := doRequest(handler, http.MethodGet, "/api/docs")

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if ct := res.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content-type = %q, want text/html", ct)
	}
	if !strings.Contains(res.Body.String(), "SwaggerUIBundle") {
		t.Error("página não referencia o Swagger UI")
	}
}

// TestContratoJSONSnakeCase trava o contrato público da API: as chaves
// serializadas são as tags json (snake_case) e campos opcionais nulos
// aparecem como null explícito.
func TestContratoJSONSnakeCase(t *testing.T) {
	handler := newTestHandler(t)

	res := doRequest(handler, http.MethodGet, "/api/coletaneas/CC/hinos")

	corpo := res.Body.String()
	for _, chave := range []string{
		`"id"`, `"coletanea_id"`, `"numeracao"`, `"titulo"`,
		`"letra"`, `"creditos"`, `"revisado"`,
	} {
		if !strings.Contains(corpo, chave) {
			t.Errorf("chave %s ausente na resposta: %s", chave, corpo)
		}
	}
}

// TestParidadeRotasSpec compara os paths declarados na spec OpenAPI embutida
// com as rotas efetivamente registradas no mux — evita documentação
// desatualizada quando uma rota entra ou sai sem tocar o YAML.
func TestParidadeRotasSpec(t *testing.T) {
	api := &API{}

	padroes := make(map[string]bool, len(api.rotas()))
	for _, rota := range api.rotas() {
		_, caminho, ok := strings.Cut(rota.padrao, " ")
		if !ok {
			t.Fatalf("padrão inesperado: %q", rota.padrao)
		}
		padroes[caminho] = true
	}

	re := regexp.MustCompile(`(?m)^  (/.+):$`)
	spec := make(map[string]bool)
	for _, m := range re.FindAllStringSubmatch(string(openapiSpec), -1) {
		spec[m[1]] = true
	}

	if len(spec) == 0 {
		t.Fatal("nenhum path extraído da spec OpenAPI")
	}

	for caminho := range padroes {
		if !spec[caminho] {
			t.Errorf("rota registrada ausente na spec: %s", caminho)
		}
	}
	for caminho := range spec {
		if !padroes[caminho] {
			t.Errorf("path documentado sem rota registrada: %s", caminho)
		}
	}
}
