package pulsar

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync/atomic"
	"time"
)

// Client is a Pulsar network client handler that communicates with a device using pulsar data transmission protocol.
type Client struct {
	// network connection
	conn Conn
	// device address
	address uint32
	// message id generator. Holds next message id value.
	ids uint32
}

// Discover searches for pulsar meters in a local network and initialises Client if device is found.
func Discover(conn Conn) (*Client, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection is required")
	}

	if err := conn.PrepareWrite(); err != nil {
		return nil, err
	}
	if _, err := conn.Write(discoveryMessage); err != nil {
		return nil, err
	}

	if err := conn.Flush(); err != nil {
		return nil, err
	}

	conn.LogRequest()

	if err := conn.PrepareRead(); err != nil {
		return nil, err
	}
	response := make([]byte, minFrameLen)
	if _, err := conn.Read(response); err != nil {
		return nil, err
	}

	conn.LogResponse()

	if err := checkCrc(response); err != nil {
		return nil, err
	}
	response = response[4:8]

	address := binary.BigEndian.Uint32(response)

	return NewClient(fmt.Sprintf("%08x", address), conn)
}

// NewClient creates a Client.
func NewClient(address string, conn Conn) (*Client, error) {
	i, err := strconv.ParseInt(address, 16, 32)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:    conn,
		address: uint32(i),
		ids:     math.MaxUint32,
	}, nil
}

// Resets the connection for a client.
func (c *Client) Reset(conn Conn) {
	c.conn = conn
}

// Address returns device's network address.
func (c *Client) Address() uint32 {
	return c.address
}

// Model retrieves model id from a device. (No documentation is found for device id decoding)
func (c *Client) Model() (uint16, error) {
	request := make([]byte, 9)
	binary.BigEndian.PutUint32(request, c.address)
	copy(request[4:], discoveryModel)
	if err := c.conn.PrepareWrite(); err != nil {
		return 0, err
	}

	if err := c.writeMessage(request); err != nil {
		return 0, err
	}

	for {
		if err := c.conn.PrepareRead(); err != nil {
			return 0, err
		}

		response := make([]byte, minFrameLen)
		if _, err := c.conn.Read(response); err != nil {
			return 0, err
		}

		if c.address != binary.BigEndian.Uint32(response) {
			continue
		}

		c.conn.LogResponse()
		if err := checkCrc(response); err != nil {
			return 0, err
		}
		return binary.BigEndian.Uint16(response[6:8]), nil
	}
}

// SysTime retrieves device's system time.
func (c *Client) SysTime() (time.Time, error) {
	data, err := c.command(fnReadSysTime, func() []byte {
		return nil
	})
	var st time.Time
	if err != nil {
		return st, err
	}
	var t sysTime
	if err = t.UnmarshalBinary(data); err != nil {
		return st, err
	}
	return time.Time(t), nil
}

// SetSysTime updates system time of the device.
func (c *Client) SetSysTime(t time.Time) error {
	data, err := c.command(fnWriteSysTime, func() []byte {
		tm := sysTime(t)
		rv, _ := tm.MarshalBinary()
		return rv
	})
	if err != nil {
		return err
	}
	if data[0] != writeOK {
		return ErrWriteFail
	}
	return nil
}

func validateChannels(chs ...uint) error {
	if len(chs) == 0 {
		return fmt.Errorf("at least a single channel is required")
	}
	for _, ch := range chs {
		if ch == 0 {
			return fmt.Errorf("channel must be non-zero")
		}
		if ch > maxChanNum {
			return fmt.Errorf("maximum %d channels is supported", maxChanNum)
		}
	}
	return nil
}

func makeMask(chs ...uint) uint32 {
	var mask uint32
	for _, ch := range chs {
		mask += 1 << (ch - 1)
	}
	return mask
}

