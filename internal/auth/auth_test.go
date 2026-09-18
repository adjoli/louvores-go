package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adjoli/louvores-go/internal/config"
)

// newService cria um serviço de autenticação para os testes.
func newService(senha string) *Service {
	return New(&config.Config{
		AuthPassword:  senha,
		SessionSecret: "test-secret",
		SessionTTL:    time.Hour,
	})
}

// TestDisabledWithoutPassword garante que AUTH_PASSWORD vazia desliga a
// autenticação: tudo é considerado autenticado e qualquer senha é aceita.
func TestDisabledWithoutPassword(t *testing.T) {
	s := newService("")

	if s.Enabled() {
		t.Error("Enabled() = true, esperado false sem senha")
	}
	if !s.Check("qualquer") {
		t.Error("Check() deveria aceitar qualquer senha com autenticação desligada")
	}
	if !s.Authenticated(httptest.NewRequest("GET", "/stats", nil)) {
		t.Error("Authenticated() = false, esperado true com autenticação desligada")
	}
}

// TestCheckPassword valida a comparação da senha configurada.
func TestCheckPassword(t *testing.T) {
	s := newService("segredo")

	if !s.Check("segredo") {
		t.Error("Check() = false para a senha correta")
	}
	if s.Check("errado") {
		t.Error("Check() = true para senha incorreta")
	}
	if s.Check("") {
		t.Error("Check() = true para senha vazia")
	}
}

// TestSessionRoundTrip verifica que o cookie emitido é aceito.
func TestSessionRoundTrip(t *testing.T) {
	s := newService("segredo")

	rec := httptest.NewRecorder()
	s.SetSession(rec)

	cookie := cookieDaResposta(t, rec)
	if cookie == nil || cookie.Value == "" {
		t.Fatal("SetSession não emitiu cookie")
	}
	if !cookie.HttpOnly {
		t.Error("cookie deveria ser HttpOnly")
	}

	req := httptest.NewRequest("GET", "/stats", nil)
	req.AddCookie(cookie)
	if !s.Authenticated(req) {
		t.Error("sessão recém-emitida deveria ser válida")
	}
}

// TestSessionTampered rejeita um token com assinatura adulterada.
func TestSessionTampered(t *testing.T) {
	s := newService("segredo")

	rec := httptest.NewRecorder()
	s.SetSession(rec)
	cookie := cookieDaResposta(t, rec)

	// Altera o último caractere da assinatura.
	cookie.Value = cookie.Value[:len(cookie.Value)-1] + "0"

	req := httptest.NewRequest("GET", "/stats", nil)
	req.AddCookie(cookie)
	if s.Authenticated(req) {
		t.Error("token adulterado deveria ser rejeitado")
	}
}

// TestSessionExpired rejeita um cookie cujo prazo já passou.
func TestSessionExpired(t *testing.T) {
	s := newService("segredo")
	s.ttl = -time.Minute

	rec := httptest.NewRecorder()
	s.SetSession(rec)
	cookie := cookieDaResposta(t, rec)

	req := httptest.NewRequest("GET", "/stats", nil)
	req.AddCookie(cookie)
	if s.Authenticated(req) {
		t.Error("sessão expirada deveria ser rejeitada")
	}
}

// TestMiddlewarePublicPaths deixa passar as rotas públicas.
func TestMiddlewarePublicPaths(t *testing.T) {
	s := newService("segredo")

	for _, path := range []string{"/login", "/api/healthz", "/static/main.css"} {
		chamou := false
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { chamou = true })
		rec := httptest.NewRecorder()
		s.Middleware(next).ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if !chamou {
			t.Errorf("rota pública %q foi bloqueada", path)
		}
	}
}

// TestMiddlewareRedirectsBrowser redireciona navegadores para /login.
func TestMiddlewareRedirectsBrowser(t *testing.T) {
	s := newService("segredo")
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler protegido não deveria ser chamado sem sessão")
	})

	rec := httptest.NewRecorder()
	s.Middleware(next).ServeHTTP(rec, httptest.NewRequest("GET", "/stats", nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, esperado 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, esperado /login", loc)
	}
}

// TestMiddlewareAPI401 responde 401 JSON para rotas de API sem sessão.
func TestMiddlewareAPI401(t *testing.T) {
	s := newService("segredo")
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler de API não deveria ser chamado sem sessão")
	})

	rec := httptest.NewRecorder()
	s.Middleware(next).ServeHTTP(rec, httptest.NewRequest("GET", "/api/stats", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "não autenticado") {
		t.Errorf("corpo = %q", rec.Body.String())
	}
}

// TestMiddlewareAllowsAuthenticated deixa passar uma requisição com cookie.
func TestMiddlewareAllowsAuthenticated(t *testing.T) {
	s := newService("segredo")

	rec := httptest.NewRecorder()
	s.SetSession(rec)
	cookie := cookieDaResposta(t, rec)

	chamou := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { chamou = true })

	req := httptest.NewRequest("GET", "/stats", nil)
	req.AddCookie(cookie)
	s.Middleware(next).ServeHTTP(httptest.NewRecorder(), req)

	if !chamou {
		t.Error("requisição autenticada deveria passar pelo middleware")
	}
}

// cookieDaResposta localiza o cookie de sessão na resposta.
func cookieDaResposta(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			return c
		}
	}
	return nil
}
