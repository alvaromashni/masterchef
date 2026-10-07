// Comando painel: sobe o painel local de visão de produto.
//
// Aqui só ligamos as peças (config, store, servidor web). A lógica de
// verdade fica nos pacotes de internal/.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alvaromashni/masterchef/internal/collector"
	"github.com/alvaromashni/masterchef/internal/config"
	"github.com/alvaromashni/masterchef/internal/github"
	"github.com/alvaromashni/masterchef/internal/linear"
	"github.com/alvaromashni/masterchef/internal/store"
	"github.com/alvaromashni/masterchef/internal/web"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if err := run(logger); err != nil {
		// fmt em vez de slog aqui: erros de config vêm em várias linhas,
		// e o slog as escaparia como "\n", deixando a lista ilegível.
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}
}

// run existe separado de main para podermos usar "return err" normalmente
// e deixar o os.Exit num lugar só.
func run(logger *slog.Logger) error {
	configPath := flag.String("config", "config.yaml", "caminho do config.yaml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	st, err := store.Open(cfg.DBPath, logger)
	if err != nil {
		return err
	}
	defer st.Close()

	srv, err := web.New(cfg, st, logger)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:    cfg.Listen,
		Handler: srv.Handler(),
		// Protege contra conexões que nunca terminam de mandar os cabeçalhos.
		ReadHeaderTimeout: 10 * time.Second,
	}

	// ctx é cancelado quando o usuário aperta Ctrl+C (SIGINT) ou o sistema
	// pede para encerrar (SIGTERM). Usamos isso para desligar com calma.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// O coletor roda em paralelo ao servidor: um ciclo agora e outro a cada
	// poll_interval. Ele para sozinho quando ctx é cancelado (Ctrl+C).
	coletor := collector.New(cfg,
		linear.New(cfg.LinearAPIKey, ""),
		github.New(cfg.GitHubToken, "", logger),
		st, logger)
	go coletor.Run(ctx)

	// ListenAndServe bloqueia, então roda numa goroutine e avisa erros pelo canal.
	erroServidor := make(chan error, 1)
	go func() {
		logger.Info("painel no ar", "endereco", "http://"+cfg.Listen)
		erroServidor <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-erroServidor:
		// ErrServerClosed é o "erro" normal de quando pedimos para fechar.
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("encerrando...")
	}

	// Dá até 5s para as requisições em andamento terminarem.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}
