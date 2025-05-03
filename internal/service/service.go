package service

import (
	"bufio"
	"bytes"
	"context"
	"digit-recognition/internal/api/gen"
	"encoding/binary"
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	ExpectedWidth  = 28
	ExpectedHeight = 28
	IndexCount     = 60000
	RedisAddress   = "localhost:6379"
	RedisPassword  = "redispassword"
	RedisDefaultDb = 0
	RedisProtocol  = 2
	MnistTrainCsv  = "mnist_train.csv"
	MnistTestCsv   = "mnist_test.csv"
)

const (
	ErrorBadRequest = "BAD_REQUEST"
)

// ErrBadRequest Internal errors
var (
	ErrBadRequest = errors.New(ErrorBadRequest)
)

type RecognitionService struct {
	ctx    context.Context
	rdb    *redis.Client
	logger *logrus.Logger

	//
	lastIndexStored int
	records         [][]string
	row             []string
	minDuration     int64
	maxDuration     int64
	totalDuration   int64
	correctGuesses  int
	wrongGuesses    int
}

// NewRecognitionService - Create a RecognitionService
func NewRecognitionService(ctx context.Context, logger *logrus.Logger) *RecognitionService {
	// Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     RedisAddress,
		Password: RedisPassword,
		DB:       RedisDefaultDb,
		Protocol: RedisProtocol,
	})

	return &RecognitionService{
		ctx:    ctx,
		rdb:    rdb,
		logger: logger,
	}
}

// createIndex - Create redis index for
// FT.CREATE mnist_index ON JSON PREFIX 1 number: SCHEMA $.embedding AS embedding VECTOR FLAT 6 DIM 784 DISTANCE_METRIC L2 TYPE FLOAT32
func (s *RecognitionService) createIndex(rdb *redis.Client) error {
	// Define the options for the FT.CREATE command
	options := &redis.FTCreateOptions{
		OnJSON: true,                          // Specify the key type is JSON (corresponds to "ON", "JSON")
		Prefix: []interface{}{"1", "number:"}, // Specify the key prefix(es) (corresponds to "PREFIX", "1", "number:")
	}
	schema := &redis.FieldSchema{
		FieldName: "$.embedding",
		As:        "embedding",
		FieldType: redis.SearchFieldTypeVector,
		VectorArgs: &redis.FTVectorArgs{
			FlatOptions: &redis.FTFlatOptions{
				Type:            "FLOAT32",
				Dim:             784,
				DistanceMetric:  "L2",
				InitialCapacity: 0,
				BlockSize:       6,
			},
		},
	}
	// Run FT.CREATE Redis command
	err := rdb.FTCreate(s.ctx, "mnist_index", options, schema).Err()
	return err
}

// storeData - Store data
func (s *RecognitionService) storeData() error {
	// Open the MNIST train CSV file
	file, err := os.Open(MnistTrainCsv)
	if err != nil {
		s.logger.Error("Could not store data: %v", err)
		return err
	}

	// Create a CSV reader
	reader := csv.NewReader(bufio.NewReader(file))

	// Read each record from the CSV file
	trainedRecords, err := reader.ReadAll()
	if err != nil {
		s.logger.Error("Could not store data: %v", err)
		return err
	}

	// Iterate over each row in the CSV file
	for i, record := range trainedRecords {
		// The first value is the result (the number)
		result, err := strconv.Atoi(record[0])
		if err != nil {
			s.logger.Error("Could not store data: %v", err)
			return err
		}

		// The rest are pixel values
		pixelValues := record[1:]

		// Convert pixel values to float32 and normalize them by dividing by 255
		var pixelStrings []string
		for _, pixel := range pixelValues {
			pixelInt, err := strconv.Atoi(pixel)
			if err != nil {
				s.logger.Error("Could not store data: %v", err)
				return err
			}
			// If the pixel value is 0, directly append "0", else format as float32 with 6 decimals
			if pixelInt == 0 {
				pixelStrings = append(pixelStrings, "0")
			} else {
				pixelFloat := float32(pixelInt) / 255.0
				pixelStrings = append(pixelStrings, fmt.Sprintf("%.6f", pixelFloat))
			}
		}
		embedding := strings.Join(pixelStrings, ",")

		// Create JSON data for Redis
		jsonData := fmt.Sprintf(`{"result": %d, "embedding": [%s]}`, result, embedding)
		s.logger.Debug("%s", jsonData)

		// Execute the JSON.SET command directly in Redis
		key := fmt.Sprintf("number:%d:%d", i, result)
		err = s.rdb.JSONSet(s.ctx, key, "$", jsonData).Err()
		if err != nil {
			s.logger.Error("Could not store data: %v", err)
			return err
		}
		s.logger.Infof("Stored JSON for number:%d:%d", i, result)
		// Store the last index to synchronize with the frontend
		s.lastIndexStored = i
	}

	return nil
}

