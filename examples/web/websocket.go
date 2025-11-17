package main

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

type WebSocketHub struct {
	clients    map[string]*WebSocketClient
	mu         sync.RWMutex
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	broadcast  chan BroadcastMessage
	logger     logger.Logger
	done       chan struct{}
}

type WebSocketClient struct {
	ID     string
	UserID string
	Conn   *websocket.Conn
	Send   chan []byte
	hub    *WebSocketHub
}

type BroadcastMessage struct {
	UserID  string
	Event   string
	Payload map[string]any
}

func NewWebSocketHub(lgr logger.Logger) *WebSocketHub {
	if lgr == nil {
		lgr = &logger.Nop{}
	}
	return &WebSocketHub{
		clients:    make(map[string]*WebSocketClient),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
		broadcast:  make(chan BroadcastMessage, 256),
		logger:     lgr,
		done:       make(chan struct{}),
	}
}

func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.ID] = client
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				close(client.Send)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for _, client := range h.clients {
				if message.UserID == "" || client.UserID == message.UserID {
					select {
					case client.Send <- h.marshalMessage(message):
					default:
						close(client.Send)
						delete(h.clients, client.ID)
					}
				}
			}
			h.mu.RUnlock()

		case <-h.done:
			return
		}
	}
}

func (h *WebSocketHub) Close() {
	close(h.done)
}

func (h *WebSocketHub) Broadcast(ctx context.Context, event broadcaster.Event) error {
	// Extract user ID from payload if it's a map
	userID := ""
	if payload, ok := event.Payload.(map[string]any); ok {
		if uid, ok := payload["user_id"].(string); ok {
			userID = uid
		}
	}

	msg := BroadcastMessage{
		UserID:  userID,
		Event:   event.Topic,
		Payload: event.Payload.(map[string]any),
	}

	select {
	case h.broadcast <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return nil
	}
}

func (h *WebSocketHub) marshalMessage(msg BroadcastMessage) []byte {
	data, _ := json.Marshal(map[string]any{
		"event":   msg.Event,
		"payload": msg.Payload,
	})
	return data
}

func (h *WebSocketHub) RegisterClient(client *WebSocketClient) {
	h.register <- client
}

func (h *WebSocketHub) UnregisterClient(client *WebSocketClient) {
	h.unregister <- client
}

func (c *WebSocketClient) HandleConnection() {
	defer func() {
		c.hub.UnregisterClient(c)
		c.Conn.Close()
	}()

	go c.writePump()
	c.readPump()
}

func (c *WebSocketClient) readPump() {
	defer func() {
		c.hub.UnregisterClient(c)
		c.Conn.Close()
	}()

	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