// CurValues retrieves current values for channels. At least 1 channel number must be provided.
func (c *Client) CurValues(chs ...uint) ([]Channel, error) {
	if err := validateChannels(chs...); err != nil {
		return nil, err
	}
	mask := makeMask(chs...)
	data, err := c.command(fnReadValues, func() []byte {
		rv := make([]byte, 4)
		binary.LittleEndian.PutUint32(rv, mask)
		return rv
	})
	if err != nil {
		return nil, err
	}

	b := bytes.NewBuffer(data)
	if len(chs) > 1 {
		sort.Slice(chs, func(i, j int) bool { return chs[i] < chs[j] })
	}
	var val float64
	var retVal []Channel
	for _, ch := range chs {
		if err = binary.Read(b, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		retVal = append(retVal, Channel{
			Id:    ch,
			Value: val,
		})
	}
	return retVal, nil
}

// SetCurValue updates current value for a channel.
func (c *Client) SetCurValue(ch uint, val float64) error {
	if ch == 0 {
		return fmt.Errorf("channel must be non-zero")
	}
	wMask := uint32(1 << (ch - 1))
	data, err := c.command(fnWriteValue, func() []byte {
		var b bytes.Buffer
		_ = binary.Write(&b, binary.LittleEndian, wMask)
		_ = binary.Write(&b, binary.LittleEndian, val)
		return b.Bytes()
	})
	if err != nil {
		return err
	}
	rMask := binary.LittleEndian.Uint32(data)
	if rMask != wMask {
		return fmt.Errorf("recorded wrong channel mask: %b", rMask)
	}
	return nil
}

// PulseWeight retrieves pulse weights for channels. At least 1 channel number must be provided.
func (c *Client) PulseWeight(chs ...uint) ([]PulseWeight, error) {
	if err := validateChannels(chs...); err != nil {
		return nil, err
	}

	data, err := c.command(fnReadPulseWeight, func() []byte {
		mask := makeMask(chs...)
		rv := make([]byte, 4)
		binary.LittleEndian.PutUint32(rv, mask)
		return rv
	})
	if err != nil {
		return nil, err
	}

	b := bytes.NewBuffer(data)
	if len(chs) > 1 {
		sort.Slice(chs, func(i, j int) bool { return chs[i] < chs[j] })
	}
	var val float32
	var p []PulseWeight
	for _, ch := range chs {
		if err = binary.Read(b, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		p = append(p, PulseWeight{
			Id:    ch,
			Value: val,
		})
	}
	return p, nil
}

// SetPulseWeight updates pulse weight for a channel.
func (c *Client) SetPulseWeight(ch uint, val float32) error {
	if ch == 0 {
		return fmt.Errorf("channel must be non-zero")
	}
	wMask := uint32(1 << (ch - 1))
	data, err := c.command(fnWritePulseWeight, func() []byte {
		var b bytes.Buffer
		_ = binary.Write(&b, binary.LittleEndian, wMask)
		_ = binary.Write(&b, binary.LittleEndian, val)
		return b.Bytes()
	})
	if err != nil {
		return err
	}
	rMask := binary.LittleEndian.Uint32(data)
	if rMask != wMask {
		return fmt.Errorf("recorded wrong channel mask: %b", rMask)
	}
	return nil
}

// Common function that retrieves configuration parameter's value.
func (c *Client) param(name configParam) ([]byte, error) {
	return c.command(fnReadSettings, func() []byte {
		rv := make([]byte, 2)
		binary.LittleEndian.PutUint16(rv, uint16(name))
		return rv
	})
}

// Common function to update configuration parameter's value.
func (c *Client) setParam(name configParam, value []byte) error {
	data, err := c.command(fnWriteSettings, func() []byte {
		rv := make([]byte, 10)
		binary.LittleEndian.PutUint16(rv, uint16(name))
		copy(rv[2:], value)
		return rv
	})

	if err != nil {
		return err
	}
	res := binary.LittleEndian.Uint16(data)
	if res == resultWR {
		return nil
	}
	return fmt.Errorf("param write fail")
}

// DayLightSaving queries device if daylight saving enabled.
// Returns true if enabled.
func (c *Client) DayLightSaving() (bool, error) {
	data, err := c.param(dayTimeSave)
	rv := binary.LittleEndian.Uint64(data) & 0xFFFF
	var value bool
	if err != nil && rv != 0 {
		value = true
	}
	return value, err
}

// SetDayLightSaving sets newValue as daylight saving param.
// true means enabled.
func (c *Client) SetDayLightSaving(newValue bool) error {
	var nv uint64
	if newValue {
		nv = 1
	}
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, nv)
	return c.setParam(dayTimeSave, b)
}

// PulseLength retrieves pulse length param value.
func (c *Client) PulseLength() (float32, error) {
	rv, err := c.param(pulseLength)
	if err != nil {
		return 0., err
	}
	b := bytes.NewReader(rv)
	var value float32
	err = binary.Read(b, binary.LittleEndian, &value)
	return value, err
}

// SetPulseLength updates pulse length param value.
func (c *Client) SetPulseLength(newValue float32) error {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, float64(newValue))
	return c.setParam(pulseLength, b.Bytes())
}

