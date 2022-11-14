package pulsar

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

// default i/o frame operations timeout
const timeout = time.Second * 5

type Conn interface {
	// PrepareWrite configures frame writing operation. Call it once before frame sequential writes.
	PrepareWrite() error
	// PrepareRead configures frame reading operation. Call it once before frame sequential reads.
	PrepareRead() error
	// logs written frame
	LogRequest()
	// logs received frame
	LogResponse()
	// Write writes data from p into the socket.
	Write(data []byte) (int, error)
	// Read reads up to len(p) bytes into p. It returns the number of bytes
	// read (0 <= n <= len(p)) and any error encountered.
	Read(p []byte) (int, error)
	// Flush writes any buffered data to the underlying io.Writer.
	Flush() error
	// Close closes the connection.
	Close() error
}

// tcpConn is a network connection handle
type tcpConn struct {
	// wrapped connection
	rwc net.Conn
	// i/o operations timeout
	to time.Duration
	// buffered reader handler.
	r reader
	//buffered writer handler
	w writer
}

// Close closes the connection.
func (c *tcpConn) Close() error {
	return c.rwc.Close()
}

// prepareRead configures frame reading operation. Call it once before frame sequential reads.
func (c *tcpConn) PrepareRead() error {
	c.r.reset(c.rwc)
	if err := c.rwc.SetReadDeadline(time.Now().Add(c.to)); err != nil {
		return err
	}
	return nil
}

// prepareWrite configures frame writing operation. Call it once before frame sequential writes.
func (c *tcpConn) PrepareWrite() error {
	c.w.reset(c.rwc)
	if err := c.rwc.SetWriteDeadline(time.Now().Add(c.to)); err != nil {
		return err
	}
	return nil
}

// write the contents of p into device.
// It returns the number of bytes written from p (0 <= n <= len(p))
// and any error encountered that caused the write to stop early.
func (c *tcpConn) Write(p []byte) (int, error) {
	return c.w.Write(p)
}

// flush writes any buffered data to the network.
func (c *tcpConn) Flush() error {
	return c.w.Flush()
}

// read reads up to len(p) bytes into p. It returns the number of bytes
// read (0 <= n <= len(p)) and any error encountered.
func (c *tcpConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

// logs received frame
func (c *tcpConn) LogResponse() {
	c.r.Log("response")
}

// logs written frame
func (c *tcpConn) LogRequest() {
	c.w.Log("request")
}

// A Dialer contains options for connecting to a network.
type Dialer struct {
	// Tcp socket connection timeout.
	ConnectionTimeOut time.Duration
	// I/O frame operations timeout.
	RWTimeOut time.Duration
	// Logger for received and sent frames.
	ProtocolLogger *log.Logger
}

// DialTCP connects to the tcp socket on the named network.
// The socket has the form "host:port".
func DialTCP(socket string) (Conn, error) {
	var d Dialer
	return d.DialTCP(socket)
}

// DialTCP connects to the tcp socket on the named network.
// The socket has the form "host:port".
func (d *Dialer) DialTCP(socket string) (Conn, error) {
	conn, err := net.DialTimeout("tcp", socket, d.ConnectionTimeOut)
	if err != nil {
		return nil, err
	}

	var to = d.RWTimeOut
	if to == 0 {
		to = timeout
	}
	return newConn(conn, d.ProtocolLogger, to), nil
}

// creates connection.
func newConn(conn net.Conn, log *log.Logger, to time.Duration) *tcpConn {
	var l = &logger{
		log: log,
	}
	return &tcpConn{
		conn,
		to,
		reader{
			l,
			bufio.NewReader(conn),
		},
		writer{
			l,
			bufio.NewWriter(conn),
		},
	}
}

// Frame logger
type logger struct {
	// buffer for partial reads writes.
	buf bytes.Buffer
	// logger
	log *log.Logger
}

// Log logs read or written frame. Contents are reset on prepareRead or prepareWrite methods call.
func (l *logger) Log(prefix string) {
	if l.log != nil {
		l.log.Println(formatMsg(prefix, l.buf.Bytes()))
	}
	l.buf.Reset()
}

// Buffered reader that logs read bytes.
type reader struct {
	*logger
	*bufio.Reader
}

// reset discards any buffered data. Also resets collected frame's log message.
func (b *reader) reset(r io.Reader) {
	b.Reader.Reset(r)
	b.logger.buf.Reset()
}

// io.Reader interface implementation.
// Read reads data into p and appends it to frame's log message.
func (b *reader) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == nil && b.log != nil {
		_, err = b.logger.buf.Write(p)
	}
	return n, err
}

// Buffered writer that logs written bytes
type writer struct {
	*logger
	*bufio.Writer
}

// reset discards any buffered data. Also resets collected frame's log message.
func (b *writer) reset(w io.Writer) {
	b.Writer.Reset(w)
	b.logger.buf.Reset()
}

// io.Writer implementation.
// Write writes data from p into the socket.
func (b *writer) Write(p []byte) (int, error) {
	nn, err := b.Writer.Write(p)
	if err == nil && b.log != nil {
		_, err = b.logger.buf.Write(p)
	}
	return nn, err
}

// formats frame log as two areas. On the left side frame bytes as hex bytes, on the right is a string representation.
func formatMsg(prefix string, data []byte) string {
	var b1 strings.Builder

	b1.WriteString(prefix)
	b1.WriteRune('\n')
	for i := 0; i < len(data); i += 16 {
		end := i + 16
		if end > len(data) {
			end = len(data)
		}
		for _, b := range data[i:end] {
			_, _ = fmt.Fprintf(&b1, "%02X ", b)
		}
		b1.WriteString(strings.Repeat(" ", 58-(3*(end-i))))
		b1.WriteString(strings.Map(mapNotPrintable, string(data[i:end])))
		b1.WriteRune('\n')
	}

	return b1.String()
}

// replaces non-printable runes with dots '.'
func mapNotPrintable(r rune) rune {
	if strconv.IsPrint(r) {
		return r
	}
	return '.'
}
