package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"tailscale.com/client/local"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"
)

func createHostName() string {
	uuid, err := uuid.NewV7()
	if err != nil {
		log.Fatalf("uuid: %v\n", err)
	}

	idSplit := strings.Split(uuid.String(), "-")

	return "ts-term-" + idSplit[len(idSplit)-1]
}

// pollStatus polls the status of the TS server until the server is running
// and outputs relevant status info to the WebSocket.
func pollStatus(r *http.Request, server *tsnet.Server, client *local.Client, conn *websocket.Conn) error {
	var authDelivered bool

	for i := 0; i < 600; i++ {
		status, err := client.Status(r.Context())
		if err != nil {
			return fmt.Errorf("ts status %q: %v", status.BackendState, err)
		}

		switch status.BackendState {
		case "NeedsLogin":
			if authDelivered || status.AuthURL == "" {
				break
			}

			msg := fmt.Sprintf("Auth required. Go to: %v\r\n", status.AuthURL)

			if err = conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				return fmt.Errorf("ws write status %q: %v", status.BackendState, err)
			}

			authDelivered = true
		case "Running":
			tsIp4, tsIp6 := server.TailscaleIPs()
			hostname := status.Self.HostName

			msg := fmt.Sprintf("Tailscale machine %v at %v %v\r\n", hostname, tsIp4, tsIp6)

			if err = conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				return fmt.Errorf("ws write status %q: %v", status.BackendState, err)
			}
			log.Print(msg)

			return nil
		}

		time.Sleep(1 * time.Second)
	}

	msg := fmt.Sprintf("%v init timed out.", server.Hostname)

	if err := conn.WriteMessage(websocket.TextMessage, []byte(msg+"\r\n")); err != nil {
		return fmt.Errorf("ws write: %v", err)
	}

	return errors.New(msg)
}

// awaitTsWsConnection uses the provided Websocket to await notification
// of a successful TS WebSocket connection. An error is returned on an
// error notification or on a timeout.
func awaitTsWsConnection(conn *websocket.Conn) error {
	go func() {
		time.Sleep(30 * time.Second)

		msg := websocket.FormatCloseMessage(websocket.CloseGoingAway, "websocket timed out.")
		conn.WriteMessage(websocket.CloseMessage, msg)
	}()

	msgType, msg, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read err: %v", err)
	}

	if msgType != websocket.TextMessage {
		return fmt.Errorf("invalid msg type received. [%v]", msgType)
	}

	switch string(msg) {
	case "ts-websocket-opened":
		return nil
	case "ts-websocket-error":
		return errors.New("websocket errored")
	default:
		return fmt.Errorf("invalid msg received. %q", string(msg))
	}
}

// createUpgraderTs creates a WebSocket Upgrader with a CheckOrigin function
// that verifies requests against the provided Tailscale server.
func createUpgraderTs(client *local.Client) websocket.Upgrader {
	checkOrigin := func(r *http.Request) bool {
		if dev {
			return true
		}

		status, err := client.Status(r.Context())
		if err != nil {
			log.Fatalf("ts status %q: %v", status.BackendState, err)
		}

		validOriginHosts := getTailnetAddresses(status)

		host := r.Host
		originHdr := r.Header.Get("Origin")
		var origin string

		parsedOrigin, err := url.Parse(originHdr)
		if err == nil {
			// Reconstruct the request origin's host ignoring the port
			// since we're allowing any machine in the tailnet
			// to host the frontend.
			origin = strings.Split(parsedOrigin.Host, ":")[0]
		}

		log.Printf("Check origin\nhost: %q\noriginHdr: %q\norigin: %q\n", host, originHdr, origin)

		return host == origin || slices.Contains(validOriginHosts, origin)
	}

	tsUpgrader := websocket.Upgrader{
		ReadBufferSize:  bufferSize,
		WriteBufferSize: bufferSize,
		CheckOrigin:     checkOrigin,
	}

	return tsUpgrader
}

// getTailnetAddresses returns all the Tailscale domains and IP addresses
// on the Tailnet.
func getTailnetAddresses(status *ipnstate.Status) []string {
	domain := status.Self.DNSName
	shortDomain := strings.Split(domain, ".")[0]
	tsIps := status.TailscaleIPs

	addresses := []string{domain, shortDomain}

	for _, ip := range tsIps {
		addresses = append(addresses, ip.String())
	}

	for _, peerStatus := range status.Peer {
		domain = peerStatus.DNSName
		shortDomain = strings.Split(domain, ".")[0]
		tsIps = peerStatus.TailscaleIPs

		addresses = append(addresses, domain, shortDomain)

		for _, ip := range tsIps {
			addresses = append(addresses, ip.String())
		}
	}

	return addresses
}
