package pulsar_test

import (
	pulsar "github.com/srgsf/tvh-pulsar"
	"log"
	"os"
	"time"
)

// This is a usage example.
func Example() {
	//Configure Dialer
	d := pulsar.Dialer{
		ConnectionTimeOut: 30 * time.Second,
		ProtocolLogger:    log.New(os.Stderr, "PROTOCOL ", log.Ldate|log.Lmicroseconds),
		RWTimeOut:         20 * time.Second,
	}

	//Get a tcp connection to rs485 to Ethernet converter on port 4001
	conn, err := d.DialTCP("rs485converter:4001")
	if err != nil {
		println(err)
		os.Exit(-1)
	}

	//Create a client with device number 12345678
	c, err := pulsar.NewClient("12345678", conn)
	if err != nil {
		println(err)
		_ = conn.Close()
		os.Exit(-1)
	}
	defer func() { _ = conn.Close() }()

	//Retrieve current values from 1st and 2nd channels.
	res, err := c.CurValues(1, 2)
	if err != nil {
		log.Fatalf("unable to retrieve current values. Error %s", err.Error())
	}
	//Print results.
	for _, ch := range res {
		log.Printf("Current value of channel %d is %.2f", ch.Id, ch.Value)
	}
}
