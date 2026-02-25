package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/athena-dhcpd/athena-dhcpd/internal/events"
	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development; tighten in production
	},
}

// WSHub manages WebSocket connections for live event streaming.
type WSHub struct {
	bus     *events.Bus
	logger  *slog.Logger
	clients map[*wsClient]struct{}
	mu      sync.Mutex
	done    chan struct{}
}

type wsClient struct {
	conn *websocket.Conn
	send chan []byte
}

// NewWSHub creates a new WebSocket hub.
func NewWSHub(bus *events.Bus, logger *slog.Logger) *WSHub {
	return &WSHub{
		bus:     bus,
		logger:  logger,
		clients: make(map[*wsClient]struct{}),
		done:    make(chan struct{}),
	}
}

// Run starts the WebSocket hub, subscribing to the event bus and broadcasting.
func (h *WSHub) Run() {
	ch := h.bus.Subscribe(500)

	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			h.broadcast(data)
		case <-h.done:
			h.bus.Unsubscribe(ch)
			return
		}
	}
}

// Stop shuts down the WebSocket hub.
func (h *WSHub) Stop() {
	close(h.done)
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		close(client.send)
		client.conn.Close()
		delete(h.clients, client)
	}
}

// broadcast sends data to all connected WebSocket clients.
func (h *WSHub) broadcast(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		select {
		case client.send <- data:
		default:
			// Client too slow — disconnect
			close(client.send)
			client.conn.Close()
			delete(h.clients, client)
		}
	}
}

// addClient registers a new WebSocket client.
func (h *WSHub) addClient(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	metrics.WebSocketConnections.Inc()
}

// removeClient unregisters a WebSocket client.
func (h *WSHub) removeClient(c *wsClient) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		close(c.send)
		delete(h.clients, c)
		metrics.WebSocketConnections.Dec()
	}
	h.mu.Unlock()
}

// handleWebSocket upgrades an HTTP connection to a WebSocket and streams events.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", "error", err)
		return
	}

	client := &wsClient{
		conn: conn,
		send: make(chan []byte, 256),
	}
	s.wsHub.addClient(client)

	// Writer goroutine
	go func() {
		defer func() {
			s.wsHub.removeClient(client)
			conn.Close()
		}()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case msg, ok := <-client.send:
				if !ok {
					conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Reader goroutine (handles pong and close)
	go func() {
		defer func() {
			s.wsHub.removeClient(client)
			conn.Close()
		}()

		conn.SetReadLimit(512)
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}
