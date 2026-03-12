package modbus

import (
	"fmt"

	"stellar/internal/telemetry/domain"
)

const holdingRegisterBaseAddress uint16 = 40001

type readPlan struct {
	startAddress     uint16
	quantity         uint16
	setpointIndex    int
	activePowerIndex int
	signedValues     bool
}

type AddressMapper struct{}

func NewAddressMapper() *AddressMapper {
	return &AddressMapper{}
}

func (m *AddressMapper) Map(mapping domain.RegisterMapping) (readPlan, error) {
	if mapping.RegisterType != domain.HoldingRegister {
		return readPlan{}, fmt.Errorf("unsupported register type %q", mapping.RegisterType)
	}

	if mapping.SetpointAddress < holdingRegisterBaseAddress {
		return readPlan{}, fmt.Errorf(
			"setpoint address %d is below holding register base %d",
			mapping.SetpointAddress,
			holdingRegisterBaseAddress,
		)
	}

	if mapping.ActivePowerAddress < holdingRegisterBaseAddress {
		return readPlan{}, fmt.Errorf(
			"active power address %d is below holding register base %d",
			mapping.ActivePowerAddress,
			holdingRegisterBaseAddress,
		)
	}

	firstAddress := min(mapping.SetpointAddress, mapping.ActivePowerAddress)
	lastAddress := max(mapping.SetpointAddress, mapping.ActivePowerAddress)

	return readPlan{
		startAddress:     holdingRegisterOffset(firstAddress),
		quantity:         (lastAddress - firstAddress) + 1,
		setpointIndex:    int(mapping.SetpointAddress - firstAddress),
		activePowerIndex: int(mapping.ActivePowerAddress - firstAddress),
		signedValues:     mapping.SignedValues,
	}, nil
}

func holdingRegisterOffset(address uint16) uint16 {
	return address - holdingRegisterBaseAddress
}
