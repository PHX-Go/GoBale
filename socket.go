package gobale

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Standard WebSocket keep-alive constants
const (
	pingPeriod  = 15 * time.Second // Interval to send ping frames
	readTimeout = 20 * time.Second // Max time to wait for client pong/frame
)

// SocketClient manages individual upgraded raw WebSocket connections
type SocketClient struct {
	conn     net.Conn
	bufrw    *bufio.ReadWriter
	mu       sync.Mutex
	Handlers map[string]func(string)
	server   *SocketServer
	pingStop chan struct{} // Stop channel for background heartbeat loop
}

// On registers a custom socket event handler
func (sc *SocketClient) On(event string, h func(string)) {
	sc.Handlers[event] = h
}

// Send transmits a raw text message directly to the browser client with write deadlines
func (sc *SocketClient) Send(msg string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	_ = sc.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = writeFrame(sc.conn, []byte(msg))
}

// EmitJSON marshals Go variables to unified JSON response and transmits natively
func (sc *SocketClient) EmitJSON(event string, data any) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	response := map[string]any{
		"action":  event,
		"payload": data,
	}
	jsonBytes, err := json.Marshal(response)
	if err != nil {
		return
	}
	_ = sc.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = writeFrame(sc.conn, jsonBytes)
}

// Close closes the upgraded socket connection safely
func (sc *SocketClient) Close() {
	_ = sc.conn.Close()
}

// pingLoop writes periodic ping frames to keep the connection alive
func (sc *SocketClient) pingLoop() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sc.mu.Lock()
			_ = sc.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			// Send WebSocket Ping frame (FIN=1, Opcode=9, Length=0)
			_, err := sc.conn.Write([]byte{0x89, 0x00})
			sc.mu.Unlock()
			if err != nil {
				sc.Close()
				return
			}
		case <-sc.pingStop:
			return
		}
	}
}

// listen reads incoming upgraded frames with strict read deadlines
func (sc *SocketClient) listen() {
	defer func() {
		sc.server.clients.Delete(sc)
		close(sc.pingStop) // Terminate background heartbeat loop
		sc.Close()
	}()

	for {
		_ = sc.conn.SetReadDeadline(time.Now().Add(readTimeout))
		payload, opcode, err := readFrame(sc.bufrw.Reader)
		if err != nil {
			return
		}

		if opcode == 10 {
			// Received Pong from client, read deadline extended automatically
			continue
		}

		if opcode == 9 {
			// Received Ping from client, respond with Pong (FIN=1, Opcode=10, Length=0)
			sc.mu.Lock()
			_ = sc.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_, _ = sc.conn.Write([]byte{0x8A, 0x00})
			sc.mu.Unlock()
			continue
		}

		if opcode == 8 {
			return // Connection close frame received
		}

		if opcode == 1 {
			msg := string(payload)
			if sc.server.onMessage != nil {
				go sc.server.onMessage(sc, msg)
			}
		}
	}
}

// SocketServer handles upgrading HTTP requests to raw WebSockets
type SocketServer struct {
	bot       *Bot
	addr      string
	onConnect func(*SocketClient)
	onMessage func(*SocketClient, string)
	clients   sync.Map
	srv       *http.Server
	srvMu     sync.Mutex
}

// Addr registers custom websocket listening port address
func (ss *SocketServer) Addr(a string) *SocketServer {
	ss.addr = a
	return ss
}

// OnConnect registers a callback for new client connections
func (ss *SocketServer) OnConnect(h func(*SocketClient)) *SocketServer {
	ss.onConnect = h
	return ss
}

// OnMessage registers a callback for incoming messages from clients
func (ss *SocketServer) OnMessage(h func(*SocketClient, string)) *SocketServer {
	ss.onMessage = h
	return ss
}

// Broadcast sends a raw text message to all active clients
func (ss *SocketServer) Broadcast(msg string) {
	ss.clients.Range(func(key, value any) bool {
		if client, ok := key.(*SocketClient); ok {
			client.Send(msg)
		}
		return true
	})
}