// searchData - Search Redis data and retrieves Statistics as a result
func (s *RecognitionService) searchData(index int, record []string) (*gen.Statistics, error) {
	digitExpected, err := strconv.Atoi(record[0])
	if err != nil {
		return nil, err
	}

	// The rest are pixel values
	pixelValues := record[1:]

	// Convert pixel values to float32 and normalize them by dividing by 255
	var embedding []float32
	for _, pixel := range pixelValues {
		pixelInt, err := strconv.Atoi(pixel)
		if err != nil {
			return nil, err
		}
		// Normalize the pixel value
		pixelFloat := float32(pixelInt) / 255.0
		embedding = append(embedding, pixelFloat)
	}

	// Perform the FT.SEARCH query using the normalized embedding
	options := &redis.FTSearchOptions{
		SortBy: []redis.FTSearchSortBy{{
			FieldName: "dist",
			Asc:       true,
		}},
		DialectVersion: 2,
	}
	digitFound, duration, err := s.searchVectorInRedis(embedding, options)
	if err != nil {
		return nil, err
	}
	if duration < s.minDuration || s.minDuration == 0 {
		s.minDuration = duration
	}
	if duration > s.maxDuration {
		s.maxDuration = duration
	}
	s.totalDuration += duration
	// Print the expected result and the found label
	isCorrect := digitExpected == digitFound
	if digitExpected == digitFound {
		s.correctGuesses++
	} else {
		s.wrongGuesses++
	}
	result := &gen.Statistics{
		DigitIndex:     index,
		DigitExpected:  int8(digitExpected),
		DigitFound:     int8(digitFound),
		Success:        isCorrect,
		CorrectGuesses: s.correctGuesses,
		WrongGuesses:   s.wrongGuesses,
		Duration:       duration,
		MinDuration:    s.minDuration,
		MaxDuration:    s.maxDuration,
		TotalDuration:  s.totalDuration,
	}

	return result, nil
}

// searchVectorRedis - Search Vector in Redis
func (s *RecognitionService) searchVectorInRedis(embedding []float32, options *redis.FTSearchOptions) (int, int64, error) {
	// Convert the embedding to a byte slice (binary format)
	embeddingBytes, err := convertFloat32ArrayToBlob(embedding)
	if err != nil {
		return 0, 0, err
	}

	start := time.Now()

	options.Params = map[string]interface{}{"blob": embeddingBytes}
	result, err := s.rdb.FTSearchWithArgs(s.ctx, "mnist_index", "*=>[KNN 1 @embedding $blob AS dist]", options).Result()
	duration := time.Since(start).Milliseconds()
	if err != nil {
		return 0, 0, err
	}
	if result.Total == 0 {
		return 0, 0, fmt.Errorf("unexpected result format")
	}

	parts := strings.Split(result.Docs[0].ID, ":")

	// Get the last part (which should be the digit)
	lastPart := parts[len(parts)-1]

	// Convert the last part to an integer
	parsedInt, err := strconv.Atoi(lastPart)
	if err != nil {
		return 0, 0, err
	}

	return parsedInt, duration, nil
}

// convertFloat32ArrayToBlob - Convert vector to a byte array
func convertFloat32ArrayToBlob(vector []float32) ([]byte, error) {
	buf := new(bytes.Buffer)
	for _, v := range vector {
		err := binary.Write(buf, binary.LittleEndian, v)
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
