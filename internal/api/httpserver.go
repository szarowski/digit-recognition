package api

import (
	"context"
	"digit-recognition/internal/service"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type HttpServer struct {
	logger *logrus.Logger
	server *http.Server
}

// NewHttpServer - Set up the Chi HTTP Server
func NewHttpServer(logger *logrus.Logger, r *chi.Mux) *HttpServer {
	logger.Info("Configuring the server...")

	server := http.Server{
		Addr:              ":8880",
		Handler:           r,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	return &HttpServer{logger: logger, server: &server}
}

// Start - Start the Chi HTTP server
func (hs *HttpServer) Start() {
	hs.logger.Info("Starting Chi HTTP Server...")
	go func() {
		if err := hs.server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			hs.logger.Panicln(err)
			hs.logger.Out = io.Discard // Disable logging
			hs.logger.Exit(1)
		}
	}()
	hs.logger.Infof("Listening on %s", hs.server.Addr)
}

func (hs *HttpServer) Stop(ctx context.Context, recognitionService *service.RecognitionService) {
	err := recognitionService.StopService()
	if err != nil {
		hs.logger.Fatalf("Service stop failed due to: %v", err)
	}

	// Create a deadline to wait for
	ctxWithTimeout, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	err = hs.server.Shutdown(ctxWithTimeout)
	if err != nil {
		hs.logger.Fatalf("Server shutdown failed due to: %v", err)
	}

	hs.logger.Info("Server gracefully shut down")

	hs.logger.Out = io.Discard // Disable logging.

	os.Exit(0)
}

// WaitForShutdown - Graceful shutdown of the HTTP Server
func (hs *HttpServer) WaitForShutdown() {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive our signal.
	<-interruptChan
}
