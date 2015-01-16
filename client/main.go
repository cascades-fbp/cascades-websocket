package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"syscall"
	"time"

	"github.com/cascades-fbp/cascades/components/utils"
	"github.com/cascades-fbp/cascades/runtime"
	zmq "github.com/pebbe/zmq4"
	"golang.org/x/net/websocket"
)

var (
	// Flags
	optionsEndpoint = flag.String("port.options", "", "Component's options port endpoint")
	inputEndpoint   = flag.String("port.in", "", "Component's input port endpoint")
	outputEndpoint  = flag.String("port.out", "", "Component's output port endpoint")
	jsonFlag        = flag.Bool("json", false, "Print component documentation in JSON")
	debug           = flag.Bool("debug", false, "Enable debug mode")

	// Internal
	optionsPort, inPort, outPort *zmq.Socket
	inCh, outCh                  chan bool
	err                          error
)

// Connection container
type Connection struct {
	WS      *websocket.Conn
	Send    chan []byte
	Receive chan []byte
}

// NewConnection constructs Connection structures
func NewConnection(wsConn *websocket.Conn) *Connection {
	connection := &Connection{
		WS:      wsConn,
		Send:    make(chan []byte, 256),
		Receive: make(chan []byte, 256),
	}
	return connection
}

// Reader is reading from socket and writes to Receive channel
func (c *Connection) Reader() {
	for {
		var data []byte
		err := websocket.Message.Receive(c.WS, &data)
		if err != nil {
			log.Println("Reader: error receiving message")
			continue
		}
		log.Println("Reader: received from websocket", data)
		c.Receive <- data
	}
	c.WS.Close()
}

// Writer is reading from channel and writes to socket
func (c *Connection) Writer() {
	for data := range c.Send {
		log.Println("Writer: will send to websocket", data)
		err := websocket.Message.Send(c.WS, data)
		if err != nil {
			break
		}
	}
	c.WS.Close()
}

// Close closes the channels and socket
func (c *Connection) Close() {
	close(c.Send)
	close(c.Receive)
	c.WS.Close()
}

func validateArgs() {
	if *optionsEndpoint == "" {
		flag.Usage()
		os.Exit(1)
	}
	if *inputEndpoint == "" {
		flag.Usage()
		os.Exit(1)
	}
	if *outputEndpoint == "" {
		flag.Usage()
		os.Exit(1)
	}
}

func openPorts() {
	optionsPort, err = utils.CreateInputPort("websocket/client.options", *optionsEndpoint, nil)
	utils.AssertError(err)

	inPort, err = utils.CreateInputPort("websocket/client.in", *inputEndpoint, inCh)
	utils.AssertError(err)
}

func closePorts() {
	optionsPort.Close()
	inPort.Close()
	if outPort != nil {
		outPort.Close()
	}
	zmq.Term()
}

func main() {
	flag.Parse()

	if *jsonFlag {
		doc, _ := registryEntry.JSON()
		fmt.Println(string(doc))
		os.Exit(0)
	}

	log.SetFlags(0)
	if *debug {
		log.SetOutput(os.Stdout)
	} else {
		log.SetOutput(ioutil.Discard)
	}

	validateArgs()

	ch := utils.HandleInterruption()
	inCh = make(chan bool)
	outCh = make(chan bool)

	openPorts()
	defer closePorts()

	log.Println("Waiting for configuration...")
	var connectString string
	for {
		ip, err := optionsPort.RecvMessageBytes(0)
		if err != nil {
			log.Println("Error receiving IP:", err.Error())
			continue
		}
		if !runtime.IsValidIP(ip) || !runtime.IsPacket(ip) {
			continue
		}
		connectString = string(ip[1])
		break
	}
	optionsPort.Close()

	// Establish WS connection
	hostname, _ := os.Hostname()
	origin := fmt.Sprintf("http://%s", hostname)
	ws, err := websocket.Dial(connectString, "", origin)
	utils.AssertError(err)
	connection := NewConnection(ws)
	defer connection.Close()

	// Sender goroutine
	go func() {
		outPort, err = utils.CreateOutputPort("websocket/client.out", *outputEndpoint, outCh)
		utils.AssertError(err)
		for data := range connection.Receive {
			log.Println("Sending data from websocket to OUT port...")
			outPort.SendMessage(runtime.NewPacket(data))
		}
	}()

	waitCh := make(chan bool)
	go func() {
		total := 0
		for {
			select {
			case v := <-inCh:
				if !v {
					log.Println("IN port is closed. Interrupting execution")
					ch <- syscall.SIGTERM
				} else {
					total++
				}
			case v := <-outCh:
				if !v {
					log.Println("OUT port is closed. Interrupting execution")
					ch <- syscall.SIGTERM
				} else {
					total++
				}
			}
			if total >= 2 && waitCh != nil {
				waitCh <- true
			}
		}
	}()

	log.Println("Waiting for port connections to establish... ")
	select {
	case <-waitCh:
		log.Println("Ports connected")
		waitCh = nil
	case <-time.Tick(30 * time.Second):
		log.Println("Timeout: port connections were not established within provided interval")
		os.Exit(1)
	}

	go connection.Reader()
	go connection.Writer()

	log.Println("Started")

	// Listen to packets from IN port
	for {
		ip, err := inPort.RecvMessageBytes(0)
		if err != nil {
			log.Println("Error receiving message:", err.Error())
			continue
		}
		if !runtime.IsValidIP(ip) {
			continue
		}

		log.Println("Sending data from IN port to websocket...")
		connection.Send <- ip[1]
	}
}
