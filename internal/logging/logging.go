package logging

import (
	"github.com/sirupsen/logrus"
)

// NewLogger - Creates a new Logger
func NewLogger() *logrus.Logger {
	return logrus.New()
}
