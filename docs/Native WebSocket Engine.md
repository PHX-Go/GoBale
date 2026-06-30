# GoBale Native WebSocket Engine (RFC 6455)

The **GoBale Native WebSocket Engine** is a lightweight, zero-dependency, and high-performance real-time communication layer built directly on top of Go's standard library (`net/http` connection hijacking). 

It allows you to bridge your Bale Messenger Bot events to any modern web frontend (or custom administration panel) in real-time, enabling bidirectional message streaming and remote command execution.

---

## Key Features

- **Zero External Dependencies:** Built entirely on standard Go library packages (`net`, `net/http`, `crypto/sha1`, `encoding/base64`, `bufio`).
- **RFC 6455 Compliant:** Natively handles the WebSocket handshake protocol, unmasking client-to-server frames, and encoding server-to-client frames.
- **Native Browser Keep-Alive:** Automatically handles standard browser Ping control frames (`opcode 9`) by replying with Pong frames (`0x8A`) to keep connections alive indefinitely.
- **Event-Simulation Layer:** Simulates Socket.io-like Event emitters (`EmitJSON` and `BroadcastJSON`) natively over raw WebSockets using structured JSON packets.
- **Security Handshake (Optional):** Supports token validation out-of-the-box via query parameters (e.g. `?token=your_key`) to secure your administration console.

---

## Server-Side API Reference

### `SocketServer` Methods

#### `bot.Socket()`
Retrieves the central singleton socket server instance connected to the active Bot.

#### `Addr(addr string) *SocketServer`
Sets the listening port for the WebSocket server (defaults to `:8081`).

#### `OnConnect(func(*SocketClient)) *SocketServer`
Registers a callback function executed whenever a new browser client connects successfully.

#### `OnMessage(func(*SocketClient, string)) *SocketServer`
Registers a callback executed when a client transmits a raw text/JSON frame to the server.

#### `Broadcast(msg string)`
Transmits a raw text message to all currently connected clients.

#### `BroadcastJSON(event string, data any)`
Serializes any Go struct, map, or slice to JSON and broadcasts it to all clients as an event packet:
```json
{
  "action": "event_name",
  "payload": data
}
```

#### `Go()`
Launches the HTTP/WebSocket server in a background goroutine.

---

### `SocketClient` Methods

#### `Send(msg string)`
Transmits a raw text message directly to this specific client.

#### `EmitJSON(event string, data any)`
Serializes a Go data structure and transmits it to this client as an event-styled JSON packet.

#### `Close()`
Closes the underlying raw TCP connection safely.

---

## Getting Started

### 1. Configure Environment (`.env`)
Store your bot credentials and port settings in your `.env` file:

```env
BALE_TOKEN="YOUR_BALE_TOKEN"
ADMIN_ID=YOUR_ADMIN_NUMERIC_ID

# --- Socket Settings ---
SOCKET_PORT=":8081"
SOCKET_TOKEN="your_secure_token_here"
```

---

### 2. Backend Implementation (`main.go`)

Initialize the GoBale bot, register the socket server, listen to connection events, and stream incoming Bale messages to the web console:

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	gobale "github.com/PHX-Go/GoBale"
)

type WebAction struct {
	Action  string `json:"action"`
	Payload string `json:"payload"`
}

type HardwareMetrics struct {
	CPU string `json:"cpu"`
	RAM string `json:"ram"`
}

