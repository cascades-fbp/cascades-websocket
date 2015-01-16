package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"syscall"
	"time"

	wsutils "github.com/cascades-fbp/cascades-websocket/utils"
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
	optionsPort, err = utils.CreateInputPort("websocket/server.options", *optionsEndpoint, nil)
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

	// Receiver routine
	go func() {
		inPort, err = utils.CreateInputPort("websocket/server.in", *inputEndpoint, inCh)
		utils.AssertError(err)
		for {
			ip, err := inPort.RecvMessageBytes(0)
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
		outPort, err = utils.CreateOutputPort("websocket/server.out", *outputEndpoint, outCh)
		utils.AssertError(err)
		for msg := range DefaultHub.Incoming {
			log.Printf("Received data from connection: %#v\n", msg)
			ip, err := wsutils.Message2IP(&msg)
			if err != nil {
				log.Println("Failed to convert Message to IP:", err.Error())
				continue
			}
			outPort.SendMessageDontwait(ip)
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

	log.Println("Waiting for configuration...")
	var bindAddr string
	for {
		ip, err := optionsPort.RecvMessageBytes(0)
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
