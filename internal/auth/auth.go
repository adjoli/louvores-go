// Package auth implementa a autenticação da aplicação: senha única
// compartilhada e sessão por cookie assinado (HMAC-SHA256).
//
// O modelo é intencionalmente simples: não há tabela de usuários nem banco
// envolvido. A senha vive em AUTH_PASSWORD e o cookie carrega apenas o
// instante de expiração, assinado com SESSION_SECRET. O middleware protege
// todas as rotas, exceto as públicas (login, healthz e estáticos).
//
// Quando AUTH_PASSWORD está vazia, a autenticação fica desligada (Enabled
// retorna false) e o middleware deixa tudo passar — útil em desenvolvimento
// local. Em produção, defina a senha.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/adjoli/louvores-go/internal/config"
)

// CookieName é o nome do cookie que carrega a sessão assinada.
const CookieName = "louvores_sessao"

// Service valida a senha e gerencia o ciclo de vida da sessão (emitir,
// validar e limpar o cookie), além de expor o middleware de proteção.
type Service struct {
	password     []byte
	secret       []byte
	cookieSecure bool
	ttl          time.Duration
}

// New cria o serviço de autenticação a partir da configuração. Se
// SESSION_SECRET não estiver definida, gera uma chave aleatória em runtime
// (as sessões deixam de sobreviver ao restart) e registra um aviso.
func New(cfg *config.Config) *Service {
	secret := []byte(cfg.SessionSecret)
	if len(secret) == 0 {
		secret = randomSecret()
		slog.Warn("SESSION_SECRET não definida; usando chave aleatória (sessões caem no restart)")
	}

	return &Service{
		password:     []byte(cfg.AuthPassword),
		secret:       secret,
		cookieSecure: cfg.CookieSecure,
		ttl:          cfg.SessionTTL,
	}
}

// Enabled indica se a autenticação está ativa (AUTH_PASSWORD definida).
func (s *Service) Enabled() bool {
	return len(s.password) > 0
}

// Check compara a senha informada com a configurada em tempo constante.
// Com a autenticação desligada, qualquer senha é aceita.
func (s *Service) Check(senha string) bool {
	if !s.Enabled() {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(senha), s.password) == 1
}

// Authenticated indica se a requisição traz um cookie de sessão válido.
// Com a autenticação desligada, sempre retorna true.
func (s *Service) Authenticated(r *http.Request) bool {
	if !s.Enabled() {
		return true
	}
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return false
	}
	return s.verify(cookie.Value)
}

// SetSession emite o cookie de sessão assinado com a expiração configurada.
func (s *Service) SetSession(w http.ResponseWriter) {
	exp := time.Now().Add(s.ttl).Unix()
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    s.sign(exp),
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.ttl.Seconds()),
	})
}

// ClearSession invalida o cookie de sessão no navegador.
func (s *Service) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// Middleware protege o handler: deixa passar requisições autenticadas e as
// rotas públicas; para as demais, responde 401 JSON em /api/* e redireciona
// navegadores para /login (303).
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPath(r.URL.Path) || s.Authenticated(r) {
			next.ServeHTTP(w, r)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"não autenticado"}`))
			return
		}

		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
}

// publicPath informa se o caminho pode ser acessado sem autenticação.
func publicPath(path string) bool {
	switch {
	case path == "/login", path == "/api/healthz":
		return true
	case strings.HasPrefix(path, "/static/"):
		return true
	default:
		return false
	}
}

// sign monta o token "<expiração>.<hmac>", assinando a expiração com a
// chave secreta.
func (s *Service) sign(exp int64) string {
	payload := strconv.FormatInt(exp, 10)

	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))

	return payload + "." + sig
}

// verify valida a assinatura e a expiração do token. Qualquer token
// malformado, adulterado ou vencido é rejeitado.
func (s *Service) verify(token string) bool {
	payload, sig, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}

	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
		return false
	}

	exp, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < exp
}

// randomSecret gera 32 bytes aleatórios para uso como chave HMAC.
// crypto/rand.Read só falha em cenários catastróficos; nesse caso não há
// como seguir com segurança, então o processo é encerrado.
func randomSecret() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("auth: falha ao gerar chave de sessão aleatória: " + err.Error())
	}
	return b
}
