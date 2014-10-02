package utils

import (
	"encoding/json"
	"github.com/cascades-fbp/cascades/runtime"
)

// Message incapsulates connection ID it received from or to be sent to
// and a payload received from or should be sent to Websocket connection.
type Message struct {
	CID     string      `json:"cid"`
	Payload interface{} `json:"payload"`
}

// Converts a given message to IP
func Message2IP(msg *Message) ([][]byte, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return runtime.NewPacket(payload), nil
}

// Converts a given IP to message structure
func IP2Message(ip [][]byte) (*Message, error) {
	var msg *Message
	err := json.Unmarshal(ip[1], &msg)
	if err != nil {
		return nil, err
	}
	return msg, nil
}
