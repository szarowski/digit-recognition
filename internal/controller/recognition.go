package controller

import (
	"context"
	"digit-recognition/internal/api/gen"
)

// RetrieveDigit - Retrieve a sample image of a digit to recognize
func (r RecognitionController) RetrieveDigit(_ context.Context, request gen.RetrieveDigitRequestObject) (gen.RetrieveDigitResponseObject, error) {
	digitSampleReader, digit, contentLength, err := r.recognitionService.GetDigitSample(request.Params.Id)
	if err != nil {
		r.logger.Errorf("Could not retrieve a digit sample from the MNIST Test CSV: %v", err)
		return nil, err
	}
	return gen.RetrieveDigit200ImagepngResponse{
		Body: digitSampleReader,
		Headers: gen.RetrieveDigit200ResponseHeaders{
			XDigitExpected: *digit,
		},
		ContentLength: *contentLength,
	}, nil
}

// RecognizeDigit - Recognize a sample image of a digit and return Statistics
func (r RecognitionController) RecognizeDigit(_ context.Context, request gen.RecognizeDigitRequestObject) (gen.RecognizeDigitResponseObject, error) {
	statistics, err := r.recognitionService.RecognizeImage(request.Body, request.Digit, request.Id)
	if err != nil {
		r.logger.Errorf("Could not recognize a digit from the provided image: %v", err)
		return nil, err
	}
	return gen.RecognizeDigit200JSONResponse{
		RecognitionResponseJSONResponse: gen.RecognitionResponseJSONResponse(*statistics),
	}, nil
}

// TrainingStatus - Make sure that all 60 000 indices are stored in Redis - A sort of heartbeat
func (r RecognitionController) TrainingStatus(_ context.Context, _ gen.TrainingStatusRequestObject) (gen.TrainingStatusResponseObject, error) {
	indexStatus := r.recognitionService.GetIndexStatus()
	return gen.TrainingStatus200JSONResponse{
		StatusResponseJSONResponse: gen.StatusResponseJSONResponse(*indexStatus),
	}, nil
}
