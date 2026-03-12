package logging

import (
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	level, err := ParseLevel("")
	if err != nil {
		t.Fatalf("ParseLevel() error = %v", err)
	}

	if level != slog.LevelInfo {
		t.Fatalf("expected INFO level, got %v", level)
	}
}

func TestParseLevelSupportsConfiguredValue(t *testing.T) {
	level, err := ParseLevel("debug")
	if err != nil {
		t.Fatalf("ParseLevel() error = %v", err)
	}

	if level != slog.LevelDebug {
		t.Fatalf("expected DEBUG level, got %v", level)
	}
}

func TestParseLevelRejectsInvalidValue(t *testing.T) {
	_, err := ParseLevel("loud")
	if err == nil {
		t.Fatal("expected invalid log level error")
	}
}
