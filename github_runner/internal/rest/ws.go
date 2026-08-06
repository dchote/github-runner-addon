package rest

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: checkWSOrigin,
}

// checkWSOrigin allows same-host Origin, HA ingress (X-Ingress-Path / X-Forwarded-Host),
// and non-browser clients with no Origin.
func checkWSOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if hostsEqual(u.Host, r.Host) {
		return true
	}
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		// May be a comma-separated list; first hop is the client-facing host.
		host := strings.TrimSpace(strings.Split(fwd, ",")[0])
		if hostsEqual(u.Host, host) {
			return true
		}
	}
	// Home Assistant ingress: browser Origin is the HA UI; request Host is internal.
	if strings.TrimSpace(r.Header.Get("X-Ingress-Path")) != "" {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Hass-Source"), "core.ingress") {
		return true
	}
	return false
}

func hostsEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

type wsClientMsg struct {
	Type     string `json:"type"`
	Channel  string `json:"channel"`
	RunnerID string `json:"runner_id"`
	Tail     string `json:"tail"`
}

type wsServerMsg struct {
	Type     string `json:"type"`
	Channel  string `json:"channel"`
	RunnerID string `json:"runner_id,omitempty"`
	Line     string `json:"line,omitempty"`
	Error    string `json:"error,omitempty"`
}

type wsConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *wsConn) writeJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(v)
}

func (c *wsConn) ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
}

type wsSession struct {
	mu           sync.Mutex
	streamCancel context.CancelFunc
}

func (s *wsSession) replaceStream(cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streamCancel != nil {
		s.streamCancel()
	}
	s.streamCancel = cancel
}

func (s *wsSession) stopStream() {
	s.replaceStream(nil)
}

func (h *Handlers) WebSocket(w http.ResponseWriter, r *http.Request) {
	raw, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Debug("websocket upgrade failed", "err", err, "host", r.Host, "origin", r.Header.Get("Origin"))
		return
	}
	defer raw.Close()

	conn := &wsConn{conn: raw}
	_ = raw.SetReadDeadline(time.Now().Add(60 * time.Second))
	raw.SetPongHandler(func(string) error {
		_ = raw.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	connCtx, connCancel := context.WithCancel(r.Context())
	defer connCancel()

	session := &wsSession{}
	defer session.stopStream()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-connCtx.Done():
				return
			case <-ticker.C:
				if err := conn.ping(); err != nil {
					connCancel()
					return
				}
			}
		}
	}()

	raw.SetReadLimit(1 << 20) // 1 MiB

	for {
		_, data, err := raw.ReadMessage()
		if err != nil {
			connCancel()
			return
		}
		var msg wsClientMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "subscribe":
			if msg.Channel != "container_logs" {
				continue
			}
			id := msg.RunnerID
			if id == "" {
				_ = conn.writeJSON(wsServerMsg{Type: "error", Channel: "container_logs", Error: "runner_id required"})
				continue
			}
			tail := msg.Tail
			if tail == "" {
				tail = "200"
			}
			streamCtx, streamCancel := context.WithCancel(connCtx)
			session.replaceStream(streamCancel)
			go h.streamLogsWS(streamCtx, conn, id, tail)
		case "unsubscribe":
			if msg.Channel == "" || msg.Channel == "container_logs" {
				session.stopStream()
			}
		}
	}
}

func (h *Handlers) streamLogsWS(ctx context.Context, conn *wsConn, runnerID, tail string) {
	rc, err := h.Manager.Logs(ctx, runnerID, true, tail)
	if err != nil {
		_ = conn.writeJSON(wsServerMsg{Type: "error", Channel: "container_logs", RunnerID: runnerID, Error: err.Error()})
		return
	}
	defer rc.Close()

	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := conn.writeJSON(wsServerMsg{
			Type:     "log_line",
			Channel:  "container_logs",
			RunnerID: runnerID,
			Line:     scanner.Text(),
		}); err != nil {
			slog.Debug("ws write failed", "err", err)
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		slog.Debug("ws log scan ended", "err", err, "runner_id", runnerID)
	}
}
