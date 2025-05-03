package main

import (
	"digit-recognition/internal"
	"os"
)

//go:generate oapi-codegen -package gen -generate chi-server,strict-server -o internal/api/gen/server.go openapi.yaml
//go:generate oapi-codegen -package gen -generate types -o internal/api/gen/types.go openapi.yaml
//go:generate oapi-codegen -package gen -generate spec -o internal/api/gen/spec.go openapi.yaml
func main() {
	workDir, _ := os.Getwd()
	internal.NewApplication(workDir)
}
