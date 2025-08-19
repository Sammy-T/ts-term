package websocket

type MessageType string

const (
	MessageInfo       MessageType = "info"
	MessagePeers      MessageType = "peers"
	MessageSshCfg     MessageType = "ssh-config"
	MessageSshHost    MessageType = "ssh-host"
	MessageSshHostAct MessageType = "ssh-host-action"
	MessageSshErr     MessageType = "ssh-error"
	MessageSshSuccess MessageType = "ssh-success"
	MessageWsOpened   MessageType = "ts-websocket-opened"
	MessageWsError    MessageType = "ts-websocket-error"
	MessageSize       MessageType = "size"
	MessageInput      MessageType = "input"
	MessageOutput     MessageType = "output"
	MessageError      MessageType = "error"
)

type Message struct {
	Type MessageType `json:"type"`
	Data string      `json:"data"`
}
