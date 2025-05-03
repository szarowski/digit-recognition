package controller

import (
	"digit-recognition/internal/api/gen"
	"digit-recognition/internal/service"
	"github.com/sirupsen/logrus"
)

type ServerController struct {
	*RecognitionController
}

// Make sure that all endpoints are supported by this controller
var _ gen.StrictServerInterface = (*ServerController)(nil)

type RecognitionController struct {
	logger             *logrus.Logger
	recognitionService *service.RecognitionService
}

// NewRecognitionController - Create a RecognitionController
func NewRecognitionController(logger *logrus.Logger, recognitionService *service.RecognitionService) *RecognitionController {
	return &RecognitionController{
		logger:             logger,
		recognitionService: recognitionService,
	}
}
