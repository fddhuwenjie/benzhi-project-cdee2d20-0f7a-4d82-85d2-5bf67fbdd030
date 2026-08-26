package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/httpapi"
	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/store"
)

type application struct {
	store      *store.Store
	httpServer *http.Server
}

func buildApplication(ctx context.Context, cfg config) (*application, error) {
	repository, err := store.Open(ctx, cfg.databasePath)
	if err != nil {
		return nil, fmt.Errorf("初始化存储: %w", err)
	}
	missionService := mission.NewService(repository)
	auditService := audit.NewService(repository)
	handler := httpapi.New(missionService, auditService)
	server := &http.Server{Addr: cfg.address, Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	return &application{store: repository, httpServer: server}, nil
}

func (a *application) close() error { return a.store.Close() }
