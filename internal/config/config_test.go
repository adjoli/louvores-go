package config

import (
	"path/filepath"
	"testing"
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
