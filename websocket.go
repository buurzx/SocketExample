package main

import (
	"encoding/json"

	"github.com/gorilla/websocket"
)

// ClientManager will keep track of all the
// connected clients, clients that are trying
// to become registered, clients that have become
// destroyed and are waiting to be removed,
// and messages that are to be broadcasted to and from all connected clients.
type ClientManager struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

// Client has a unique id, a socket connection, and a message waiting to be sent.
type Client struct {
	id     string
	socket *websocket.Conn
	send   chan []byte
}

type Message struct {
	Sender    string `json:"sender,omitempty"`
	Recipient string `json:"recipient,omitempty"`
	Content   string `json:"content,omitempty"`
}

var manager = ClientManager{
	broadcast:  make(chan []byte),
	register:   make(chan *Client),
	unregister: make(chan *Client),
	clients:    make(map[*Client]bool),
}

// Every time the manager.register channel has data,
// the client will be added to the map of available clients
// managed by the client manager. After adding the client,
// a JSON message is sent to all other clients,
// not including the one that just connected.

// If a client disconnects for any reason,
// the manager.unregister channel will have data.
// The channel data in the disconnected client will
// be closed and the client will be removed from the
// client manager. A message announcing the
// disappearance of a socket will be sent to all remaining connections.

// If the manager.broadcast channel has data
// it means that we’re trying to send and receive
// messages. We want to loop through each managed
// client sending the message to each of them. If
// for some reason the channel is clogged or the
// message can’t be sent, we assume the client
// has disconnected and we remove them instead.
func (manager *ClientManager) start() {
	for {
		select {
		case conn := <-manager.register:
			manager.clients[conn] = true
			jsonMessage, _ := json.Marshal(&Message{Content: "/A new socket has connected."})
			manager.send(jsonMessage, conn)
		case conn := <-manager.unregister:
			if _, ok := manager.clients[conn]; ok {
				close(conn.send)
				delete(manager.clients, conn)
				jsonMessage, _ := json.Marshal(&Message{Content: "/A socket has disconnected."})
				manager.send(jsonMessage, conn)
			}
		case message := <-manager.broadcast:
			for conn := range manager.clients {
				select {
				case conn.send <- message:
				default:
					close(conn.send)
					delete(manager.clients, conn)
				}
			}
		}
	}
}

func (manager *ClientManager) send(message []byte, ignore *Client) {
	for conn := range manager.clients {
		if conn != ignore {
			conn.send <- message
		}
	}
}

// The point of this goroutine is to read the socket data and
// add it to the manager.broadcast for further orchestration
func (c *Client) read() {
	defer func() {
		manager.unregister <- c
		c.socket.Close()
	}()

	for {
		_, message, err := c.socket.ReadMessage()
		// If there was an error reading the websocket data
		// it probably means the client has disconnected.
		// If that is the case we need to unregister the client from our server.
		if err != nil {
			manager.unregister <- c
			c.socket.Close()
			break
		}
		jsonMessage, _ := json.Marshal(&Message{Sender: c.id, Content: string(message)})
		manager.broadcast <- jsonMessage
	}
}

func (c *Client) write() {
	defer func() {
		c.socket.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.socket.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.socket.WriteMessage(websocket.TextMessage, message)
		}
	}
}
