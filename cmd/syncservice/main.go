package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/httpapi"
	appservice "github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/service"
	"github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/storage"
)

func main() {
	addr := flag.String("addr", ":8096", "HTTP listen address")
	dataRoot := flag.String("data", "./var/sync-conformer", "persistent data root")
	flag.Parse()

	repository, err := storage.New(*dataRoot)
	if err != nil {
		log.Fatalf("initialize storage: %v", err)
	}
	api := httpapi.New(appservice.New(repository))
	server := &http.Server{
		Addr:              *addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       45 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Printf("audiovisual sync conformer listening on %s", *addr)
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("HTTP server stopped: %v", serveErr)
			stop()
		}
	}()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
