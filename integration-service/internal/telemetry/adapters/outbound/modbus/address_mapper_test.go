package modbus

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"stellar/internal/telemetry/domain"
)

type AddressMapperTestSuite struct {
	suite.Suite
}

func TestAddressMapperTestSuite(t *testing.T) {
	suite.Run(t, new(AddressMapperTestSuite))
}

func (s *AddressMapperTestSuite) TestAddressMapperMap() {
	mapping, err := domain.NewRegisterMapping(domain.HoldingRegister, 40100, 40101, true)
	s.Require().NoError(err)

	mapper := NewAddressMapper()
	plan, err := mapper.Map(mapping)
	s.Require().NoError(err)

	s.Equal(uint16(99), plan.startAddress)
	s.Equal(uint16(2), plan.quantity)
	s.Equal(0, plan.setpointIndex)
	s.Equal(1, plan.activePowerIndex)
	s.True(plan.signedValues)
}
