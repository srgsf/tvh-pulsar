package pulsar

import (
	"bytes"
	"encoding/binary"
	"errors"
	"time"
)

// minimal message length that can be parsed.
const minFrameLen = 10

// successful result of system time writing operation.
const writeOK byte = 0x01

// successful result of configuration param writing operation.
const resultWR uint16 = 0x00

// magic message for device address discovery. (Details are not provided in protocol description).
var discoveryMessage = []byte{0xF0, 0x0F, 0x0F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0xA5, 0x44}

// magic payload for device model discovery. (Details are not provided in protocol description).
var discoveryModel = []byte{0x03, 0x02, 0x46, 0x00, 0x01}

// Function codes that are used in communication.
const (
	fnError            byte = 0x00
	fnReadValues       byte = 0x01
	fnWriteValue       byte = 0x03
	fnReadSysTime      byte = 0x04
	fnWriteSysTime     byte = 0x05
	fnReadArchive      byte = 0x06
	fnReadPulseWeight  byte = 0x07
	fnWritePulseWeight byte = 0x08
	fnLineTest         byte = 0x09
	fnReadSettings     byte = 0x0A
	fnWriteSettings    byte = 0x0B
	fnInputTest        byte = 0x19
)

// SerialConfig is a bitset that encodes different serial line configuration params.
// Name contains number of bits, parity and stop bits number, e.g. Serial8N1 stands for 8 bits, parity: None, Stop bits: 1.
type SerialConfig byte

const (
	Serial8N1 SerialConfig = 0
	Serial8N2 SerialConfig = 8
	Serial8O1 SerialConfig = 128
	Serial8O2 SerialConfig = 136
	Serial8E1 SerialConfig = 192
	Serial8E2 SerialConfig = 200
)

// ErrorCode is a code returned by device on invalid request.
type ErrorCode uint8

const (
	UnknownError ErrorCode = iota
	IllegalFunction
	InvalidBitMask
	InvalidLength
	MissingParam
	IllegalAccess
	InvalidParamValue
	MissingArchive
	TooLongPeriod
)

// ProtocolError wraps ErrorCode.
type ProtocolError struct {
	code ErrorCode
}

// Code returns ErrorCode returned by device.
func (e *ProtocolError) Code() ErrorCode {
	return e.code
}

func (e *ProtocolError) Error() string {
	switch e.code {
	case IllegalFunction:
		return "illegal function"
	case InvalidBitMask:
		return "invalid bitmask param value"
	case InvalidLength:
		return "invalid request length"
	case MissingParam:
		return "missing required param"
	case IllegalAccess:
		return "access denied"
	case InvalidParamValue:
		return "invalid param value"
	case MissingArchive:
		return "archive not found"
	case TooLongPeriod:
		return "too long period"
	default:
		return "unknown error"
	}
}

var CRC = errors.New("crc fail")
var ToShort = errors.New("value too short")
var WriteFail = errors.New("write failed")

// configuration param encoded name.
type configParam uint16

const (
	dayTimeSave configParam = 0x0001
	pulseLength configParam = 0x0003
	pauseLength configParam = 0x0004
	firmwareVer configParam = 0x0005
	health      configParam = 0x0006
	speed       configParam = 0x0008
	serial      configParam = 0x0009
)

// time wrapper for protocol specific binary Marshalling/Unmarshalling
// Binary format is [year starting from 2000, month (1 - January), date, hour, minute, second]
type sysTime time.Time

func (t *sysTime) UnmarshalBinary(data []byte) error {
	if len(data) < 6 {
		return ToShort
	}
	*t = sysTime(time.Date(2000+int(data[0]),
		time.Month(data[1]),
		int(data[2]),
		int(data[3]),
		int(data[4]),
		int(data[5]),
		0,
		time.Local,
	))
	return nil
}

func (t sysTime) MarshalBinary() ([]byte, error) {
	tm := time.Time(t)
	return []byte{
		byte(tm.Year() - 2000),
		byte(tm.Month()),
		byte(tm.Day()),
		byte(tm.Hour()),
		byte(tm.Minute()),
		byte(tm.Second()),
	}, nil
}

// Channel is a current value response holder.
type Channel struct {
	// Number of channel.
	Id uint
	// Current value.
	Value float64
}

// PulseWeight is a current pulse weight value response holder.
type PulseWeight struct {
	// Number fo channel.
	Id uint
	// Current value of pulse weight.
	Value float32
}

// ArchType is an archive (log) type.
type ArchType byte

const (
	Hourly ArchType = iota + 1
	Daily
	Monthly
)

// ChannelLog is a response holder for archive values response.
type ChannelLog struct {
	// Number o channel.
	Id uint
	// Type of archive.
	Type ArchType
	// 1'st value time.
	Start time.Time
	// Archive values.
	Values []float32
}

func (l *ChannelLog) UnmarshalBinary(data []byte) (err error) {
	if len(data) < minFrameLen {
		return ToShort
	}
	b := bytes.NewBuffer(data)
	var id uint32
	if err = binary.Read(b, binary.LittleEndian, &id); err != nil {
		return
	}
	l.Id = uint(id)

	var t sysTime
	if err = t.UnmarshalBinary(b.Next(6)); err != nil {
		return
	}
	l.Start = time.Time(t)
	var cur float32
	for b.Len() > 0 {
		if err = binary.Read(b, binary.LittleEndian, &cur); err != nil {
			return err
		}
		l.Values = append(l.Values, cur)
	}
	return
}
