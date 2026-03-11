package domain

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type RegisterMappingTestSuite struct {
	suite.Suite
}

func TestRegisterMappingTestSuite(t *testing.T) {
	suite.Run(t, new(RegisterMappingTestSuite))
}

func (s *RegisterMappingTestSuite) TestNewRegisterMapping() {
	tests := []struct {
		name               string
		registerType       RegisterType
		setpointAddress    uint16
		activePowerAddress uint16
		signedValues       bool
		wantErrText        string
	}{
		{
			name:               "empty register type rejected",
			registerType:       "",
			setpointAddress:    40100,
			activePowerAddress: 40101,
			signedValues:       true,
			wantErrText:        "register type must not be empty",
		},
		{
			name:               "zero setpoint address rejected",
			registerType:       HoldingRegister,
			setpointAddress:    0,
			activePowerAddress: 40101,
			signedValues:       true,
			wantErrText:        "setpoint address must not be zero",
		},
		{
			name:               "zero active power address rejected",
			registerType:       HoldingRegister,
			setpointAddress:    40100,
			activePowerAddress: 0,
			signedValues:       true,
			wantErrText:        "active power address must not be zero",
		},
		{
			name:               "equal addresses rejected",
			registerType:       HoldingRegister,
			setpointAddress:    40100,
			activePowerAddress: 40100,
			signedValues:       true,
			wantErrText:        "setpoint and active power addresses must be different",
		},
		{
			name:               "valid mapping accepted",
			registerType:       HoldingRegister,
			setpointAddress:    40100,
			activePowerAddress: 40101,
			signedValues:       true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			mapping, err := NewRegisterMapping(tt.registerType, tt.setpointAddress, tt.activePowerAddress, tt.signedValues)
			if tt.wantErrText == "" {
				s.Require().NoError(err)
				s.Equal(tt.registerType, mapping.RegisterType)
				s.Equal(tt.setpointAddress, mapping.SetpointAddress)
				s.Equal(tt.activePowerAddress, mapping.ActivePowerAddress)
				s.Equal(tt.signedValues, mapping.SignedValues)
				return
			}

			s.Require().Error(err)
			s.Equal(tt.wantErrText, err.Error())
		})
	}
}

func (s *RegisterMappingTestSuite) TestNewDefaultRegisterMapping() {
	mapping := NewDefaultRegisterMapping()

	s.Equal(HoldingRegister, mapping.RegisterType)
	s.Equal(uint16(40100), mapping.SetpointAddress)
	s.Equal(uint16(40101), mapping.ActivePowerAddress)
	s.True(mapping.SignedValues)
}
