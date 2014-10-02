# Websocket components for Cascades FBP

This repository contains the following components:

 * Websocket server
 * Websocket client

## Websocket Server component

Usage of Websocket server component:

```
$ ./components/websocket/server
Usage of ./components/websocket/server:
  -debug=false: Enable debug mode
  -json=false: Print component documentation in JSON
  -port.in="": Component's input port endpoint
  -port.options="": Component's options port endpoint
  -port.out="": Component's output port endpoint
```

Example (Websocket echo server):

```
# Configure server and forward its output to packet cloning component
'127.0.0.1:8000' -> OPTIONS Server(websocket/server)
Server OUT -> IN Demux(core/splitter)

# Show incoming data on the screen
Demux OUT[0] -> IN Log(core/console)

# Return receive data back to server's client
Demux OUT[1] -> IN Server
```

Running example:

```
$ bin/cascades run examples/tcp-server.fbp
runtime | Starting processes...
runtime | Activating processes by sending IIPs...
runtime | Sending 'localhost:9999' to socket 'tcp://127.0.0.1:5000'
```

Open http://www.websocket.org/echo.html and type in the following location `ws://localhost:9999/ws`. Then enter the following message `{"msg":"Hi!"}` and send it. You should receive exactly the same response back.
