package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNewDefaultsToLocalSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hinos.db")
	t.Setenv(EnvDatabasePath, dbPath)
	t.Setenv(EnvTursoURL, "")
	t.Setenv(EnvTursoAuthToken, "")

	cfg, err := New()
	if err != nil {
		t.Fatalf("New() erro inesperado: %v", err)
	}

	if cfg.UsarTurso() {
		t.Error("UsarTurso() = true, esperado false sem TURSO_DATABASE_URL")
	}
	if cfg.DBPath != dbPath {
		t.Errorf("DBPath = %q, esperado %q", cfg.DBPath, dbPath)
	}
}

func TestNewUsesTursoWhenConfigured(t *testing.T) {
	t.Setenv(EnvDatabasePath, filepath.Join(t.TempDir(), "hinos.db"))
	t.Setenv(EnvTursoURL, "libsql://exemplo.turso.io")
	t.Setenv(EnvTursoAuthToken, "token-de-teste")

	cfg, err := New()
	if err != nil {
		t.Fatalf("New() erro inesperado: %v", err)
	}

	if !cfg.UsarTurso() {
		t.Error("UsarTurso() = false, esperado true com TURSO_DATABASE_URL definida")
	}
	if cfg.TursoURL != "libsql://exemplo.turso.io" {
		t.Errorf("TursoURL = %q", cfg.TursoURL)
	}
	if cfg.TursoAuthToken != "token-de-teste" {
		t.Errorf("TursoAuthToken = %q", cfg.TursoAuthToken)
	}
}

func TestNewRejectsTursoWithoutToken(t *testing.T) {
	t.Setenv(EnvDatabasePath, filepath.Join(t.TempDir(), "hinos.db"))
	t.Setenv(EnvTursoURL, "libsql://exemplo.turso.io")
	t.Setenv(EnvTursoAuthToken, "")

	if _, err := New(); err == nil {
		t.Fatal("New() deveria falhar com TURSO_DATABASE_URL sem TURSO_AUTH_TOKEN")
	}
}

func TestNewReadsAuthSettings(t *testing.T) {
	t.Setenv(EnvDatabasePath, filepath.Join(t.TempDir(), "hinos.db"))
	t.Setenv(EnvAuthPassword, "segredo")
	t.Setenv(EnvSessionSecret, "chave")
	t.Setenv(EnvCookieSecure, "true")
	t.Setenv(EnvSessionTTL, "12h")

	cfg, err := New()
	if err != nil {
		t.Fatalf("New() erro inesperado: %v", err)
	}

	if !cfg.AuthEnabled() {
		t.Error("AuthEnabled() = false, esperado true com AUTH_PASSWORD definida")
	}
	if cfg.AuthPassword != "segredo" {
		t.Errorf("AuthPassword = %q", cfg.AuthPassword)
	}
	if cfg.SessionSecret != "chave" {
		t.Errorf("SessionSecret = %q", cfg.SessionSecret)
	}
	if !cfg.CookieSecure {
		t.Error("CookieSecure = false, esperado true")
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Errorf("SessionTTL = %v, esperado 12h", cfg.SessionTTL)
	}
}

func TestNewDefaultsAuthDisabled(t *testing.T) {
	t.Setenv(EnvDatabasePath, filepath.Join(t.TempDir(), "hinos.db"))
	t.Setenv(EnvAuthPassword, "")
	t.Setenv(EnvSessionSecret, "")
	t.Setenv(EnvCookieSecure, "")
	t.Setenv(EnvSessionTTL, "")

	cfg, err := New()
	if err != nil {
		t.Fatalf("New() erro inesperado: %v", err)
	}

	if cfg.AuthEnabled() {
		t.Error("AuthEnabled() = true, esperado false sem AUTH_PASSWORD")
	}
	if cfg.SessionTTL != DefaultSessionTTL {
		t.Errorf("SessionTTL = %v, esperado default %v", cfg.SessionTTL, DefaultSessionTTL)
	}
	if cfg.CookieSecure {
		t.Error("CookieSecure = true, esperado false por padrão")
	}
}

func TestNewRejectsInvalidCookieSecure(t *testing.T) {
	t.Setenv(EnvDatabasePath, filepath.Join(t.TempDir(), "hinos.db"))
	t.Setenv(EnvCookieSecure, "talvez")

	if _, err := New(); err == nil {
		t.Fatal("New() deveria falhar com COOKIE_SECURE inválido")
	}
}
