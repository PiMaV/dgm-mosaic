// Package hub serves the WETTER Viewer Contract (Socket.IO + HTTP .npy).
//
// Minimal Engine.IO v4 + Socket.IO protocol 5 server compatible with
// python-socketio 5 / flask-socketio (BLITZ, DONNER). No third-party Socket.IO lib.
package hub

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/PiMaV/dgm-mosaic/internal/npy"
	"github.com/gorilla/websocket"
)

const (
	defaultPingInterval = 25000
	defaultPingTimeout  = 20000
	defaultMaxPayload   = 1000000
	recordSep           = "\x1e"
)

// Publisher holds the latest cube and pushes it to connected viewers.
type Publisher struct {
	Host      string
	Port      int
	Token     string
	StackName string

	mu      sync.RWMutex
	stack   *npy.Array
	clients map[string]*client
	up      websocket.Upgrader
	onIndex func(index int)
}

type client struct {
	id   string
	conn *websocket.Conn
	mu   sync.Mutex
}

// New creates a publisher. Default StackName is "mosaic.npy".
func New(host string, port int, token, stackName string) *Publisher {
	if stackName == "" {
		stackName = "mosaic.npy"
	}
	return &Publisher{
		Host:      host,
		Port:      port,
		Token:     token,
		StackName: stackName,
		clients:   make(map[string]*client),
		up: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (p *Publisher) BaseURL() string {
	return fmt.Sprintf("http://%s:%d", p.Host, p.Port)
}

func (p *Publisher) ConnectHint() string {
	return fmt.Sprintf("%s  token %s", p.BaseURL(), p.Token)
}

// OnViewerIndex sets a callback when a viewer emits viewer_index.
func (p *Publisher) OnViewerIndex(fn func(int)) { p.onIndex = fn }

// SetStack replaces the served cube and optionally notifies viewers.
func (p *Publisher) SetStack(a npy.Array, push bool) {
	cp := a
	cp.Shape = append([]int(nil), a.Shape...)
	cp.Data = append([]byte(nil), a.Data...)
	p.mu.Lock()
	p.stack = &cp
	p.mu.Unlock()
	log.Printf("stack ready shape=%v dtype=%s (%.1f MB)", a.Shape, a.DType, float64(len(a.Data))/(1024*1024))
	if push {
		p.Push()
	}
}

// HasStack reports whether a cube is ready.
func (p *Publisher) HasStack() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stack != nil
}

// Push emits send_file_message to all clients.
func (p *Publisher) Push() {
	p.emitAll("send_file_message", map[string]any{"file_name": p.StackName}, "")
}

// PushIndex broadcasts a playhead seek (no cube bytes).
func (p *Publisher) PushIndex(index int, skipID string) {
	p.emitAll("send_file_message", map[string]any{
		"file_name": p.StackName,
		"index":     index,
	}, skipID)
}

// Handler returns an http.Handler mounting Socket.IO + token GET + CORS.
func (p *Publisher) Handler(ui http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/socket.io/", p.handleEngineIO)
	mux.HandleFunc("/"+p.Token, p.handleNPY)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      true,
			"token":   p.Token,
			"stack":   p.StackName,
			"ready":   p.HasStack(),
			"connect": p.ConnectHint(),
		})
	})
	if ui != nil {
		mux.Handle("/", ui)
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "DGM sidecar hub — connect BLITZ Stream to %s\n", p.ConnectHint())
		})
	}
	return mux
}

// ListenAndServe starts the HTTP server (blocking).
func (p *Publisher) ListenAndServe(ui http.Handler) error {
	addr := fmt.Sprintf("%s:%d", p.Host, p.Port)
	log.Printf("serving %s (token=%s)", p.BaseURL(), p.Token)
	return http.ListenAndServe(addr, p.Handler(ui))
}

func applyCORS(w http.ResponseWriter, r *http.Request) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	reqH := strings.TrimSpace(r.Header.Get("Access-Control-Request-Headers"))
	if reqH == "" {
		reqH = "Content-Type"
	}
	w.Header().Set("Access-Control-Allow-Headers", reqH)
	w.Header().Set("Access-Control-Allow-Private-Network", "true")
	w.Header().Set("Vary", "Origin, Access-Control-Request-Headers")
}

