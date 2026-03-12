package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

const DefaultLogLevel = "INFO"

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
		return 0, fmt.Errorf("invalid log level %q", level)
	}

	return parsedLevel, nil
}
