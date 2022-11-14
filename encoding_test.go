package pulsar

import (
	"encoding/hex"
	"testing"
	"time"
)

var tm = time.Date(2022, 2, 3, 4, 5, 6, 0, time.Local)
var bt = []byte{22, 2, 3, 4, 5, 6}
var chanLogPayload = []byte{
	0x01, 0x00, 0x00, 0x00, 0x16, 0x09, 0x05, 0x0D, 0x00, 0x00, 0x06, 0x89, 0x2C, 0x44, 0x06, 0x89,
	0x2C, 0x44, 0x06, 0x89, 0x2C, 0x44, 0x06, 0x89, 0x2C, 0x44, 0xAA, 0x89, 0x2C, 0x44, 0xAA, 0x89,
	0x2C, 0x44, 0xF2, 0x8A, 0x2C, 0x44, 0x96, 0x8B, 0x2C, 0x44, 0xC9, 0x8E, 0x2C, 0x44, 0xE7, 0x93,
	0x2C, 0x44, 0xE7, 0x93, 0x2C, 0x44, 0xE7, 0x93, 0x2C, 0x44, 0xD3, 0x95, 0x2C, 0x44, 0xD3, 0x95,
	0x2C, 0x44, 0x77, 0x96, 0x2C, 0x44, 0x77, 0x96, 0x2C, 0x44, 0xAC, 0xDC, 0xE6, 0x43, 0x77, 0x96,
	0x2C, 0x44, 0xBE, 0x97, 0x2C, 0x44, 0xAA, 0x99, 0x2C, 0x44, 0xAA, 0x99, 0x2C, 0x44, 0x4E, 0x9A,
	0x2C, 0x44, 0x4E, 0x9A, 0x2C, 0x44, 0x4E, 0x9A, 0x2C, 0x44, 0xF2, 0x9A, 0x2C, 0x44}

func TestSysTimeMarshal(t *testing.T) {
	ts := sysTime(tm)
	res, err := ts.MarshalBinary()
	if err != nil {
		t.Error(err)
	}
	if len(res) != len(bt) {
		t.Error("systime marshal failed. Wrong length")
	}
	for i := range bt {
		if res[i] != bt[i] {
			t.Error("systime marshal failed. values don't match")
		}
	}

}

func TestSysTimeUnmarshal(t *testing.T) {
	var ts sysTime
	err := ts.UnmarshalBinary(bt)
	if err != nil {
		t.Error(err)
	}
	res := time.Time(ts)
	if res != tm {
		t.Errorf("systime unmarshal failed. expected [%v] but was [%v]", tm, res)
	}
}

func TestSysTimeUnmarshalFail(t *testing.T) {
	var ts sysTime
	var short = []byte{22, 2, 3}
	err := ts.UnmarshalBinary(short)
	if err == nil {
		t.Error("unmarshal input validation failed.")
	}
	if err != ErrTooShort {
		t.Errorf("unmarshal input validation failed. Wrong error is returned. %v", err)
	}
}

func TestChanLogUnmarshal(t *testing.T) {
	var res ChannelLog
	err := res.UnmarshalBinary(chanLogPayload)
	if err != nil {
		t.Error(err)
	}
	if len(res.Values) != 25 {
		t.Errorf("wrong number of log values. Expected %d, got %d", 25, len(res.Values))
	}
	start := time.Date(2022, 9, 5, 13, 0, 0, 0, time.Local)
	if start != res.Start {
		t.Errorf("wrong start date. Expected %v got %v", start, res.Start)
	}
	if res.Id != 1 {
		t.Errorf("wrong channel Number. Expected %d got %d", 1, res.Id)
	}
}

func TestChanLogUnmarshalFail(t *testing.T) {
	var res ChannelLog
	var short = []byte{22, 2, 3}
	err := res.UnmarshalBinary(short)
	if err == nil {
		t.Error("unmarshal input validation failed.")
	}
	if err != ErrTooShort {
		t.Errorf("unmarshal input validation failed. Wrong error is returned. %v", err)
	}
}

func TestCrc(t *testing.T) {
	tests := []struct {
		input    string
		expected crc
	}{
		{"0207", 0x1241},
		{"01040040000a", 0xD971},
		{"0104143a11e6ee3b16c44d39e24257381730ba3d862437", 0xAED0},
	}

	var c crc
	for _, test := range tests {
		c.reset()
		v, _ := hex.DecodeString(test.input)
		c.update(v)
		if c != test.expected {
			t.Errorf("crc fail: expected %#x, actual: %#x", test.expected, c)
		}
	}
}
