package internal

import (
	"context"
	"digit-recognition/internal/api"
	"digit-recognition/internal/api/gen"
	"digit-recognition/internal/controller"
	"digit-recognition/internal/logging"
	"digit-recognition/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"net/http"
	"path/filepath"
	"runtime"
)

// NewApplication - Create an Application
func NewApplication(workDir string) {
	// Create a logger
	logger := logging.NewLogger()

	// Log the available resources
	logger.Infof("MaxProcs: %d", runtime.GOMAXPROCS(-1))
	logger.Infof("NumCPU: %d", runtime.NumCPU())

	// Create a service
	ctx := context.Background()
	recognitionService := service.NewRecognitionService(ctx, logger)
	// Create a controller supporting all endpoints in the openapi.yaml file
	recognitionController := controller.NewRecognitionController(logger, recognitionService)

	// Create a router for Chi Http Server
	router := chi.NewRouter()

	// Use some basic middlewares
	router.Use(middleware.Recoverer)
	router.Use(middleware.RequestID)
	router.Use(middleware.AllowContentType("image/png"))

	// Serve index.html located in the root of the repository
	indexPath := filepath.Join(workDir, "index.html")
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, indexPath)
	})

	// Assign the strict controller to the router
	gen.HandlerFromMux(gen.NewStrictHandler(recognitionController, nil), router)

	// Create and run the HTTP Server
	server := api.NewHttpServer(logger, router)
	server.Start()

	// Train the digit recognition in the background
	err := recognitionService.TrainRecognition()
	if err != nil {
		logger.Errorf("Failed training recognition: %v", err)
		server.Stop(ctx, recognitionService)
	}
	// Read 10 000 digit samples
	err = recognitionService.ReadDigitSamples()
	if err != nil {
		logger.Errorf("Failed reading digit samples for recognition: %v", err)
		server.Stop(ctx, recognitionService)
	}

	// Block the shutdown unless required
	server.WaitForShutdown()

	// Stop the recognition web service and http server
	server.Stop(ctx, recognitionService)
}