// PauseLength retrieves pause length param value.
func (c *Client) PauseLength() (float32, error) {
	rv, err := c.param(pauseLength)
	if err != nil {
		return 0., err
	}
	b := bytes.NewReader(rv)
	var value float32
	err = binary.Read(b, binary.LittleEndian, &value)
	return value, err
}

// SetPauseLength updates pause length param value.
func (c *Client) SetPauseLength(newValue float32) error {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, float64(newValue))
	return c.setParam(pauseLength, b.Bytes())
}

// FirmwareVersion retrieves current firmware version of a device.
func (c *Client) FirmwareVersion() (uint16, error) {
	rv, err := c.param(firmwareVer)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(rv), nil
}

// DiagnosticsFlags retrieves self-check results.
// 0x04 means EEPROM write error, 0x08 - negative current value in a channel.
func (c *Client) DiagnosticsFlags() (uint8, error) {
	rv, err := c.param(health)
	if err != nil {
		return 0, err
	}
	return rv[0], nil
}

// SerialSpeed returns serial line speed configuration.
func (c *Client) SerialSpeed() (uint32, error) {
	rv, err := c.param(speed)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(rv), nil
}

// SetSerialSpeed updates device serial line communication speed.
// Possible values are: 1200..19200
func (c *Client) SetSerialSpeed(newValue uint32) error {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, uint64(newValue))
	return c.setParam(speed, b.Bytes())
}

// SerialConfig retrieves encoded serial line communication parameters.
func (c *Client) SerialConfig() (SerialConfig, error) {
	var value SerialConfig
	rv, err := c.param(serial)
	if err != nil {
		return value, err
	}
	value = SerialConfig(rv[0])
	return value, nil
}

// SetSerialConfig updates serial line communication parameters.
func (c *Client) SetSerialConfig(newValue SerialConfig) error {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, uint64(newValue))
	return c.setParam(serial, b.Bytes())
}

// common function for archive retrieval.
func (c *Client) valuesLog(arch ArchType, ch uint, from, to sysTime) (*ChannelLog, error) {
	if ch == 0 {
		return nil, fmt.Errorf("channel must be non-zero")
	}
	mask := uint32(1 << (ch - 1))
	tmStart, err := from.MarshalBinary()
	if err != nil {
		return nil, err
	}

	tmEnd, err := to.MarshalBinary()
	if err != nil {
		return nil, err
	}
	data, err := c.command(fnReadArchive, func() []byte {
		var b bytes.Buffer
		_ = binary.Write(&b, binary.LittleEndian, mask)
		_ = binary.Write(&b, binary.LittleEndian, uint16(arch))
		_, _ = b.Write(tmStart)
		_, _ = b.Write(tmEnd)
		return b.Bytes()
	})

	if err != nil {
		return nil, err
	}
	chl := &ChannelLog{}
	err = chl.UnmarshalBinary(data)
	return chl, err
}

// HourlyLog retrieves hourly archive from device.
func (c *Client) HourlyLog(ch uint, from, to time.Time) (*ChannelLog, error) {
	start := sysTime(time.Date(from.Year(), from.Month(), from.Day(), from.Hour(), 0, 0, 0, from.Location()))
	end := sysTime(time.Date(to.Year(), to.Month(), to.Day(), to.Hour(), 0, 0, 0, to.Location()))
	l, err := c.valuesLog(Hourly, ch, start, end)
	if err != nil {
		return nil, err
	}
	l.Type = Hourly
	return l, nil
}

// DailyLog retrieves daily archive from device.
func (c *Client) DailyLog(ch uint, from, to time.Time) (*ChannelLog, error) {
	start := sysTime(time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location()))
	end := sysTime(time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location()))
	l, err := c.valuesLog(Daily, ch, start, end)
	if err != nil {
		return nil, err
	}
	l.Type = Daily
	return l, nil
}

// MonthlyLog retrieves monthly archive from device.
func (c *Client) MonthlyLog(ch uint, from, to time.Time) (*ChannelLog, error) {
	start := sysTime(time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, from.Location()))
	end := sysTime(time.Date(to.Year(), to.Month()+1, 1, 0, 0, 0, 0, to.Location()))
	l, err := c.valuesLog(Monthly, ch, start, end)
	if err != nil {
		return nil, err
	}
	l.Type = Monthly
	return l, nil
}

