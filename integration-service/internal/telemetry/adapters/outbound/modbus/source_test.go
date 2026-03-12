package modbus

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"stellar/internal/telemetry/domain"
)

type SourceTestSuite struct {
	suite.Suite
	validMapping domain.RegisterMapping
}

func TestSourceTestSuite(t *testing.T) {
	suite.Run(t, new(SourceTestSuite))
}

func (s *SourceTestSuite) SetupTest() {
	validMapping, err := domain.NewRegisterMapping(domain.HoldingRegister, 40100, 40101, true)
	s.Require().NoError(err)
	s.validMapping = validMapping
}

func (s *SourceTestSuite) TestNewSource() {
	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name: "empty host rejected",
			config: Config{
				Port:            502,
				UnitID:          1,
				RegisterMapping: s.validMapping,
			},
			wantErr: ErrEmptyHost,
		},
		{
			name: "zero port rejected",
			config: Config{
				Host:            "127.0.0.1",
				UnitID:          1,
				RegisterMapping: s.validMapping,
			},
			wantErr: ErrZeroPort,
		},
		{
			name: "zero unit id rejected",
			config: Config{
				Host:            "127.0.0.1",
				Port:            502,
				RegisterMapping: s.validMapping,
			},
			wantErr: ErrZeroUnitID,
		},
		{
			name: "empty register type rejected",
			config: Config{
				Host:   "127.0.0.1",
				Port:   502,
				UnitID: 1,
				RegisterMapping: domain.RegisterMapping{
					SetpointAddress:    40100,
					ActivePowerAddress: 40101,
					SignedValues:       true,
				},
			},
			wantErr: ErrEmptyRegisterType,
		},
		{
			name: "zero setpoint address rejected",
			config: Config{
				Host:   "127.0.0.1",
				Port:   502,
				UnitID: 1,
				RegisterMapping: domain.RegisterMapping{
					RegisterType:       domain.HoldingRegister,
					ActivePowerAddress: 40101,
					SignedValues:       true,
				},
			},
			wantErr: ErrZeroSetpointAddress,
		},
		{
			name: "zero active power address rejected",
			config: Config{
				Host:   "127.0.0.1",
				Port:   502,
				UnitID: 1,
				RegisterMapping: domain.RegisterMapping{
					RegisterType:    domain.HoldingRegister,
					SetpointAddress: 40100,
					SignedValues:    true,
				},
			},
			wantErr: ErrZeroActivePowerAddress,
		},
		{
			name: "unsupported register type rejected",
			config: Config{
				Host:   "127.0.0.1",
				Port:   502,
				UnitID: 1,
				RegisterMapping: domain.RegisterMapping{
					RegisterType:       domain.RegisterType("input"),
					SetpointAddress:    40100,
					ActivePowerAddress: 40101,
					SignedValues:       true,
				},
			},
			wantErr: ErrUnsupportedRegisterType,
		},
		{
			name: "valid config accepted",
			config: Config{
				Host:            "127.0.0.1",
				Port:            502,
				UnitID:          1,
				RegisterMapping: s.validMapping,
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			source, err := NewSource(tt.config, NewAddressMapper(), NewDecoder())
			if tt.wantErr == nil {
				s.Require().NoError(err)
				s.NotNil(source)
				return
			}

			s.Require().Error(err)
			s.ErrorIs(err, tt.wantErr)
		})
	}
}

func (s *SourceTestSuite) TestSourceRead() {
	source, err := NewSource(DefaultConfig(), NewAddressMapper(), NewDecoder())
	s.Require().NoError(err)

	plan, err := source.mapper.Map(source.config.RegisterMapping)
	s.Require().NoError(err)

	registers := make([]uint16, int(plan.quantity))
	registers[plan.setpointIndex] = 100
	registers[plan.activePowerIndex] = 55

	conn := newFakeConn(buildReadHoldingResponse(1, source.config.UnitID, registers))

	source.dialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		s.Equal("tcp", network)
		s.Equal(net.JoinHostPort(source.config.Host, "5020"), address)
		return conn, nil
	}

	reading, err := source.Read(context.Background())
	s.Require().NoError(err)

	s.Equal(float64(100), reading.Setpoint)
	s.Equal(float64(55), reading.ActivePower)
	s.True(conn.closed)
	s.False(conn.deadline.IsZero())
	s.Len(conn.written, 12)
	s.Equal(byte(readHoldingRegisters), conn.written[7])
	s.Equal(plan.quantity, binary.BigEndian.Uint16(conn.written[10:12]))
}