func (p *Publisher) handleNPY(w http.ResponseWriter, r *http.Request) {
	applyCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("filename") == "" {
		http.Error(w, "filename required", http.StatusBadRequest)
		return
	}
	p.mu.RLock()
	stack := p.stack
	p.mu.RUnlock()
	if stack == nil {
		http.NotFound(w, r)
		return
	}
	var buf bytes.Buffer
	if err := npy.Write(&buf, *stack); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	raw := buf.Bytes()
	name := r.URL.Query().Get("filename")
	if !strings.HasSuffix(name, ".npy") {
		name = p.StackName
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(raw)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(raw)
	log.Printf("serve %s raw=%.1f MB", name, float64(len(raw))/(1024*1024))
}

func (p *Publisher) handleEngineIO(w http.ResponseWriter, r *http.Request) {
	applyCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	q := r.URL.Query()
	if q.Get("EIO") != "4" {
		http.Error(w, "only EIO=4", http.StatusBadRequest)
		return
	}
	transport := q.Get("transport")
	sid := q.Get("sid")

	if transport == "websocket" {
		p.handleWS(w, r, sid)
		return
	}
	if transport != "polling" {
		http.Error(w, "unsupported transport", http.StatusBadRequest)
		return
	}

	if sid == "" {
		// Handshake
		newSID := newSID()
		open := map[string]any{
			"sid":          newSID,
			"upgrades":     []string{"websocket"},
			"pingInterval": defaultPingInterval,
			"pingTimeout":  defaultPingTimeout,
			"maxPayload":   defaultMaxPayload,
		}
		body, _ := json.Marshal(open)
		packet := "0" + string(body)
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		_, _ = io.WriteString(w, packet)
		return
	}

	if r.Method == http.MethodPost {
		// Client→server polling POST; drain and ACK
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		_, _ = io.WriteString(w, "ok")
		return
	}

	// Long-poll GET: hold briefly then noop (WS is preferred after handshake)
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	_, _ = io.WriteString(w, "6") // noop
}

func (p *Publisher) handleWS(w http.ResponseWriter, r *http.Request, sid string) {
	conn, err := p.up.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}
	if sid == "" {
		sid = newSID()
		open := map[string]any{
			"sid":          sid,
			"upgrades":     []string{},
			"pingInterval": defaultPingInterval,
			"pingTimeout":  defaultPingTimeout,
			"maxPayload":   defaultMaxPayload,
		}
		body, _ := json.Marshal(open)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("0"+string(body)))
	}

	c := &client{id: sid, conn: conn}
	p.mu.Lock()
	p.clients[sid] = c
	p.mu.Unlock()
	log.Printf("viewer client connected sid=%s", sid)
	defer func() {
		p.mu.Lock()
		delete(p.clients, sid)
		p.mu.Unlock()
		_ = conn.Close()
		log.Printf("viewer client disconnected sid=%s", sid)
	}()

	// Probe / messages
	for {
		_ = conn.SetReadDeadline(time.Now().Add(time.Duration(defaultPingInterval+defaultPingTimeout) * time.Millisecond))
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		msg := string(data)
		switch {
		case msg == "2": // ping
			_ = c.writeRaw("3")
		case msg == "2probe":
			_ = c.writeRaw("3probe")
		case msg == "5": // upgrade done
			// ignore
		case strings.HasPrefix(msg, "4"):
			p.handleSocketPacket(c, msg[1:])
		}
	}
}

func (p *Publisher) handleSocketPacket(c *client, payload string) {
	if payload == "" {
		return
	}
	typ := payload[0]
	rest := payload[1:]
	switch typ {
	case '0': // CONNECT
		// Reply with CONNECT ack including sid (Socket.IO v5)
		ack, _ := json.Marshal(map[string]string{"sid": c.id})
		_ = c.writeRaw("40" + string(ack))
		_ = c.writeEvent("Connected successfully", nil)
		if p.HasStack() {
			_ = c.writeEvent("send_file_message", map[string]any{"file_name": p.StackName})
		}
	case '2': // EVENT
		p.handleEvent(c, rest)
	}
}

func (p *Publisher) handleEvent(c *client, rest string) {
	// Format: ["event", data?] optionally prefixed with ack id digits
	for len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
		rest = rest[1:]
	}
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(rest), &arr); err != nil || len(arr) == 0 {
		return
	}
	var name string
	if err := json.Unmarshal(arr[0], &name); err != nil {
		return
	}
	if name != "viewer_index" || len(arr) < 2 {
		return
	}
	var data struct {
		Index int `json:"index"`
	}
	if err := json.Unmarshal(arr[1], &data); err != nil {
		return
	}
	if p.onIndex != nil {
		p.onIndex(data.Index)
	}
	p.PushIndex(data.Index, c.id)
}

func (p *Publisher) emitAll(event string, data any, skipID string) {
	p.mu.RLock()
	list := make([]*client, 0, len(p.clients))
	for id, c := range p.clients {
		if id == skipID {
			continue
		}
		list = append(list, c)
	}
	p.mu.RUnlock()
	for _, c := range list {
		_ = c.writeEvent(event, data)
	}
}

func (c *client) writeRaw(s string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, []byte(s))
}

func (c *client) writeEvent(name string, data any) error {
	var payload []any
	if data == nil {
		payload = []any{name}
	} else {
		payload = []any{name, data}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.writeRaw("42" + string(body))
}

func newSID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// DummyStack returns a small uint16 cube for hub handshake tests.
func DummyStack() npy.Array {
	const h, w = 64, 64
	data := make([]uint16, h*w)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			data[y*w+x] = uint16(1000 + (x+y)%200)
		}
	}
	return npy.FromUint16LE([]int{h, w}, data)
}
