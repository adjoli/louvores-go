package web

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/adjoli/louvores-go/internal/auth"
	"github.com/adjoli/louvores-go/internal/config"
	"github.com/adjoli/louvores-go/internal/database"
	"github.com/adjoli/louvores-go/internal/models"
	"github.com/adjoli/louvores-go/internal/repository"
	"github.com/adjoli/louvores-go/internal/services"
)

// testVersion é a versão injetada nos handlers web durante os testes.
const testVersion = "test-version"

// setupWeb constrói a interface web com um banco em memória semeado com uma
// coletânea e três hinos cobrindo os três estados de letra/revisão:
//   - número 1: sem letra (Letra nil)
//   - número 2: com letra, não revisado
//   - número 3: com letra, revisado
//
// Retorna também a conexão *sql.DB para testes que precisam inspecionar o
// banco após uma escrita.
func setupWeb(t *testing.T) (*Web, *sql.DB) {
	t.Helper()
	return setupWebComSenha(t, "")
}

// setupWebComSenha é a variação de setupWeb que ativa a autenticação com a
// senha informada (string vazia = autenticação desligada).
func setupWebComSenha(t *testing.T, senha string) (*Web, *sql.DB) {
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

	authSvc := auth.New(&config.Config{
		AuthPassword:  senha,
		SessionSecret: "test-secret",
		SessionTTL:    time.Hour,
	})

	// O handler da API é opcional para os testes web; passamos nil, que o
	// Routes() trata com delegação desativada.
	return New(nil, hinoSvc, statsSvc, authSvc, testVersion), conn
}

// TestStatsPage serve a página /stats com status 200 e conteúdo HTML,
// incluindo a tabela de estatísticas embutida (sem HTMX).
func TestStatsPage(t *testing.T) {
	w, _ := setupWeb(t)

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
	if !strings.Contains(body, "<table") {
		t.Error("página não contém a tabela de estatísticas")
	}
	if !strings.Contains(body, "CC") || !strings.Contains(body, "Cantor Cristão") {
		t.Error("página não contém os dados da coletânea semeada")
	}
	if !strings.Contains(body, "% com letra") || !strings.Contains(body, "% revisados") {
		t.Error("tabela não exibe os cabeçalhos de percentual de letra e de revisados")
	}
	if !strings.Contains(body, "v"+testVersion) {
		t.Error("rodapé não exibe a versão da aplicação")
	}
	if strings.Contains(body, "htmx") {
		t.Error("página não deveria depender de HTMX")
	}
}

// TestStatsPageVazio renderiza a mensagem de vazio quando não há coletâneas.
func TestStatsPageVazio(t *testing.T) {
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
	authSvc := auth.New(&config.Config{SessionSecret: "test-secret", SessionTTL: time.Hour})
	w := New(nil, hinoSvc, statsSvc, authSvc, testVersion)

	req := httptest.NewRequest("GET", "/stats", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, esperado 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Nenhuma coletânea") {
		t.Errorf("esperado mensagem de vazio, obtido: %s", rec.Body.String())
	}
}

// TestSlidesPage serve a página /slides com status 200, o seletor de
// coletânea e as opções de coletânea. Sem ?codigo, a grade ainda não aparece.
func TestSlidesPage(t *testing.T) {
	w, _ := setupWeb(t)

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
	if !strings.Contains(body, "Cantor Cristão") {
		t.Error("seletor não contém a opção da coletânea semeada")
	}
	if strings.Contains(body, "htmx") {
		t.Error("página não deveria depender de HTMX")
	}
}

// TestSlidesGrade serve a página /slides?codigo=CC com a grade de cards já
// preenchida, verificando numeração zero-padded, cores por estado e o link
// de geração apenas para o hino revisado.
func TestSlidesGrade(t *testing.T) {
	w, _ := setupWeb(t)

	req := httptest.NewRequest("GET", "/slides?codigo=CC", nil)
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

	// Ícone de edição presente em todos os cards.
	if !strings.Contains(body, `aria-label="Editar hino"`) || !strings.Contains(body, `src="/static/edit.png"`) {
		t.Error("card não exibe o ícone de edição de hino (edit.png)")
	}
	if !strings.Contains(body, `mt-auto flex items-center justify-center`) {
		t.Error("ícones não estão agrupados na linha do rodapé do card")
	}
}

