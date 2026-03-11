package modbus

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type DecoderTestSuite struct {
	suite.Suite
	decoder *Decoder
}

func TestDecoderTestSuite(t *testing.T) {
	suite.Run(t, new(DecoderTestSuite))
}

func (s *DecoderTestSuite) SetupTest() {
	s.decoder = NewDecoder()
}

func (s *DecoderTestSuite) TestDecodeRegister() {
	tests := []struct {
		name         string
		raw          uint16
		signedValues bool
		want         float64
	}{
		{
			name:         "signed register is decoded as negative int16",
			raw:          0xFFFF,
			signedValues: true,
			want:         -1,
		},
		{
			name:         "unsigned register keeps raw value",
			raw:          0xFFFF,
			signedValues: false,
			want:         65535,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.want, s.decoder.DecodeRegister(tt.raw, tt.signedValues))
		})
	}
}