func main() {
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	bot, err := gobale.New(token).Admin(adminID).Gzip().Go()
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	server := bot.Socket()

	server.OnConnect(func(client *gobale.SocketClient) {
		log.Println("Admin dashboard connected via WebSockets.")
	})

	// handle incoming commands from the web console
	server.OnMessage(func(client *gobale.SocketClient, msg string) {
		var action WebAction
		if err := json.Unmarshal([]byte(msg), &action); err != nil {
			return
		}

		switch action.Action {
		case "get_status":
			stats := bot.GetMemoryStats()
			cpu := bot.GetCPU()
			metrics := HardwareMetrics{
				CPU: fmt.Sprintf("%.2f", cpu),
				RAM: fmt.Sprintf("%.2f", stats.AllocMegabytes),
			}
			client.EmitJSON("status_response", metrics)

		case "toggle_maintenance":
			bot.Maintenance = !bot.Maintenance
			statusStr := fmt.Sprintf("%t", bot.Maintenance)
			server.BroadcastJSON("maintenance_response", statusStr)
		}
	})

	// stream bale messenger chats to the web panel in real-time
	bot.On().Msg().Do(func(c *gobale.Ctx) {
		updateText := fmt.Sprintf("[%s]: %s", c.Message.From.FirstName, c.RawText())
		server.BroadcastJSON("alert_notify", updateText)
		c.Next()
	})

	// load port and start the socket server
	socketPort := gobale.GetEnv[string]("SOCKET_PORT")
	bot.Socket().Addr(socketPort).Go()

	log.Println("Bot and Socket servers are running...")
	bot.Run().Polling().Go()
}
```

---

### 3. Frontend Web Client (`index.html`)

Create an `index.html` file in the same directory as `main.go`. It connects natively without needing any third-party JS libraries:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>GoBale Admin Console</title>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/tailwindcss/2.2.19/tailwind.min.js"></script>
</head>
<body class="bg-gray-950 text-gray-100 min-h-screen p-6">

    <div class="max-w-4xl mx-auto">
        <header class="flex justify-between border-b border-gray-800 pb-4 mb-6">
            <h1 class="text-xl font-black text-emerald-400">GoBale Admin Web Console</h1>
            <div id="status" class="text-xs bg-red-950 text-red-400 px-3 py-1 rounded">Disconnected</div>
        </header>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
            <div class="bg-gray-900 p-4 rounded-xl border border-gray-800">
                <span class="text-xs text-gray-400">Server Metrics</span>
                <p id="metrics" class="text-sm text-gray-500 mt-2">No data yet...</p>
                <button onclick="sendAction('get_status')" class="w-full mt-3 bg-gray-800 text-xs py-2 rounded-lg">Update Metrics</button>
            </div>
            <div class="bg-gray-900 p-4 rounded-xl border border-gray-800">
                <span class="text-xs text-gray-400">Maintenance Mode</span>
                <p id="maintenance" class="text-sm text-gray-500 mt-2">Unknown</p>
                <button onclick="sendAction('toggle_maintenance')" class="w-full mt-3 bg-amber-600 text-xs py-2 rounded-lg">Toggle Mode</button>
            </div>
        </div>

        <div class="bg-gray-900 p-4 rounded-xl border border-gray-800">
            <span class="text-xs text-gray-400">Live Console Feed</span>
            <div id="logs" class="mt-3 h-44 overflow-y-auto space-y-1 text-xs font-mono"></div>
        </div>
    </div>

    <script>
        const logs = document.getElementById("logs");
        const status = document.getElementById("status");

        function log(msg, color = "gray") {
            logs.innerHTML += `<p style="color: ${color}; border-bottom: 1px solid #1f2937; padding-bottom: 4px;">${msg}</p>`;
            logs.scrollTop = logs.scrollHeight;
        }

        const urlParams = new URLSearchParams(window.location.search);
        const token = urlParams.get('token') || '';

        if (window.location.protocol === "file:") {
            log("🔴 Security Error: Do not open index.html as local file://, access http://localhost:8081 directly.", "red");
        } else if (!token) {
            log("🔴 Access Refused: Please append ?token=your_token to the URL bar.", "red");
        } else {
            log("⏳ Establishing WebSocket secure pipeline...");
            
            // connect natively over standard browser WebSocket protocol
            const ws = new WebSocket("ws://" + window.location.host + "/ws?token=" + token);

            ws.onopen = () => {
                status.className = "text-xs bg-emerald-950 text-emerald-400 px-3 py-1 rounded";
                status.innerText = "Connected";
                log("🟢 Successfully connected to GoBale native WebSocket server.", "green");
            };

            ws.onmessage = (e) => {
                const response = JSON.parse(e.data);
                switch (response.action) {
                    case "user_status":
                        log("👤 System: " + response.payload.message, "green");
                        break;
                    case "status_response":
                        document.getElementById("metrics").innerHTML = `RAM: ${response.payload.ram} MB | CPU: ${response.payload.cpu}%`;
                        log(`📊 Metrics: CPU: ${response.payload.cpu}%, RAM: ${response.payload.ram}MB`, "gray");
                        break;
                    case "maintenance_response":
                        const active = response.payload === "true";
                        document.getElementById("maintenance").innerText = active ? "Enabled" : "Disabled";
                        log("🛡️ Maintenance mode state toggled successfully.", "amber");
                        break;
                    case "alert_notify":
                        log("💬 Bale update: " + response.payload, "purple");
                        break;
                }
            };

            ws.onclose = () => {
                status.className = "text-xs bg-red-950 text-red-400 px-3 py-1 rounded";
                status.innerText = "Disconnected";
                log("🔴 Connection with server closed.", "red");
            };

            window.sendAction = function(action) {
                ws.send(JSON.stringify({ action: action, payload: "" }));
            };
        }
    </script>
</body>
</html>
```
