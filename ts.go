package main

import (
	"fmt"
	"log"
	"net"
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
func pollStatus(listener net.Listener, r *http.Request, server *tsnet.Server, client *local.Client, conn *websocket.Conn) {
	var authDelivered bool

	for i := 0; i < 600; i++ {
		status, err := client.Status(r.Context())
		if err != nil {
			log.Fatalf("ts status %q: %v", status.BackendState, err)
		}

		switch status.BackendState {
		case "NeedsLogin":
			if authDelivered || status.AuthURL == "" {
				break
			}

			msg := fmt.Sprintf("Auth required. Go to: %v\r\n", status.AuthURL)

			if err = conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				log.Fatalf("ws write status %q: %v", status.BackendState, err)
			}

			authDelivered = true
		case "Running":
			// Waiting a bit might prevent the frontend from initiating
			// the ts WebSocket connection too quickly.
			time.Sleep(1 * time.Second)

			tsIp4, tsIp6 := server.TailscaleIPs()
			hostname := status.Self.HostName

			msg := fmt.Sprintf("Tailscale machine %v at %v %v\r\n", hostname, tsIp4, tsIp6)

			if err = conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				log.Fatalf("ws write status %q: %v", status.BackendState, err)
			}
			log.Print(msg)

			conn.Close()
			return
		}

		time.Sleep(1 * time.Second)
	}

	msg := fmt.Sprintf("%v init timed out.\r\n", server.Hostname)

	if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		log.Fatalf("ws write: %v", err)
	}
	log.Print(msg)

	conn.Close()
	listener.Close()
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