// BroadcastJSON serializes any data to JSON and broadcasts to all clients
func (ss *SocketServer) BroadcastJSON(event string, data any) {
	ss.clients.Range(func(key, value any) bool {
		if client, ok := key.(*SocketClient); ok {
			client.EmitJSON(event, data)
		}
		return true
	})
}

// Go initializes the mux router, handles static files and runs server in background
func (ss *SocketServer) Go() {
	ss.srvMu.Lock()
	if ss.srv != nil {
		ss.srvMu.Unlock()
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		ss.ServeHTTP(w, r)
	})

	ss.srv = &http.Server{
		Addr:    ss.addr,
		Handler: mux,
	}
	ss.srvMu.Unlock()

	_ = ss.srv.ListenAndServe()
}

// ServeHTTP implements standard net/http handler interface with clean CORS and handshake execution
func (ss *SocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	secret := GetEnv[string]("SOCKET_TOKEN")
	if secret != "" && r.URL.Query().Get("token") != secret {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("401 Unauthorized: Invalid socket token"))
		return
	}

	conn, bufrw, err := handshake(w, r)
	if err != nil {
		return
	}

	client := &SocketClient{
		conn:     conn,
		bufrw:    bufrw,
		server:   ss,
		pingStop: make(chan struct{}),
	}

	ss.clients.Store(client, true)

	welcomePacket := map[string]any{
		"username": "سیستم مانیتورینگ",
		"userId":   123456789,
		"message":  "اتصال شما به وب‌سایت با موفقیت تایید شد.",
	}
	client.EmitJSON("user_status", welcomePacket)

	if ss.onConnect != nil {
		go ss.onConnect(client)
	}

	go client.pingLoop() // Start background keep-alive heartbeat loop
	client.listen()
}

// handshake hijacks HTTP connection and completes websocket RFC 6455 upgrade
func handshake(w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		w.WriteHeader(http.StatusBadRequest)
		return nil, nil, errors.New("missing sec-websocket-key header")
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return nil, nil, errors.New("hijack unsupported by the webserver")
	}

	conn, bufrw, err := hj.Hijack()
	if err != nil {
		return nil, nil, err
	}

	const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	h.Write([]byte(key + magic))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	handshakeStr := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"

	_, _ = conn.Write([]byte(handshakeStr))
	time.Sleep(10 * time.Millisecond)

	return conn, bufrw, nil
}

// readFrame reads and decodes masked websocket client frames
func readFrame(r *bufio.Reader) ([]byte, byte, error) {
	b1, err := r.ReadByte()
	if err != nil {
		return nil, 0, err
	}
	opcode := b1 & 0x0F

	b2, err := r.ReadByte()
	if err != nil {
		return nil, 0, err
	}
	hasMask := (b2 & 0x80) != 0
	length := int(b2 & 0x7F)

	switch length {
	case 126:
		lenBytes := make([]byte, 2)
		if _, err := io.ReadFull(r, lenBytes); err != nil {
			return nil, 0, err
		}
		length = int(binary.BigEndian.Uint16(lenBytes))
	case 127:
		lenBytes := make([]byte, 8)
		if _, err := io.ReadFull(r, lenBytes); err != nil {
			return nil, 0, err
		}
		length = int(binary.BigEndian.Uint64(lenBytes))
	}

	var maskKey []byte
	if hasMask {
		maskKey = make([]byte, 4)
		if _, err := io.ReadFull(r, maskKey); err != nil {
			return nil, 0, err
		}
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, 0, err
	}

	if hasMask {
		for i := 0; i < length; i++ {
			payload[i] ^= maskKey[i%4]
		}
	}

	return payload, opcode, nil
}

// writeFrame encodes and transmits unmasked websocket server-to-client frames
func writeFrame(w io.Writer, payload []byte) error {
	length := len(payload)
	var header []byte
	header = append(header, 0x81)

	if length <= 125 {
		header = append(header, byte(length))
	} else if length <= 65535 {
		header = append(header, 126)
		lenBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBytes, uint16(length))
		header = append(header, lenBytes...)
	} else {
		header = append(header, 127)
		lenBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(lenBytes, uint64(length))
		header = append(header, lenBytes...)
	}

	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}
