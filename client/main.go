package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"golang.org/x/net/websocket"
	"github.com/cascades-fbp/cascades/components/utils"
	"github.com/cascades-fbp/cascades/runtime"
	zmq "github.com/pebbe/zmq4"
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
	err                          error
)

type Connection struct {
	WS      *websocket.Conn
	Send    chan []byte
	Receive chan []byte
}

func NewConnection(wsConn *websocket.Conn) *Connection {
	connection := &Connection{
		WS:      wsConn,
		Send:    make(chan []byte, 256),
		Receive: make(chan []byte, 256),
	}
	return connection
}

func (self *Connection) Reader() {
	for {
		var data []byte
		err := websocket.Message.Receive(self.WS, &data)
		if err != nil {
			log.Println("Reader: error receiving message")
			continue
		}
		log.Println("Reader: received from websocket", data)
		self.Receive <- data
	}
	self.WS.Close()
}

func (self *Connection) Writer() {
	for data := range self.Send {
		log.Println("Writer: will send to websocket", data)
		err := websocket.Message.Send(self.WS, data)
		if err != nil {
			break
		}
	}
	self.WS.Close()
}

func (self *Connection) Close() {
	close(self.Send)
	close(self.Receive)
	self.WS.Close()
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
	optionsPort, err = utils.CreateInputPort(*optionsEndpoint)
	utils.AssertError(err)

	inPort, err = utils.CreateInputPort(*inputEndpoint)
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

	openPorts()
	defer closePorts()

	utils.HandleInterruption()

	// Wait for the configuration on the options port
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

	go connection.Reader()
	go connection.Writer()
	go func() {
		outPort, err = utils.CreateOutputPort(*outputEndpoint)
		utils.AssertError(err)
		for data := range connection.Receive {
			log.Println("Sending data from websocket to OUT port...")
			outPort.SendMessage(runtime.NewPacket(data))
		}
	}()
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
