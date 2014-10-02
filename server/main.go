package main

import (
	"code.google.com/p/go.net/websocket"
	"flag"
	"fmt"
	zmq "github.com/alecthomas/gozmq"
	wsutils "github.com/cascades-fbp/cascades-websocket/utils"
	"github.com/cascades-fbp/cascades/components/utils"
	"github.com/cascades-fbp/cascades/runtime"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

var (
	// Flags
	optionsEndpoint = flag.String("port.options", "", "Component's options port endpoint")
	inputEndpoint   = flag.String("port.in", "", "Component's input port endpoint")
	outputEndpoint  = flag.String("port.out", "", "Component's output port endpoint")
	jsonFlag        = flag.Bool("json", false, "Print component documentation in JSON")
	debug           = flag.Bool("debug", false, "Enable debug mode")

	// Internal
	context                      *zmq.Context
	optionsPort, inPort, outPort *zmq.Socket
	err                          error
)

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
	context, err = zmq.NewContext()
	utils.AssertError(err)

	optionsPort, err = utils.CreateInputPort(context, *optionsEndpoint)
	utils.AssertError(err)
}

func closePorts() {
	optionsPort.Close()
	if inPort != nil {
		inPort.Close()
	}
	if outPort != nil {
		outPort.Close()
	}
	context.Close()
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
	var bindAddr string
	for {
		ip, err := optionsPort.RecvMultipart(0)
		if err != nil {
			log.Println("Error receiving IP:", err.Error())
			continue
		}
		if !runtime.IsValidIP(ip) || !runtime.IsPacket(ip) {
			continue
		}
		bindAddr = string(ip[1])
		break
	}
	optionsPort.Close()

	// Receiver routine
	go func() {
		inPort, err = utils.CreateInputPort(context, *inputEndpoint)
		utils.AssertError(err)
		for {
			ip, err := inPort.RecvMultipart(0)
			if err != nil {
				log.Println("Error receiving message:", err.Error())
				continue
			}
			if !runtime.IsValidIP(ip) {
				continue
			}
			msg, err := wsutils.IP2Message(ip)
			if err != nil {
				log.Println("Failed to convert IP to Message:", err.Error())
				continue
			}
			log.Printf("Received response: %#v\n", msg)
			DefaultHub.Outgoing <- *msg
		}
	}()

	// Sender routine
	go func() {
		outPort, err = utils.CreateOutputPort(context, *outputEndpoint)
		utils.AssertError(err)
		for msg := range DefaultHub.Incoming {
			log.Printf("Received data from connection: %#v\n", msg)
			ip, err := wsutils.Message2IP(&msg)
			if err != nil {
				log.Println("Failed to convert Message to IP:", err.Error())
				continue
			}
			outPort.SendMultipart(ip, zmq.NOBLOCK)
		}
	}()

	// Configure & start websocket server
	http.Handle("/", websocket.Handler(WebHandler))
	go DefaultHub.Start()

	// Listen & serve
	log.Printf("Listening %v", bindAddr)
	log.Printf("Web-socket endpoint: ws://%s/", bindAddr)
	if err := http.ListenAndServe(bindAddr, nil); err != nil {
		log.Fatal("ListenAndServe Error:", err)
	}
}
