package modbus

type Decoder struct{}

func NewDecoder() *Decoder {
	return &Decoder{}
}

func (d *Decoder) DecodeRegister(raw uint16, signedValues bool) float64 {
	if signedValues {
		return float64(int16(raw))
	}

	return float64(raw)
}
