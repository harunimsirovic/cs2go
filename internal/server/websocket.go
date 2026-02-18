package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins in development. Tighten this in production.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WebSocketHub tracks all active WebSocket connections keyed by job ID.
// Multiple clients can subscribe to the same job (e.g., two browser tabs).
type WebSocketHub struct {
	mu      sync.RWMutex
	clients map[string][]*websocket.Conn
}

func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		clients: make(map[string][]*websocket.Conn),
	}
}

// Broadcast sends a JSON message to all clients subscribed to jobID.
// Dead connections are silently removed.
func (h *WebSocketHub) Broadcast(jobID string, msg any) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.clients[jobID]
	alive := conns[:0] // re-use the slice to avoid allocation

	for _, conn := range conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[ws] dropping dead connection for job %s: %v", jobID, err)
			conn.Close()
		} else {
			alive = append(alive, conn)
		}
	}

	if len(alive) == 0 {
		delete(h.clients, jobID)
	} else {
		h.clients[jobID] = alive
	}
}

// HandleWebSocket upgrades an HTTP connection to WebSocket and subscribes
// it to a job's progress stream.
//
// URL: GET /ws?job_id=<uuid>
func (h *WebSocketHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, "job_id query param required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws] upgrade error: %v", err)
		return
	}

	h.mu.Lock()
	h.clients[jobID] = append(h.clients[jobID], conn)
	h.mu.Unlock()

	log.Printf("[ws] client connected for job %s", jobID)

	// Keep the connection open by reading (and discarding) client messages.
	// When the client disconnects, ReadMessage returns an error and we exit.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			log.Printf("[ws] client disconnected for job %s", jobID)
			h.removeConn(jobID, conn)
			break
		}
	}
}

func (h *WebSocketHub) removeConn(jobID string, target *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.clients[jobID]
	filtered := conns[:0]
	for _, c := range conns {
		if c != target {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) == 0 {
		delete(h.clients, jobID)
	} else {
		h.clients[jobID] = filtered
	}
}
