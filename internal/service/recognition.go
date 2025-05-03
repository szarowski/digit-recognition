package service

import (
	"bufio"
	"bytes"
	"digit-recognition/internal/api/gen"
	"encoding/csv"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"strconv"
	"strings"
)

// StopService - Stops gracefully the Recognition Service (close Redis connection)
func (s *RecognitionService) StopService() error {
	err := s.rdb.Close()
	if err != nil {
		s.logger.Errorf("Could not stop Redis Connection: %v", err)
		return err
	}
	return nil
}

// TrainRecognition - Train Recognition by creating 60 000 Vector indices
func (s *RecognitionService) TrainRecognition() error {
	err := s.createIndex(s.rdb)
	if err != nil {
		if strings.Contains(err.Error(), "Index already exists") {
			s.logger.Warn("Index already exists")
		} else {
			s.logger.Errorf("Could not create search index: %v", err)
			return err
		}
	} else {
		s.logger.Info("Index Created")
	}

	go func() {
		err = s.storeData()
		if err != nil {
			err = s.StopService()
			if err != nil {
				s.logger.Errorf("Could not stop recognition service gracefully: %v", err)
			}
		}
	}()

	return nil
}

// ReadDigitSamples - Read 10 000 predefined test digit samples
func (s *RecognitionService) ReadDigitSamples() error {
	// Open the MNIST test CSV file
	file, err := os.Open(MnistTestCsv)
	if err != nil {
		s.logger.Errorf("Could not open MNIST Test CSV file: %v", err)
		return err
	}
	defer func(file *os.File) {
		err = file.Close()
		if err != nil {
			s.logger.Errorf("Could not close MNIST Test CSV file: %v", err)
		}
	}(file)

	// Create a CSV reader
	reader := csv.NewReader(bufio.NewReader(file))

	// Read each record from the CSV file
	s.records, err = reader.ReadAll()
	if err != nil {
		s.logger.Errorf("Error reading MNIST Test CSV file: %v", err)
		return err
	}
	return nil
}

// GetIndexStatus - Get train status of the Recognition training out of 60 000 Vector indices
func (s *RecognitionService) GetIndexStatus() *gen.IndexStatus {
	return &gen.IndexStatus{
		IndexCount:    IndexCount,
		IndicesStored: s.lastIndexStored,
	}
}

// GetDigitSample - Get one of 10 000 digit image samples by index
func (s *RecognitionService) GetDigitSample(index int) (io.Reader, *int, *int64, error) {
	// The rest are pixel values
	s.row = s.records[index]
	data := s.records[index][1:]

	if len(data) < 2 { // Need at least a label and one pixel
		s.logger.Errorf("Could not retrieve a digit sample from the MNIST Test CSV.")
		return nil, nil, nil, ErrBadRequest
	}

	pixels := make([]uint8, len(data))
	for j := 0; j < len(data); j++ {
		pixelVal, err := strconv.Atoi(data[j])
		if err != nil || pixelVal < 0 || pixelVal > 255 {
			s.logger.Error("Could not retrieve a digit sample from the MNIST Test CSV.")
			s.logger.Debugf("Skipping pixel %d in row %d due to invalid value: %v\n", j, index, err)
			pixels[j] = 0 // Default to black
			continue
		}
		pixels[j] = uint8(pixelVal)
	}

	img := image.NewGray(image.Rect(0, 0, ExpectedWidth, ExpectedHeight))
	for y := 0; y < ExpectedHeight; y++ {
		for x := 0; x < ExpectedWidth; x++ {
			p := image.Point{X: x, Y: y}
			img.SetGray(p.X, p.Y, color.Gray{Y: pixels[y*ExpectedWidth+x]})
		}
	}
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	if err != nil {
		s.logger.Errorf("Could not encode to PNG a digit sample from the MNIST Test CSV: %v", err)
		return nil, nil, nil, err
	}

	digit, err := strconv.Atoi(s.records[index][0])
	if err != nil {
		s.logger.Errorf("Could not retrieve a digit representation of the digit sample from the MNIST Test CSV: %v", err)
		return nil, nil, nil, err
	}

	var contentLength = int64(buf.Len())

	return &buf, &digit, &contentLength, nil
}

// RecognizeImage - Recognize provided image in POST method body and return recognition Statistics
func (s *RecognitionService) RecognizeImage(digitImage io.Reader, digitValue int8, index int) (*gen.Statistics, error) {
	img, err := png.Decode(digitImage)
	if err != nil {
		s.logger.Errorf("Could not retrieve a digit representation of the digit sample from the MNIST Test CSV: %v", err)
		return nil, err
	}

	bounds := img.Bounds()
	pixels := []string{strconv.Itoa(int(digitValue))}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray, ok := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			if ok {
				pixels = append(pixels, strconv.Itoa(int(gray.Y)))
			} else {
				// Handle cases where the image is not grayscale
				s.logger.Warnf("Pixel at (%d, %d) is not grayscale.", x, y)
				pixels = append(pixels, "0") // Default to black
			}
		}
	}

	statistics, err := s.searchData(index, pixels)
	if err != nil {
		s.logger.Errorf("Could not search data: %v", err)
		return nil, err
	}

	return statistics, nil
}