// TestSlidesColetaneaInexistente retorna 404 para código desconhecido.
func TestSlidesColetaneaInexistente(t *testing.T) {
	w, _ := setupWeb(t)

	req := httptest.NewRequest("GET", "/slides?codigo=ZZ", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, esperado 404", rec.Code)
	}
}

// TestEditarHinoPage serve o formulário de edição com os dados do hino.
func TestEditarHinoPage(t *testing.T) {
	w, _ := setupWeb(t)

	req := httptest.NewRequest("GET", "/web/hinos/CC/2/editar", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, esperado 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Editar Hino") {
		t.Error("página não contém o título 'Editar Hino'")
	}
	if !strings.Contains(body, `name="titulo"`) || !strings.Contains(body, `name="letra"`) {
		t.Error("formulário não contém os campos titulo/letra")
	}
	if !strings.Contains(body, `name="revisado"`) {
		t.Error("formulário não contém o campo revisado")
	}
	if !strings.Contains(body, "002") {
		t.Error("página não exibe a numeração do hino")
	}
}

// TestEditarHinoPageRevisadoDisabled garante que o checkbox revisado fica
// desabilitado (read-only) quando o hino já foi revisado.
func TestEditarHinoPageRevisadoDisabled(t *testing.T) {
	w, _ := setupWeb(t) // hino 3 é revisado

	req := httptest.NewRequest("GET", "/web/hinos/CC/3/editar", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, esperado 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="revisado"`) || !strings.Contains(body, `disabled`) {
		t.Error("checkbox revisado deveria estar desabilitado para hino revisado")
	}
	if !strings.Contains(body, `checked`) {
		t.Error("checkbox revisado deveria estar marcado")
	}
}

// TestEditarHinoPageRevisadoNaoMarcado garante que, para hino não revisado,
// o checkbox NÃO vem marcado nem desabilitado (atributo ausente).
func TestEditarHinoPageRevisadoNaoMarcado(t *testing.T) {
	w, _ := setupWeb(t) // hino 2 não é revisado

	req := httptest.NewRequest("GET", "/web/hinos/CC/2/editar", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, esperado 200", rec.Code)
	}
	body := rec.Body.String()
	// O checkbox deve existir sem os atributos checked/disabled.
	if !strings.Contains(body, `id="revisado"`) {
		t.Fatal("checkbox revisado ausente")
	}
	// Extrai apenas o trecho do input revisado para evitar falsos positivos.
	start := strings.Index(body, `id="revisado"`)
	end := strings.Index(body[start:], ">") + start
	input := body[start:end]
	if strings.Contains(input, "checked") {
		t.Error("checkbox revisado deveria estar desmarcado para hino não revisado")
	}
	if strings.Contains(input, "disabled") {
		t.Error("checkbox revisado deveria estar habilitado para hino não revisado")
	}
}

// TestEditarHinoPageNaoEncontrado retorna 404 para hino/coletânea inexistentes.
func TestEditarHinoPageNaoEncontrado(t *testing.T) {
	w, _ := setupWeb(t)

	req := httptest.NewRequest("GET", "/web/hinos/CC/999/editar", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, esperado 404", rec.Code)
	}
}

// TestSalvarHino submete o formulário e verifica redirecionamento (303) e
// persistência das alterações com Title Case na letra.
func TestSalvarHino(t *testing.T) {
	w, conn := setupWeb(t)

	form := url.Values{
		"titulo":   {"Novo Título"},
		"letra":    {"letra em minúsculas"},
		"creditos": {"Novos créditos"},
		"revisado": {"on"},
	}
	req := httptest.NewRequest("POST", "/web/hinos/CC/2", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 303 {
		t.Fatalf("status = %d, esperado 303 (See Other)", rec.Code)
	}

	// Verifica a persistência lendo direto do repositório via serviço.
	hinoSvc := services.NewHinoService(
		repository.NewSQLiteHinoRepository(conn),
		repository.NewSQLiteColetaneaRepository(conn),
		"",
	)
	hino, err := hinoSvc.ObterHino(context.Background(), "CC", 2)
	if err != nil {
		t.Fatalf("ObterHino: %v", err)
	}
	if hino.Titulo != "Novo Título" {
		t.Errorf("titulo = %q", hino.Titulo)
	}
	if hino.Letra == nil || *hino.Letra != "Letra Em Minúsculas" {
		t.Errorf("letra = %v, esperado Title Case", hino.Letra)
	}
	if !hino.Revisado {
		t.Error("revisado deveria ser true")
	}
}

// TestSalvarHinoRevisaoIrreversivel garante que o POST não desmarca um hino
// já revisado.
func TestSalvarHinoRevisaoIrreversivel(t *testing.T) {
	w, conn := setupWeb(t) // hino 3 é revisado

	form := url.Values{
		"titulo":   {"Revisado Título"},
		"letra":    {"letra"},
		"creditos": {"Autor"},
		// revisado ausente = desmarcado
	}
	req := httptest.NewRequest("POST", "/web/hinos/CC/3", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != 303 {
		t.Fatalf("status = %d, esperado 303", rec.Code)
	}

	hinoSvc := services.NewHinoService(
		repository.NewSQLiteHinoRepository(conn),
		repository.NewSQLiteColetaneaRepository(conn),
		"",
	)
	hino, err := hinoSvc.ObterHino(context.Background(), "CC", 3)
	if err != nil {
		t.Fatalf("ObterHino: %v", err)
	}
	if !hino.Revisado {
		t.Error("revisado deveria permanecer true (irreversível)")
	}
}

func TestSeloImportadoCardsEEdicao(t *testing.T) {
	w, conn := setupWeb(t)

	// Adiciona import_batch e import_log ativo para o hino 2
	if _, err := conn.Exec(`
		INSERT INTO import_batch (batch_id, manifest_sha256) VALUES ('batch1', 'sha1');
		INSERT INTO import_log (batch_id, manifest_sha256, hino_id, codigo, numero, titulo_destino, letra_nova, creditos_alterados)
		SELECT 'batch1', 'sha1', id, 'CC', 2, 'Com Letra Não Revisado', 'Letra nova', 0
		FROM hino WHERE coletanea_id = 1 AND numeracao = 2;
	`); err != nil {
		t.Fatalf("inserir import_log: %v", err)
	}

	badge := `<span class="import-badge">Letra importada — confira</span>`

	// 1. GET /slides?codigo=CC: badge deve aparecer no card do hino 2, e não nos outros
	req := httptest.NewRequest("GET", "/slides?codigo=CC", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET /slides status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, badge) {
		t.Fatalf("GET /slides deveria conter o selo de importação: %s", body)
	}
	if strings.Count(body, "Letra importada — confira") != 1 {
		t.Errorf("selo de importação deveria aparecer exatamente 1 vez na grade de hinos, apareceu %d", strings.Count(body, "Letra importada — confira"))
	}

	// 2. GET /web/hinos/CC/2/editar: formulário deve exibir o selo
	reqEdit := httptest.NewRequest("GET", "/web/hinos/CC/2/editar", nil)
	recEdit := httptest.NewRecorder()
	w.Routes().ServeHTTP(recEdit, reqEdit)
	if recEdit.Code != 200 {
		t.Fatalf("GET /web/hinos/CC/2/editar status = %d", recEdit.Code)
	}
	if !strings.Contains(recEdit.Body.String(), badge) {
		t.Errorf("edição do hino 2 deveria conter o selo: %s", recEdit.Body.String())
	}

	// 3. GET /web/hinos/CC/1/editar e CC/3/editar: não devem exibir o selo
	for _, num := range []string{"1", "3"} {
		reqOutro := httptest.NewRequest("GET", "/web/hinos/CC/"+num+"/editar", nil)
		recOutro := httptest.NewRecorder()
		w.Routes().ServeHTTP(recOutro, reqOutro)
		if strings.Contains(recOutro.Body.String(), badge) {
			t.Errorf("edição do hino %s não deveria conter o selo", num)
		}
	}

	// 4. Salvar sem marcar revisado mantém o selo
	form := url.Values{
		"titulo":   {"Com Letra Não Revisado Editado"},
		"letra":    {"Estrofe editada\n\n    Refrão"},
		"creditos": {"Autor"},
	}
	reqPost := httptest.NewRequest("POST", "/web/hinos/CC/2", strings.NewReader(form.Encode()))
	reqPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recPost := httptest.NewRecorder()
	w.Routes().ServeHTTP(recPost, reqPost)
	if recPost.Code != 303 {
		t.Fatalf("POST status = %d, esperado 303", recPost.Code)
	}

	recAposSalvar := httptest.NewRecorder()
	w.Routes().ServeHTTP(recAposSalvar, httptest.NewRequest("GET", "/slides?codigo=CC", nil))
	if !strings.Contains(recAposSalvar.Body.String(), badge) {
		t.Error("após salvar sem revisar, o selo deveria continuar visível na grade")
	}

	// 5. Salvar marcando revisado esconde o selo
	formRevisar := url.Values{
		"titulo":   {"Com Letra Agora Revisado"},
		"letra":    {"Estrofe editada\n\n    Refrão"},
		"creditos": {"Autor"},
		"revisado": {"on"},
	}
	reqPostRev := httptest.NewRequest("POST", "/web/hinos/CC/2", strings.NewReader(formRevisar.Encode()))
	reqPostRev.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recPostRev := httptest.NewRecorder()
	w.Routes().ServeHTTP(recPostRev, reqPostRev)
	if recPostRev.Code != 303 {
		t.Fatalf("POST status = %d, esperado 303", recPostRev.Code)
	}

	recAposRevisar := httptest.NewRecorder()
	w.Routes().ServeHTTP(recAposRevisar, httptest.NewRequest("GET", "/slides?codigo=CC", nil))
	if strings.Contains(recAposRevisar.Body.String(), "Letra importada — confira") {
		t.Error("após revisar o hino, o selo deveria desaparecer da grade")
	}

	recEditAposRevisar := httptest.NewRecorder()
	w.Routes().ServeHTTP(recEditAposRevisar, httptest.NewRequest("GET", "/web/hinos/CC/2/editar", nil))
	if strings.Contains(recEditAposRevisar.Body.String(), "Letra importada — confira") {
		t.Error("após revisar o hino, o selo deveria desaparecer do formulário de edição")
	}
}

func TestSeloImportadoRevertidoOuSemConteudo(t *testing.T) {
	w, conn := setupWeb(t)

	// Adiciona import_log para hino 1 (que não tem letra) e hino 2 (revertido)
	if _, err := conn.Exec(`
		INSERT INTO import_batch (batch_id, manifest_sha256) VALUES ('batch1', 'sha1');
		INSERT INTO import_log (batch_id, manifest_sha256, hino_id, codigo, numero, titulo_destino, letra_nova, creditos_alterados)
		SELECT 'batch1', 'sha1', id, 'CC', 1, 'Sem Letra', 'Letra', 0
		FROM hino WHERE coletanea_id = 1 AND numeracao = 1;

		INSERT INTO import_log (batch_id, manifest_sha256, hino_id, codigo, numero, titulo_destino, letra_nova, creditos_alterados, revertido_em)
		SELECT 'batch1', 'sha1', id, 'CC', 2, 'Com Letra Revertido', 'Letra', 0, '2026-10-18 00:00:00'
		FROM hino WHERE coletanea_id = 1 AND numeracao = 2;
	`); err != nil {
		t.Fatalf("inserir logs: %v", err)
	}

	// Nem hino 1 (sem letra) nem hino 2 (revertido) devem exibir o selo
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/slides?codigo=CC", nil))
	if strings.Contains(rec.Body.String(), "Letra importada — confira") {
		t.Error("hino sem letra ou import revertido não devem exibir selo")
	}
}

func TestCardCorFundoSemConteudo(t *testing.T) {
	w, conn := setupWeb(t)

	// Atualiza hino 2 para ter letra apenas com whitespace ASCII
	if _, err := conn.Exec(`UPDATE hino SET letra = ? WHERE coletanea_id = 1 AND numeracao = 2`, "   \t\n   "); err != nil {
		t.Fatalf("update whitespace letra: %v", err)
	}

	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/slides?codigo=CC", nil))
	body := rec.Body.String()

	// Hino 1 (nil) e Hino 2 (whitespace) devem ter cor de fundo #FFB7B2 (sem letra)
	countSemLetra := strings.Count(body, "background-color: #FFB7B2")
	if countSemLetra != 2 {
		t.Errorf("esperado 2 cards com background-color: #FFB7B2 (nil e whitespace), encontrado %d", countSemLetra)
	}

	// Hino 3 (revisado) deve ter #B5EAD7
	if !strings.Contains(body, "background-color: #B5EAD7") {
		t.Error("hino 3 (revisado) deveria ter background-color: #B5EAD7")
	}
}