// LineTest starts sensor test procedure.
// This command suppress counting up to 200ms which can affect counting results.
// See documentation for testing stand and bitmask result meaning.
func (c *Client) LineTest(chs ...uint) (uint32, error) {
	if err := validateChannels(chs...); err != nil {
		return 0, err
	}
	wMask := makeMask(chs...)
	data, err := c.command(fnLineTest, func() []byte {
		rv := make([]byte, 4)
		binary.LittleEndian.PutUint32(rv, wMask)
		return rv
	})
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(data), nil
}

// InputTest retrieves sensor state for channels.
// Returns a bitmask where 0s represent shorted sensors for a channel.
func (c *Client) InputTest(chs ...uint) (uint32, error) {
	if err := validateChannels(chs...); err != nil {
		return 0, err
	}
	wMask := makeMask(chs...)
	data, err := c.command(fnInputTest, func() []byte {
		rv := make([]byte, 4)
		binary.LittleEndian.PutUint32(rv, wMask)
		return rv
	})
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(data), nil
}

// id generator. Just adds a 1 to the next id.
func (c *Client) nextId() uint16 {
	atomic.AddUint32(&c.ids, 1)
	return uint16(c.ids%math.MaxUint16) + 1
}

// command encodes frame, sends to device, receives, decodes and validates responses.
// Request and response message pattern  [address, function, length, payload, id, crc]
func (c *Client) command(cmd byte, payload func() []byte) ([]byte, error) {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, c.address)
	_ = b.WriteByte(cmd)
	_ = b.WriteByte(0)
	ln, _ := b.Write(payload())
	id := c.nextId()
	_ = binary.Write(&b, binary.BigEndian, id)
	req := b.Bytes()
	req[5] = byte(ln + minFrameLen)

	if err := c.writeMessage(req); err != nil {
		return nil, err
	}

	data, err := c.readMessage()
	if err != nil {
		return nil, err
	}

	if data[4] != cmd {
		err = fmt.Errorf("worng function in response")
		return nil, err
	}
	ln = int(data[5]) - minFrameLen
	return data[6 : 6+ln], nil
}

// prepares message and sends it to a device.
func (c *Client) writeMessage(request []byte) error {
	var check crc
	check.reset()
	check.update(request)
	if err := c.conn.PrepareWrite(); err != nil {
		return err
	}
	if _, err := c.conn.Write(request); err != nil {
		return err
	}

	if err := binary.Write(c.conn, binary.LittleEndian, check); err != nil {
		return err
	}

	err := c.conn.Flush()
	if err == nil {
		c.conn.LogRequest()
	}
	return nil
}

// reads and validates incoming message.
func (c *Client) readMessage() ([]byte, error) {
	rv, err := func(c *Client) ([]byte, error) {
		for {
			if err := c.conn.PrepareRead(); err != nil {
				return nil, err
			}
			var cl = 6
			response := make([]byte, cl)
			if _, err := c.conn.Read(response); err != nil {
				return nil, err
			}

			n := int(response[cl-1]) - cl
			if n < 5 {
				return nil, ErrInvalidFrame
			}
			response = append(response[:cl], make([]byte, n)...)

			if _, err := c.conn.Read(response[cl:]); err != nil {
				return nil, err
			}

			if c.address != binary.BigEndian.Uint32(response) {
				continue
			}

			if err := checkCrc(response); err != nil {
				return nil, err
			}
			if response[4] == fnError {
				return nil, &ProtocolError{ErrorCode(response[6])}
			}
			return response, nil
		}
	}(c)

	if err == nil {
		c.conn.LogResponse()
	}
	return rv, err
}

// crc16 check.
func checkCrc(response []byte) error {
	ln := len(response) - 2
	if ln <= 0 {
		return ErrTooShort
	}
	var check crc
	check.reset()
	check.update(response[:ln])
	tst := crc(binary.LittleEndian.Uint16(response[ln:]))
	if tst != check {
		return ErrCRC
	}
	return nil
}

type crc uint16

func (c *crc) reset() {
	*c = 0xffff
}

func (c *crc) update(data []byte) {
	for _, b := range data {
		*c ^= crc(b)
		for i := 0; i < 8; i++ {
			if *c&1 > 0 {
				*c = (*c >> 1) ^ 0xA001
			} else {
				*c >>= 1
			}
		}
	}
}
