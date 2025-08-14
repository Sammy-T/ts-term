package main

import (
	"fmt"
	"log"
	"net"

	"github.com/gorilla/websocket"
	ws "github.com/sammy-t/ts-term/internal/websocket"
)

// connLog is a helper to output to the log and close the associated
// Websocket connection and net listener.
type connLog struct {
	conn     *ws.SyncedWebsocket
	listener net.Listener
}

// LessFatalf writes the error to the log and WebSocket.
// Then closes the WebSocket and net listener.
func (c connLog) LessFatalf(format string, v ...any) {
	log.Printf(format, v...)

	msg := fmt.Sprintf(format, v...)

	if err := c.conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		log.Printf("connlog conn write: %v", err)
	}

	if err := c.conn.Close(); err != nil {
		log.Printf("connlog conn close: %v", err)
	}

	if err := c.listener.Close(); err != nil {
		log.Printf("connlog listener close: %v", err)
	}
}
