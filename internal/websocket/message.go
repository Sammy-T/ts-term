package websocket

type MessageType string

const (
	MessageStatus MessageType = "status"
	MessageInfo   MessageType = "info"
	MessagePeers  MessageType = "peers"
	MessageSize   MessageType = "size"
	MessageInput  MessageType = "input"
	MessageOutput MessageType = "output"
	MessageError  MessageType = "error"

	StatusSshCfg   string = "ssh-config"
	StatusWsOpened string = "ts-websocket-opened"
	StatusWsError  string = "ts-websocket-error"
)

type Message struct {
	Type MessageType `json:"type"`
	Data string      `json:"data"`
}
