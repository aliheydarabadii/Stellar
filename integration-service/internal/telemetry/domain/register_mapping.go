package domain

import "fmt"

type RegisterType string

const (
	HoldingRegister RegisterType = "holding"
)

type RegisterMapping struct {
	RegisterType       RegisterType
	SetpointAddress    uint16
	ActivePowerAddress uint16
	SignedValues       bool
}

func NewRegisterMapping(registerType RegisterType, setpointAddress, activePowerAddress uint16, signedValues bool) (RegisterMapping, error) {
	switch {
	case registerType == "":
		return RegisterMapping{}, fmt.Errorf("register type must not be empty")
	case setpointAddress == 0:
		return RegisterMapping{}, fmt.Errorf("setpoint address must not be zero")
	case activePowerAddress == 0:
		return RegisterMapping{}, fmt.Errorf("active power address must not be zero")
	case setpointAddress == activePowerAddress:
		return RegisterMapping{}, fmt.Errorf("setpoint and active power addresses must be different")
	}

	return RegisterMapping{
		RegisterType:       registerType,
		SetpointAddress:    setpointAddress,
		ActivePowerAddress: activePowerAddress,
		SignedValues:       signedValues,
	}, nil
}

func NewDefaultRegisterMapping() RegisterMapping {
	mapping, err := NewRegisterMapping(HoldingRegister, 40100, 40101, true)
	if err != nil {
		panic("domain: default register mapping is invalid")
	}

	return mapping
}
