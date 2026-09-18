package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/adjoli/louvores-go/internal/auth"
)

// sessaoCookie devolve o cookie de sessão emitido na resposta (ou nil).
func sessaoCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c
		}
	}
	return nil
}

// TestRotaProtegidaSemSessaoRedireciona garante que, com autenticação ativa,
// uma página protegida redireciona o navegador para /login.
func TestRotaProtegidaSemSessaoRedireciona(t *testing.T) {
	w, _ := setupWebComSenha(t, "segredo")

	req := httptest.NewRequest("GET", "/stats", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, esperado 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, esperado /login", loc)
	}
}

// TestAPIProtegidaSemSessaoRetorna401 garante 401 JSON (sem redirect) para
// requisições de API não autenticadas.
func TestAPIProtegidaSemSessaoRetorna401(t *testing.T) {
	w, _ := setupWebComSenha(t, "segredo")

	req := httptest.NewRequest("GET", "/api/stats", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "não autenticado") {
		t.Errorf("corpo = %q, esperado mensagem de não autenticado", rec.Body.String())
	}
}

// TestLoginComSenhaCorreta emite o cookie de sessão e redireciona para a raiz.
func TestLoginComSenhaCorreta(t *testing.T) {
	w, _ := setupWebComSenha(t, "segredo")

	form := url.Values{"senha": {"segredo"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, esperado 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/slides" {
		t.Errorf("Location = %q, esperado /slides", loc)
	}
	if c := sessaoCookie(t, rec); c == nil || c.Value == "" {
		t.Error("cookie de sessão não foi emitido")
	}
}

// TestLoginJaAutenticadoRedirecionaParaSlides garante que acessar /login com
// sessão válida não re-renderiza o formulário, e sim vai para /slides.
func TestLoginJaAutenticadoRedirecionaParaSlides(t *testing.T) {
	w, _ := setupWebComSenha(t, "segredo")

	form := url.Values{"senha": {"segredo"}}
	loginReq := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRec := httptest.NewRecorder()
	w.Routes().ServeHTTP(loginRec, loginReq)

	cookie := sessaoCookie(t, loginRec)
	if cookie == nil {
		t.Fatal("login não emitiu cookie de sessão")
	}

	req := httptest.NewRequest("GET", "/login", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, esperado 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/slides" {
		t.Errorf("Location = %q, esperado /slides", loc)
	}
}

// TestRaizAutenticadaRedirecionaParaSlides garante que GET / com sessão válida
// vai para /slides (o destino do logo no cabeçalho).
func TestRaizAutenticadaRedirecionaParaSlides(t *testing.T) {
	w, _ := setupWebComSenha(t, "segredo")

	form := url.Values{"senha": {"segredo"}}
	loginReq := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRec := httptest.NewRecorder()
	w.Routes().ServeHTTP(loginRec, loginReq)

	cookie := sessaoCookie(t, loginRec)
	if cookie == nil {
		t.Fatal("login não emitiu cookie de sessão")
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, esperado 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/slides" {
		t.Errorf("Location = %q, esperado /slides", loc)
	}
}

// TestRaizSemSessaoRedirecionaParaLogin garante que a raiz continua protegida
// quando a autenticação está ativa.
func TestRaizSemSessaoRedirecionaParaLogin(t *testing.T) {
	w, _ := setupWebComSenha(t, "segredo")

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, esperado 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, esperado /login", loc)
	}
}

// TestLoginComSenhaErrada retorna 401 e a mensagem de erro, sem cookie.
func TestLoginComSenhaErrada(t *testing.T) {
	w, _ := setupWebComSenha(t, "segredo")

	form := url.Values{"senha": {"errada"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Senha incorreta") {
		t.Error("página de login não exibe a mensagem de erro")
	}
	if c := sessaoCookie(t, rec); c != nil {
		t.Error("cookie de sessão não deveria ser emitido com senha errada")
	}
}

// TestAcessaAposLogin garante que, com o cookie emitido no login, uma rota
// protegida responde 200.
func TestAcessaAposLogin(t *testing.T) {
	w, _ := setupWebComSenha(t, "segredo")

	form := url.Values{"senha": {"segredo"}}
	loginReq := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRec := httptest.NewRecorder()
	w.Routes().ServeHTTP(loginRec, loginReq)

	cookie := sessaoCookie(t, loginRec)
	if cookie == nil {
		t.Fatal("login não emitiu cookie de sessão")
	}

	req := httptest.NewRequest("GET", "/stats", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200 após login", rec.Code)
	}
}

// TestLogoutLimpaSessao redireciona para /login e invalida o cookie.
func TestLogoutLimpaSessao(t *testing.T) {
	w, _ := setupWebComSenha(t, "segredo")

	// Faz login para obter uma sessão válida e então encerra.
	form := url.Values{"senha": {"segredo"}}
	loginReq := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRec := httptest.NewRecorder()
	w.Routes().ServeHTTP(loginRec, loginReq)

	cookie := sessaoCookie(t, loginRec)
	if cookie == nil {
		t.Fatal("login não emitiu cookie de sessão")
	}

	req := httptest.NewRequest("POST", "/logout", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	w.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, esperado 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, esperado /login", loc)
	}

	// O cookie de limpeza tem Max-Age=0; Response.Cookies() o descarta, por
	// isso inspecionamos o header Set-Cookie cru.
	setCookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, auth.CookieName+"=") {
		t.Fatalf("logout não emitiu cookie de sessão: %q", setCookie)
	}
	if !strings.Contains(setCookie, "Max-Age=0") {
		t.Errorf("cookie de sessão não foi invalidado: %q", setCookie)
	}
}
