package logging

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/suite"
)

type LoggerSuite struct {
	suite.Suite
}

func TestLoggerSuite(t *testing.T) {
	suite.Run(t, new(LoggerSuite))
}

func (s *LoggerSuite) TestParseLevelUsesDefaultWhenEmpty() {
	level, err := ParseLevel("")

	s.Require().NoError(err)
	s.Equal(slog.LevelInfo, level)
}

func (s *LoggerSuite) TestParseLevelAcceptsCaseInsensitiveValue() {
	level, err := ParseLevel("debug")

	s.Require().NoError(err)
	s.Equal(slog.LevelDebug, level)
}

func (s *LoggerSuite) TestParseLevelRejectsUnknownValue() {
	_, err := ParseLevel("verbose")

	s.Require().Error(err)
	s.ErrorContains(err, `invalid LOG_LEVEL "verbose"`)
}
