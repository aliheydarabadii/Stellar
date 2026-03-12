// Package modbus implements the Modbus TCP telemetry source adapter.
package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	telemetry "stellar/internal/telemetry"
)

const (
	defaultHost                 = "127.0.0.1"
	defaultPort          uint16 = 5020
	defaultUnitID        uint8  = 1
	readHoldingRegisters        = 0x03

	defaultDialTimeout = 5 * time.Second
	defaultIOTimeout   = 5 * time.Second
)

type Config struct {
	Host            string
	Port            uint16
	UnitID          uint8
	RegisterMapping telemetry.RegisterMapping
}

type Source struct {
	config      Config
	mapper      *AddressMapper
	decoder     *Decoder
	dialContext func(ctx context.Context, network, address string) (net.Conn, error)

	mu            sync.Mutex
	transactionID uint16
}

func NewSource(config Config, mapper *AddressMapper, decoder *Decoder) (*Source, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	if mapper == nil {
		mapper = NewAddressMapper()
	}

	if decoder == nil {
		decoder = NewDecoder()
	}

	dialer := &net.Dialer{Timeout: defaultDialTimeout}

	return &Source{
		config:  config,
		mapper:  mapper,
		decoder: decoder,
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, address)
		},
	}, nil
}

func DefaultConfig() Config {
	return Config{
		Host:            defaultHost,
		Port:            defaultPort,
		UnitID:          defaultUnitID,
		RegisterMapping: telemetry.NewDefaultRegisterMapping(),
	}
}

func validateConfig(config Config) error {
	if config.Host == "" {
		return fmt.Errorf("modbus config: %w", ErrEmptyHost)
	}
	if config.Port == 0 {
		return fmt.Errorf("modbus config: %w", ErrZeroPort)
	}
	if config.UnitID == 0 {
		return fmt.Errorf("modbus config: %w", ErrZeroUnitID)
	}
	if config.RegisterMapping.RegisterType == "" {
		return fmt.Errorf("modbus config: %w", ErrEmptyRegisterType)
	}
	if config.RegisterMapping.SetpointAddress == 0 {
		return fmt.Errorf("modbus config: %w", ErrZeroSetpointAddress)
	}
	if config.RegisterMapping.ActivePowerAddress == 0 {
		return fmt.Errorf("modbus config: %w", ErrZeroActivePowerAddress)
	}
	if config.RegisterMapping.RegisterType != telemetry.HoldingRegister {
		return fmt.Errorf("modbus config: %w: %q", ErrUnsupportedRegisterType, config.RegisterMapping.RegisterType)
	}
	return nil
}

func (s *Source) Read(ctx context.Context) (telemetry.TelemetryReading, error) {
	plan, err := s.mapper.Map(s.config.RegisterMapping)
	if err != nil {
		return telemetry.TelemetryReading{}, err
	}

	conn, err := s.dialContext(ctx, "tcp", net.JoinHostPort(s.config.Host, strconv.Itoa(int(s.config.Port))))
	if err != nil {
		return telemetry.TelemetryReading{}, fmt.Errorf("dial modbus source: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(deadlineFromContext(ctx, defaultIOTimeout)); err != nil {
		return telemetry.TelemetryReading{}, fmt.Errorf("set modbus deadline: %w", err)
	}

	registers, err := s.readHoldingRegisters(conn, plan)
	if err != nil {
		return telemetry.TelemetryReading{}, err
	}

	return telemetry.TelemetryReading{
		Setpoint:    s.decoder.DecodeRegister(registers[plan.setpointIndex], plan.signedValues),
		ActivePower: s.decoder.DecodeRegister(registers[plan.activePowerIndex], plan.signedValues),
	}, nil
}

func (s *Source) readHoldingRegisters(conn net.Conn, plan readPlan) ([]uint16, error) {
	transactionID := s.nextTransactionID()

	request := make([]byte, 12)
	binary.BigEndian.PutUint16(request[0:2], transactionID)
	binary.BigEndian.PutUint16(request[2:4], 0)
	binary.BigEndian.PutUint16(request[4:6], 6)
	request[6] = s.config.UnitID
	request[7] = readHoldingRegisters
	binary.BigEndian.PutUint16(request[8:10], plan.startAddress)
	binary.BigEndian.PutUint16(request[10:12], plan.quantity)

	if err := writeAll(conn, request); err != nil {
		return nil, fmt.Errorf("write modbus request: %w", err)
	}

	header := make([]byte, 7)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, fmt.Errorf("read modbus response header: %w", err)
	}

	responseTransactionID := binary.BigEndian.Uint16(header[0:2])
	if responseTransactionID != transactionID {
		return nil, fmt.Errorf("unexpected transaction id %d", responseTransactionID)
	}

	if protocolID := binary.BigEndian.Uint16(header[2:4]); protocolID != 0 {
		return nil, fmt.Errorf("unexpected protocol id %d", protocolID)
	}

	length := binary.BigEndian.Uint16(header[4:6])
	if length < 3 {
		return nil, fmt.Errorf("invalid modbus response length %d", length)
	}

	body := make([]byte, int(length)-1)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, fmt.Errorf("read modbus response body: %w", err)
	}

	functionCode := body[0]
	if functionCode&0x80 != 0 {
		if len(body) < 2 {
			return nil, fmt.Errorf("invalid modbus exception response")
		}
		return nil, fmt.Errorf("modbus exception code %d", body[1])
	}

	if functionCode != readHoldingRegisters {
		return nil, fmt.Errorf("unexpected function code %d", functionCode)
	}

	if len(body) < 2 {
		return nil, fmt.Errorf("invalid modbus response body")
	}

	byteCount := int(body[1])
	if byteCount != int(plan.quantity)*2 {
		return nil, fmt.Errorf("unexpected byte count %d", byteCount)
	}

	registerBytes := body[2:]
	if len(registerBytes) != byteCount {
		return nil, fmt.Errorf("incomplete register payload")
	}

	registers := make([]uint16, int(plan.quantity))
	for i := range registers {
		offset := i * 2
		registers[i] = binary.BigEndian.Uint16(registerBytes[offset : offset+2])
	}

	return registers, nil
}

func (s *Source) nextTransactionID() uint16 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.transactionID++
	if s.transactionID == 0 {
		s.transactionID = 1
	}

	return s.transactionID
}

func deadlineFromContext(ctx context.Context, fallback time.Duration) time.Time {
	if deadline, ok := ctx.Deadline(); ok {
		return deadline
	}
	return time.Now().Add(fallback)
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

var _ telemetry.TelemetrySource = (*Source)(nil)
