package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

const DefaultLogLevel = "INFO"

func NewDefaultLogger() *slog.Logger {
	logger, err := NewLogger(DefaultLogLevel)
	if err == nil {
		return logger
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func NewLogger(level string) (*slog.Logger, error) {
	parsedLevel, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parsedLevel})), nil
}

func ParseLevel(level string) (slog.Level, error) {
	level = strings.TrimSpace(level)
	if level == "" {
		level = DefaultLogLevel
	}

	var parsedLevel slog.Level
	if err := parsedLevel.UnmarshalText([]byte(strings.ToUpper(level))); err != nil {
		return 0, fmt.Errorf("invalid LOG_LEVEL %q", level)
	}

	return parsedLevel, nil
}
