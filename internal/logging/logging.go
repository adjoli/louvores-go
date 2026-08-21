// Package logging configura o logger estruturado (log/slog) da aplicação.
// A saída é fan-out para o arquivo de log e o console simultaneamente,
// com nível INFO.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Setup cria o logger da aplicação, gravando cada registro tanto em
// logPath (arquivo criado se necessário, modo append) quanto no stdout.
func Setup(logPath string) (*slog.Logger, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("criar diretório de logs: %w", err)
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("abrir arquivo de log: %w", err)
	}
	h := slog.NewTextHandler(io.MultiWriter(f, os.Stdout), &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(h), nil
}
