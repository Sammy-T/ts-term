package websocket

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/gorilla/websocket"
)

type Hub struct {
	Conn      *SyncedWebsocket
	listeners map[MessageType][]chan msgResp
}

type msgResp struct {
	msg Message
	err error
}

func NewHub(conn *SyncedWebsocket) Hub {
	h := Hub{
		Conn:      conn,
		listeners: make(map[MessageType][]chan msgResp),
	}

	go h.listen()

	return h
}

func (h Hub) listen() {
	readLimit := 60 * time.Second

	h.Conn.SetReadDeadline(time.Now().Add(readLimit))

	h.Conn.SetPongHandler(func(appData string) error {
		h.Conn.SetReadDeadline(time.Now().Add(readLimit))
		return nil
	})

	var err error

	for {
		if err = readMessages(h); err != nil {
			log.Printf("closing hub listener: %v", err)
			return
		}
	}
}

func (h Hub) AwaitMsg(msgType MessageType, timeout time.Duration) (Message, error) {
	ch := make(chan msgResp)

	var msg Message
	var err error

	go func() {
		if timeout.Milliseconds() == 0 {
			return
		}

		time.Sleep(timeout)

		if msg.Type == msgType || err != nil {
			return
		}

		ch <- msgResp{err: fmt.Errorf("await msg %q timed out", msgType)}
	}()

	h.listeners[msgType] = append(h.listeners[msgType], ch)

	mResp := <-ch

	msg = mResp.msg
	err = mResp.err

	h.listeners[msgType] = slices.DeleteFunc(h.listeners[msgType], func(channel chan msgResp) bool {
		return channel == ch
	})

	return msg, err
}

func readMessages(h Hub) error {
	msgType, p, err := h.Conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read msg: %v", err)
	}

	// There shouldn't be any binary messages
	if msgType != websocket.TextMessage {
		return fmt.Errorf("invalid msg type: %v", msgType)
	}

	var msg Message

	if err = json.Unmarshal(p, &msg); err != nil {
		return err
	}

	log.Printf("hub msg: %q", msg.Type)

	switch msg.Type {
	case MessageError, MessageSshErr, MessageWsError:
		resp := msgResp{
			msg: msg,
			err: errors.New(string(msg.Type)),
		}

		for _, channels := range h.listeners {
			for _, respChan := range channels {
				respChan <- resp
			}
		}
		return nil
	}

	channels, ok := h.listeners[msg.Type]
	if !ok {
		return nil
	}

	for _, ch := range channels {
		ch <- msgResp{msg: msg, err: nil}
	}

	return nil
}
