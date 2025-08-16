package log

import (
	"fmt"
	"log"
	"net"

	"github.com/gorilla/websocket"
	ws "github.com/sammy-t/ts-term/internal/websocket"
)

// ConnLog is a helper to output to the log and close the associated
// Websocket connection and net listener.
type ConnLog struct {
	Conn     *ws.SyncedWebsocket
	Listener net.Listener
}

// Printf writes to the log and WebSocket.
func (c ConnLog) Printf(format string, v ...any) {
	log.Printf(format, v...)

	msg := ws.Message{
		Type: ws.MessageInfo,
		Data: fmt.Sprintf(format, v...),
	}

	if err := c.Conn.WriteJSON(msg); err != nil {
		log.Printf("connlog conn write: %v", err)
	}
}

// LessFatalf writes the error to the log and WebSocket.
// Then closes the WebSocket and net listener.
func (c ConnLog) LessFatalf(format string, v ...any) {
	msg := websocket.FormatCloseMessage(websocket.CloseGoingAway, fmt.Sprintf(format, v...))

	if err := c.Conn.WriteMessage(websocket.CloseMessage, msg); err != nil {
		log.Printf("connlog close: %v", err)
	}

	if err := c.Listener.Close(); err != nil {
		log.Printf("connlog listener close: %v", err)
	}
}
