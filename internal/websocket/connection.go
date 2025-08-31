package websocket

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

func PingConn(conn *SyncedWebsocket, interval time.Duration) {
	go func() {
		for {
			time.Sleep(interval)

			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("ping err: %v", err)
				return
			}
		}
	}()
}

func AwaitMsg(conn *SyncedWebsocket, msgType MessageType, timeout time.Duration) (Message, error) {
	ch := make(chan int)
	closeCh := func() { ch <- 0 }

	var msg Message
	var err error

	log.Printf("Awaiting %q...\n", msgType)

	go func() {
		defer closeCh()

		if timeout.Milliseconds() == 0 {
			return
		}

		time.Sleep(timeout)

		if msg.Type == msgType || err != nil {
			return
		}

		err = errors.New("websocket timed out")
	}()

	go func() {
		defer closeCh()

		for {
			if err = conn.ReadJSON(&msg); err != nil {
				return
			}

			// Check for the desired message, ignore other valid messages,
			// and exit on error or on an invalid message.
			switch msg.Type {
			case msgType:
				return
			case MessageInfo, MessagePeers, MessageWsOpened, MessageSize:
				continue
			case MessageSshCfg, MessageSshHost, MessageSshHostAct, MessageSshSuccess:
				continue
			case MessageInput, MessageOutput:
				continue
			case MessageError, MessageSshErr, MessageWsError:
				err = errors.New(string(msg.Type))
				return
			default:
				err = fmt.Errorf("invalid msg received. [%v] %q", msg.Type, msg.Data)
				return
			}
		}
	}()

	<-ch

	return msg, err
}