func (s *SourceTestSuite) TestSourceReadUnexpectedTransactionID() {
	source, err := NewSource(DefaultConfig(), NewAddressMapper(), NewDecoder())
	s.Require().NoError(err)

	plan, err := source.mapper.Map(source.config.RegisterMapping)
	s.Require().NoError(err)

	registers := make([]uint16, int(plan.quantity))
	conn := newFakeConn(buildReadHoldingResponse(99, source.config.UnitID, registers))

	source.dialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return conn, nil
	}

	_, err = source.Read(context.Background())
	s.Require().Error(err)
	s.Contains(err.Error(), "unexpected transaction id")
}

func (s *SourceTestSuite) TestDeadlineFromContextUsesContextDeadline() {
	expected := time.Now().Add(2 * time.Second).Round(0)
	ctx, cancel := context.WithDeadline(context.Background(), expected)
	defer cancel()

	got := deadlineFromContext(ctx, defaultIOTimeout)
	s.True(got.Equal(expected))
}

func (s *SourceTestSuite) TestDeadlineFromContextUsesFallback() {
	before := time.Now()
	got := deadlineFromContext(context.Background(), defaultIOTimeout)
	after := time.Now()

	min := before.Add(defaultIOTimeout)
	max := after.Add(defaultIOTimeout + 100*time.Millisecond)

	s.False(got.Before(min))
	s.False(got.After(max))
}

func (s *SourceTestSuite) TestWriteAllWritesEntirePayload() {
	conn := &fakeConn{writeChunkSize: 3}
	payload := []byte{1, 2, 3, 4, 5, 6, 7}

	s.Require().NoError(writeAll(conn, payload))
	s.True(bytes.Equal(conn.written, payload))
}

func (s *SourceTestSuite) TestNextTransactionIDWrapsToOne() {
	source, err := NewSource(DefaultConfig(), NewAddressMapper(), NewDecoder())
	s.Require().NoError(err)

	source.transactionID = ^uint16(0)

	got := source.nextTransactionID()
	s.Equal(uint16(1), got)
}

func buildReadHoldingResponse(transactionID uint16, unitID uint8, registers []uint16) []byte {
	byteCount := len(registers) * 2
	length := uint16(3 + byteCount)

	response := make([]byte, 7+2+byteCount)
	binary.BigEndian.PutUint16(response[0:2], transactionID)
	binary.BigEndian.PutUint16(response[2:4], 0)
	binary.BigEndian.PutUint16(response[4:6], length)
	response[6] = unitID
	response[7] = readHoldingRegisters
	response[8] = byte(byteCount)

	offset := 9
	for _, register := range registers {
		binary.BigEndian.PutUint16(response[offset:offset+2], register)
		offset += 2
	}

	return response
}

type fakeConn struct {
	reader         *bytes.Reader
	written        []byte
	writeChunkSize int
	deadline       time.Time
	closed         bool
}

func newFakeConn(response []byte) *fakeConn {
	return &fakeConn{
		reader: bytes.NewReader(response),
	}
}

func (c *fakeConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func (c *fakeConn) Write(p []byte) (int, error) {
	n := len(p)
	if c.writeChunkSize > 0 && n > c.writeChunkSize {
		n = c.writeChunkSize
	}

	c.written = append(c.written, p[:n]...)
	return n, nil
}

func (c *fakeConn) Close() error {
	c.closed = true
	return nil
}

func (c *fakeConn) LocalAddr() net.Addr                { return fakeAddr("local") }
func (c *fakeConn) RemoteAddr() net.Addr               { return fakeAddr("remote") }
func (c *fakeConn) SetDeadline(t time.Time) error      { c.deadline = t; return nil }
func (c *fakeConn) SetReadDeadline(t time.Time) error  { c.deadline = t; return nil }
func (c *fakeConn) SetWriteDeadline(t time.Time) error { c.deadline = t; return nil }

type fakeAddr string

func (a fakeAddr) Network() string { return string(a) }
func (a fakeAddr) String() string  { return string(a) }
