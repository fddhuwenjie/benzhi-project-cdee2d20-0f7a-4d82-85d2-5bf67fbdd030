package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func run(ctx context.Context, cfg config) error {
	if cfg.selfCheck {
		return runSelfCheck(ctx, cfg)
	}
	app, err := buildApplication(ctx, cfg)
	if err != nil {
		return err
	}
	defer app.close()
	listener, err := net.Listen("tcp", cfg.address)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.address, err)
	}
	serverErrors := make(chan error, 1)
	go func() {
		err := app.httpServer.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
			return
		}
		serverErrors <- nil
	}()
	signalContext, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serverErrors:
		return err
	case <-signalContext.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.httpServer.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("关闭 HTTP 服务: %w", err)
		}
		return <-serverErrors
	}
}
