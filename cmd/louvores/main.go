package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adjoli/louvores-go/internal/api"
	"github.com/adjoli/louvores-go/internal/app"
	"github.com/adjoli/louvores-go/internal/web"
)

// main é o ponto de entrada do binário: inicia o servidor HTTP da API.
//
// Mantém-se propositalmente enxuta: apenas cria o container de dependências
// (composition root em internal/app), monta as rotas (internal/api) e sobe
// o servidor com graceful shutdown em SIGINT/SIGTERM. Toda a montagem de
// camadas vive em app.New; o main não conhece config, banco nem services.
func main() {
	aplicacao, err := app.New()
	if err != nil {
		slog.Error("inicializar aplicação", "erro", err)
		os.Exit(1)
	}
	defer aplicacao.Close()

	// Registra o logger do App (arquivo + console) como logger global, para
	// que chamadas a slog.Error em qualquer pacote (ex.: api) caiam no
	// logs/app.log e não apenas no stderr.
	slog.SetDefault(aplicacao.Logger())

	// API REST (JSON) e interface web (HTML/templ+HTMX) compartilham o mesmo
	// servidor: a API é delegada para "/", e as rotas web (/stats, /slides,
	// /static) são mais específicas, portanto ganham prioridade no mux raiz.
	handler := web.New(
		api.New(aplicacao.HinoService(), aplicacao.StatsService()).Routes(),
		aplicacao.HinoService(),
		aplicacao.StatsService(),
	).Routes()

	srv := &http.Server{
		Addr:    aplicacao.Config().Addr(),
		Handler: handler,
	}

	// O contexto é cancelado no primeiro SIGINT/SIGTERM, disparando o
	// shutdown ordenado do servidor.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		aplicacao.Logger().Info("servidor HTTP iniciado", "addr", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		aplicacao.Logger().Info("encerrando servidor", "addr", srv.Addr)
		if err := srv.Shutdown(shutdownCtx); err != nil {
			aplicacao.Logger().Error("shutdown do servidor", "erro", err)
		}
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			aplicacao.Logger().Error("servidor falhou", "erro", err)
			os.Exit(1)
		}
	}
}
