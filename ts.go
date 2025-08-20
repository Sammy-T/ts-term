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
	ws "github.com/sammy-t/ts-term/internal/websocket"
	"tailscale.com/client/local"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"
)

type PeerConnInfo struct {
	Domain      string   `json:"domain"`
	ShortDomain string   `json:"shortDomain"`
	Ips         []string `json:"ips"`
}

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
func pollStatus(r *http.Request, server *tsnet.Server, client *local.Client, conn *ws.SyncedWebsocket) error {
	var authDelivered bool

	for range 600 {
		status, err := client.Status(r.Context())
		if err != nil {
			return fmt.Errorf("ts status %q: %w", status.BackendState, err)
		}

		switch status.BackendState {
		case "NeedsLogin":
			if authDelivered || status.AuthURL == "" {
				break
			}

			wsMsg := ws.Message{
				Type: ws.MessageInfo,
				Data: fmt.Sprintf("Auth required. Go to: %v", status.AuthURL),
			}

			if err = conn.WriteJSON(wsMsg); err != nil {
				return fmt.Errorf("ws write status %q: %w", status.BackendState, err)
			}

			authDelivered = true
		case "Running":
			tsIp4, tsIp6 := server.TailscaleIPs()
			hostname := status.Self.HostName

			msg := fmt.Sprintf("Tailscale machine %v at %v %v", hostname, tsIp4, tsIp6)

			wsMsg := ws.Message{
				Type: ws.MessageInfo,
				Data: msg,
			}

			if err = conn.WriteJSON(wsMsg); err != nil {
				return fmt.Errorf("ws write status %q: %w", status.BackendState, err)
			}
			log.Println(msg)

			return nil
		}

		time.Sleep(1 * time.Second)
	}

	msg := fmt.Sprintf("%v init timed out.", server.Hostname)

	wsMsg := ws.Message{
		Type: ws.MessageError,
		Data: msg,
	}

	if err := conn.WriteJSON(wsMsg); err != nil {
		return fmt.Errorf("ws write: %w", err)
	}

	return errors.New(msg)
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

		validOriginHosts := getValidHosts(status)

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

		log.Printf("Check origin\nhost: %q\noriginHdr: %q\norigin: %q", host, originHdr, origin)

		return host == origin || slices.Contains(validOriginHosts, origin)
	}

	tsUpgrader := websocket.Upgrader{
		ReadBufferSize:  bufferSize,
		WriteBufferSize: bufferSize,
		CheckOrigin:     checkOrigin,
	}

	return tsUpgrader
}

// getValidHosts returns all the Tailscale domains and IP addresses
// on the Tailnet.
func getValidHosts(status *ipnstate.Status) []string {
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

func getPeerConnInfo(r *http.Request, client *local.Client) ([]PeerConnInfo, error) {
	status, err := client.Status(r.Context())
	if err != nil {
		return nil, fmt.Errorf("ts status %q: %w", status.BackendState, err)
	}

	infos := []PeerConnInfo{}

	for _, peerStatus := range status.Peer {
		domain := peerStatus.DNSName
		shortDomain := strings.Split(domain, ".")[0]
		tsIps := peerStatus.TailscaleIPs

		ips := []string{}

		for _, ip := range tsIps {
			ips = append(ips, ip.String())
		}

		info := PeerConnInfo{
			Domain:      domain,
			ShortDomain: shortDomain,
			Ips:         ips,
		}

		infos = append(infos, info)
	}

	return infos, nil
}
