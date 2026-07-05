package gobale

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Bot manages central state, workers, sessions, and route mappings
type Bot struct {
	*Client
	Sessions           SessionStore
	workerChan         chan *Update
	numWorkers         int
	workersWg          sync.WaitGroup
	bgSemaphore        chan struct{}
	shieldMode         uint32
	ctxPool            sync.Pool
	mu                 sync.RWMutex
	middlewares        []Handler
	anyMsg             []Handler
	cmds               map[string][]Handler
	texts              map[string][]Handler
	states             map[string][]Handler
	callbacks          map[string][]Handler
	preCheckouts       []Handler
	settings           []SettingEntry
	Blacklist          map[int64]bool
	Maintenance        bool
	MaintenanceAdminID int64
	MaintenanceText    string
	loggerInstance     *Logger
	totalUpdates       uint64
	i18n               map[string]map[string]string
	OnError            func(err error, c *Ctx)
	inviteCache        sync.Map
	dashServer         *http.Server
	tasks              []*ScheduledTask
	muTasks            sync.Mutex
	dbInstance         Storage
	settingsDB         Storage
	startHooks         []func()
	stopHooks          []func()
	safirKey           string
	safirBotID         int64
	defenseStop        chan struct{}
	defenseOnce        sync.Once
	editMsg            []Handler
	cache              *BotCache
	socketInstance     *SocketServer
	socketMu           sync.Mutex
	AutoStretch        bool
	groupSettings      []GroupSetting
	analyticsDB        Storage
	paginations        map[string]*PaginationBuilder
	pagMu              sync.RWMutex
	Bus                *EventBus
}

type BotBuilder struct {
	token      string
	workers    int
	gzip       bool
	bgTasks    int
	logger     *Logger
	proxy      string
	dryRun     bool
	adminID    int64
	safirKey   string
	safirBotID int64
}

// GroupSetting represents a chat-isolated boolean config template
type GroupSetting struct {
	Key     string
	Label   string
	Default bool
}

// DryRun configures the bot to run in sandbox mode without sending physical messages
func (b *BotBuilder) DryRun() *BotBuilder {
	b.dryRun = true
	return b
}

// Logger registers a custom Logger instance into the bot builder fluidly
func (b *BotBuilder) Logger(l *Logger) *BotBuilder {
	b.logger = l
	return b
}

// SetLogger replaces the active logger instance of the bot dynamically thread-safely
func (b *Bot) SetLogger(l *Logger) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if l != nil {
		b.loggerInstance = l
	}
}

// New initiates a fluent bot creation pipeline with adaptive CPU-scaled workers
func New(token string) *BotBuilder {
	defaultWorkers := runtime.NumCPU() * 2
	if defaultWorkers < 2 {
		defaultWorkers = 2
	}
	return &BotBuilder{
		token:   token,
		workers: defaultWorkers,
	}
}

// Workers registers concurrent queue processing worker count
func (b *BotBuilder) Workers(n int) *BotBuilder {
	if n > 0 {
		b.workers = n
	}
	return b
}

// rxUsername compiles a fast regex to validate Bale username format
var rxUsername = regexp.MustCompile(`^[a-zA-Z0-9_]{5,32}$`)

// MaxBgTasks sets maximum concurrent background tasks allowed
func (b *BotBuilder) MaxBgTasks(n int) *BotBuilder {
	b.bgTasks = n
	return b
}

// Gzip enables HTTP compression on Bale API requests
func (b *BotBuilder) Gzip() *BotBuilder {
	b.gzip = true
	return b
}

// Go completes the build chain, starts traffic monitor and returns Bot and an optional error safely
func (b *BotBuilder) Go() (*Bot, error) {
	if b.token == "" {
		return nil, fmt.Errorf("bale bot token is empty, please verify your .env file or configuration")
	}

	if b.workers <= 0 {
		b.workers = runtime.NumCPU() * 2
	}
	if b.workers < 2 {
		b.workers = 2
	}

	store := NewGOBStore(DataPath("gobale_sessions.gob"))
	_ = store.Load()

	bot := &Bot{
		Client:      NewClient(b.token),
		Sessions:    store,
		workerChan:  make(chan *Update, 1000),
		numWorkers:  b.workers,
		cmds:        make(map[string][]Handler),
		texts:       make(map[string][]Handler),
		states:      make(map[string][]Handler),
		callbacks:   make(map[string][]Handler),
		Blacklist:   make(map[int64]bool),
		dbInstance:  NewDatabase(DataPath("gobale_database.gob")),
		settingsDB:  NewDatabase(DataPath("gobale_settings.gob")),
		cache:       newBotCache(),
		safirKey:    b.safirKey,
		safirBotID:  b.safirBotID,
		paginations: make(map[string]*PaginationBuilder),
		Bus:         NewEventBus(),
	}

	bot.MaintenanceAdminID = b.adminID

	if b.logger != nil {
		bot.loggerInstance = b.logger
	} else {
		bot.loggerInstance = NewLogger(LevelInfo, "bot.log", true)
	}

	if b.bgTasks > 0 {
		bot.bgSemaphore = make(chan struct{}, b.bgTasks)
	} else {
		bot.bgSemaphore = make(chan struct{}, 100)
	}

	bot.Client.Gzip = b.gzip
	bot.Client.DryRun = b.dryRun

	// Initialize the central context pool with a clean constructor function
	bot.ctxPool.New = func() any { return &Ctx{} }

	bot.shieldMode = 0
	bot.totalUpdates = 0

	bot.OnError = func(err error, c *Ctx) {
		if bot.loggerInstance != nil {
			bot.loggerInstance.Log(LevelError, "[Runtime Error] ", "%v", []any{err})
		}
	}

	bot.On().Use(Recovery())

	// Unifies global, local, and remote settings processing with maximum security and owner bypass
	bot.On().Callback("_sys_cfg").Do(func(c *Ctx) {
		if c.Update == nil || c.Update.CallbackQuery == nil {
			return
		}

		var key string
		var targetChat string // Fixed: Declare as string so ScanCallbackArgs can parse it successfully
		_ = c.ScanCallbackArgs(&key, &targetChat)

		// 1. Identify setting scope (local vs global)
		isLocal := false
		c.Bot.mu.RLock()
		for _, s := range c.Bot.settings {
			if s.Key == key {
				isLocal = s.IsLocal
				break
			}
		}
		c.Bot.mu.RUnlock()

		// 2. Resolve target chat ID
		var resolved any
		if targetChat != "" { // Fixed: Check string empty
			resolved = c.Bot.ResolveChatID(targetChat)
		} else {
			id, _ := c.ChatID()
			resolved = c.Bot.ResolveChatID(id)
		}

		// 3. Security: Bypass immediately for Owner, verify group Admins for localized settings
		isOwner := c.IsOwner()
		if !isOwner {
			if !isLocal {
				// Non-owners cannot modify global settings (e.g. maintenance)
				_ = c.Answer().Text("❌ تغییر تنظیمات سراسری فقط مخصوص مدیریت کل ربات است!").Alert().Go()
				c.Abort()
				return
			}

			// Non-owners must be validated as group admins for local settings
			isAdmin, err := c.Bot.Chat(resolved).IsAdmin(c.SenderID()).Go()
			if err != nil || !isAdmin {
				_ = c.Answer().Text("❌ تغییر تنظیمات گروه فقط مخصوص مدیران است!").Alert().Go()
				c.Abort()
				return
			}
		}

		// 4. Toggle state in memory and GOB database natively
		errToggle := c.Settings(resolved).Toggle(key).Go()
		if errToggle != nil {
			return
		}

		// Edit the settings keyboard in-place dynamically
		_, _ = c.Edit().Settings(resolved).Go()
		_ = c.Answer().Go()
	})

	bot.optimizeForHardware()

	bot.defenseStop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		var last uint64
		for {
			select {
			case <-bot.defenseStop:
				return
			case <-ticker.C:
				current := atomic.LoadUint64(&bot.totalUpdates)
				diff := current - last
				last = current
				ups := float64(diff) / 10.0
				queue := len(bot.workerChan)
				if queue > 800 || ups > 150 {
					_ = bot.Shield().Activate().Go()
					if atomic.CompareAndSwapUint32(&bot.shieldMode, 0, 1) {
						log.Printf("[Auto Defense] Spike detected! UPS: %.2f, Queue: %d. Throttling active.", ups, queue)
					}
				} else if queue < 100 && ups < 10 {
					_ = bot.Shield().Deactivate().Go()
				}
			}
		}
	}()

	return bot, nil
}

// stopDefense closes the defenseStop channel safely exactly once to prevent goroutine leaks
func (b *Bot) stopDefense() {
	b.defenseOnce.Do(func() {
		if b.defenseStop != nil {
			close(b.defenseStop)
		}
	})
}

// optimizeForHardware configures Garbage Collector based on host processor specs
func (b *Bot) optimizeForHardware() {
	arch := runtime.GOARCH
	if arch == "arm" || arch == "arm64" {
		b.Mem().Limit(16).GCPercent(80).Go()
	} else {
		b.Mem().Limit(32).GCPercent(100).Go()
	}
}

// On accesses the central Dot System for registering event pipelines
func (b *Bot) On() *OnChain {
	if b == nil {
		log.Println("[GoBale Error] Attempted to call On() on a nil Bot pointer. Please verify that your bot was successfully initialized and the token is valid.")
		return &OnChain{}
	}
	return &OnChain{bot: b}
}

// Edit registers handlers for edited message events
func (o *OnChain) Edit() *RouteChain {
	return &RouteChain{
		on: o,
		reg: func(h ...Handler) {
			o.bot.mu.Lock()
			o.bot.editMsg = h
			o.bot.mu.Unlock()
		},
	}
}

// OnChain controls event registrations via closures
type OnChain struct {
	bot         *Bot
	middlewares []Handler
}

// Group creates a nested routing sub-group with its own private middlewares
func (o *OnChain) Group(middlewares ...Handler) *OnChain {
	combined := append(o.middlewares, middlewares...)
	return &OnChain{
		bot:         o.bot,
		middlewares: combined,
	}
}

// RouteChain wraps a fluent handler registration closure with guard and cooldown capabilities
type RouteChain struct {
	on            *OnChain
	reg           func(h ...Handler)
	guards        []func(*Ctx) bool
	cooldown      time.Duration
	cooldownAlert string
}

// Do attaches executable closure handler array, injects Group middlewares, Guards, and private Cooldowns
func (r *RouteChain) Do(h ...Handler) *OnChain {
	var finalHandlers []Handler

	// Inject isolated thread-safe cooldown middleware if configured for this specific command
	if r.cooldown > 0 {
		var cdUsers sync.Map
		cooldownMiddleware := func(c *Ctx) {
			if c.Message == nil || c.Message.From == nil {
				c.Next()
				return
			}
			userID := c.Message.From.ID
			now := time.Now()
			val, loaded := cdUsers.LoadOrStore(userID, now)
			if loaded {
				last := val.(time.Time)
				if now.Sub(last) < r.cooldown {
					rem := r.cooldown - now.Sub(last)
					_, _ = c.Send().Text(fmt.Sprintf(r.cooldownAlert, rem.Round(time.Second))).Go()
					c.Abort()
					return
				}
				cdUsers.Store(userID, now)
			}
			c.Next()
		}
		finalHandlers = append(finalHandlers, cooldownMiddleware)
	}

	// Inject guard validation middleware if registered
	if len(r.guards) > 0 {
		guardMiddleware := func(c *Ctx) {
			for _, guard := range r.guards {
				if !guard(c) {
					c.Abort()
					return
				}
			}
			c.Next()
		}
		finalHandlers = append(finalHandlers, guardMiddleware)
	}

	finalHandlers = append(finalHandlers, r.on.middlewares...)
	finalHandlers = append(finalHandlers, h...)
	r.reg(finalHandlers...)
	return r.on
}

// Use registers global middleware handler
func (o *OnChain) Use(h ...Handler) *OnChain {
	// Guard against nil OnChain or Bot pointers
	if o == nil || o.bot == nil {
		log.Println("[GoBale Error] Attempted to call Use() on a nil OnChain or Bot pointer.")
		return o
	}

	o.bot.mu.Lock()
	o.bot.middlewares = append(o.bot.middlewares, h...)
	o.bot.mu.Unlock()
	return o
}

// Msg registers generic handler for all text messages
func (o *OnChain) Msg() *RouteChain {
	return &RouteChain{
		on: o,
		reg: func(h ...Handler) {
			o.bot.mu.Lock()
			o.bot.anyMsg = h
			o.bot.mu.Unlock()
		},
	}
}

// Cmd registers command specific routes
func (o *OnChain) Cmd(cmd string) *RouteChain {
	return &RouteChain{
		on: o,
		reg: func(h ...Handler) {
			o.bot.mu.Lock()
			o.bot.cmds["/"+cmd] = h
			o.bot.mu.Unlock()
		},
	}
}

// Text registers exact matching raw text routes
func (o *OnChain) Text(text string) *RouteChain {
	return &RouteChain{
		on: o,
		reg: func(h ...Handler) {
			o.bot.mu.Lock()
			o.bot.texts[text] = h
			o.bot.mu.Unlock()
		},
	}
}

// State registers FSM matching state routes
func (o *OnChain) State(state string) *RouteChain {
	return &RouteChain{
		on: o,
		reg: func(h ...Handler) {
			o.bot.mu.Lock()
			o.bot.states[state] = h
			o.bot.mu.Unlock()
		},
	}
}

// Callback registers matching inline query callback routes
func (o *OnChain) Callback(data string) *RouteChain {
	return &RouteChain{
		on: o,
		reg: func(h ...Handler) {
			o.bot.mu.Lock()
			if strings.HasPrefix(data, "_sys_") {
				// Safely append system handlers instead of overwriting
				o.bot.callbacks[data] = append(o.bot.callbacks[data], h...)
			} else {
				o.bot.callbacks[data] = h
			}
			o.bot.mu.Unlock()
		},
	}
}

// PreCheckout registers payment pre checkout query handlers
func (o *OnChain) PreCheckout(h ...Handler) *OnChain {
	o.bot.mu.Lock()
	o.bot.preCheckouts = h
	o.bot.mu.Unlock()
	return o
}

// Run accesses the central runner Dot system chain
func (b *Bot) Run() *RunChain {
	if b == nil {
		log.Println("[GoBale Error] Attempted to call Run() on a nil Bot pointer.")
		return &RunChain{}
	}
	return &RunChain{bot: b}
}

// RunChain handles execution routing setups
type RunChain struct {
	bot *Bot
}

// Polling configures a long polling runner
func (r *RunChain) Polling() *PollChain {
	return &PollChain{run: r}
}

// Webhook configures a webhook server runner
func (r *RunChain) Webhook() *WebChain {
	return &WebChain{
		run:  r,
		addr: ":443",
		path: "/webhook",
	}
}

// PollChain manages long polling loop execution
type PollChain struct {
	run *RunChain
}

// Go spins up polling worker threads and starts fetching updates
func (p *PollChain) Go() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errClear := p.run.bot.BaseRequest(ctx, "deleteWebhook", nil, nil)
	if errClear != nil {
		log.Printf("[GoBale Webhook Clear Warn] %v", errClear)
	}

	p.run.bot.StartWorkers(ctx)
	offset := -1

	// Fire all registered OnStart lifecycle hooks concurrently with safe panic recovery
	p.run.bot.mu.RLock()
	for _, fn := range p.run.bot.startHooks {
		go func(f func()) {
			defer func() {
				if r := recover(); r != nil {
					handlePanic(p.run.bot, r, nil)
				}
			}()
			f()
		}(fn)
	}
	p.run.bot.mu.RUnlock()

	for {
		select {
		case <-ctx.Done():
			close(p.run.bot.workerChan)
			p.run.bot.workersWg.Wait()

			p.run.bot.stopDefense() // Clean up defense monitoring goroutine on exit

			// Fire all registered OnStop lifecycle hooks sequentially with safe panic recovery
			p.run.bot.mu.RLock()
			for _, fn := range p.run.bot.stopHooks {
				func(f func()) {
					defer func() {
						if r := recover(); r != nil {
							handlePanic(p.run.bot, r, nil)
						}
					}()
					f()
				}(fn)
			}
			p.run.bot.mu.RUnlock()

			// Clean and release the dashboard HTTP server port immediately with a timeout context
			if p.run.bot.dashServer != nil {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = p.run.bot.dashServer.Shutdown(shutdownCtx)
				cancel()
			}

			// Forcefully stop and release all background cron tasks
			p.run.bot.muTasks.Lock()
			for _, task := range p.run.bot.tasks {
				task.Stop()
			}
			p.run.bot.tasks = nil
			p.run.bot.muTasks.Unlock()

			// Safely close and flush the main GOB database on shutdown
			if p.run.bot.dbInstance != nil {
				_ = p.run.bot.dbInstance.Close()
			}

			// Safely close and flush the settings GOB database on shutdown
			if p.run.bot.settingsDB != nil {
				_ = p.run.bot.settingsDB.Close()
			}

			// Safely close and flush the analytics database on shutdown
			if p.run.bot.analyticsDB != nil {
				_ = p.run.bot.analyticsDB.Close()
			}

			// Safely close GOB store cleanup goroutine using io.Closer
			_ = p.run.bot.Sessions.Close()

			// Forcefully close and release all idle TCP socket connections to prevent socket leaks
			p.run.bot.Client.httpClient.CloseIdleConnections()
			return
		default:
			params := map[string]any{"offset": offset, "limit": 100, "timeout": 20}
			var updates []Update
			err := p.run.bot.BaseRequest(ctx, "getUpdates", params, &updates)
			if err != nil {
				log.Printf("[GoBale Polling Error] Failed to fetch updates: %v", err)
				// ctx-aware backoff so shutdown signal is not delayed by up to 3s
				select {
				case <-ctx.Done():
					continue
				case <-time.After(3 * time.Second):
					continue
				}
			}

			// Push updates to queue and advance offset to avoid fetching duplicates
			if len(updates) > 0 {
				for i := range updates {
					p.run.bot.workerChan <- &updates[i]
				}
				offset = updates[len(updates)-1].UpdateID + 1
			}
		}
	}
}

// WebChain configures webhook server structures with insecure SSL bypasses and auto ngrok tunnels
type WebChain struct {
	run      *RunChain
	addr     string
	path     string
	cert     string
	key      string
	url      string
	insecure bool
	useNgrok bool
	ngrokAPI string
}

// Addr registers custom webhook listening port address
func (w *WebChain) Addr(a string) *WebChain { w.addr = a; return w }

// Path registers custom webhook trigger url path
func (w *WebChain) Path(p string) *WebChain { w.path = p; return w }

// Cert registers SSL certificate file path
func (w *WebChain) Cert(c string) *WebChain { w.cert = c; return w }

// Key registers SSL private key file path
func (w *WebChain) Key(k string) *WebChain { w.key = k; return w }

// URL registers public webhook domain url for Bale servers
func (w *WebChain) URL(u string) *WebChain { w.url = u; return w }

// Insecure bypasses SSL cert requirements and boots webhook in plain HTTP (Recommended for ngrok)
func (w *WebChain) Insecure() *WebChain {
	w.insecure = true
	return w
}

// Ngrok instructs the webhook runner to automatically fetch the active public URL from a locally running ngrok agent
func (w *WebChain) Ngrok(apiURL ...string) *WebChain {
	w.useNgrok = true
	if len(apiURL) > 0 {
		w.ngrokAPI = apiURL[0]
	} else {
		w.ngrokAPI = "http://127.0.0.1:4040/api/tunnels"
	}
	return w
}

// Go registers webhook URLs and starts HTTP/HTTPS server with graceful resources cleanup
func (w *WebChain) Go() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	w.run.bot.StartWorkers(ctx)

	// Automatically resolve dynamic public URL from local ngrok agent if configured
	if w.useNgrok && w.url == "" {
		fetchedURL, err := fetchNgrokURL(w.ngrokAPI)
		if err != nil {
			if w.run.bot.loggerInstance != nil {
				w.run.bot.loggerInstance.Log(LevelError, "[Webhook Ngrok Error] ", "failed to auto-resolve ngrok URL: %v", []any{err})
			}
		} else {
			w.url = fetchedURL
			if w.run.bot.loggerInstance != nil {
				w.run.bot.loggerInstance.Log(LevelInfo, "[Webhook Ngrok Info] ", "successfully auto-resolved ngrok URL: %s", []any{w.url})
			}
		}
	}

	if w.url != "" {
		webhookURL := strings.TrimSuffix(w.url, "/") + w.path
		_ = w.run.bot.BaseRequest(ctx, "setWebhook", map[string]any{"url": webhookURL}, nil)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(w.path, func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var update Update
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		w.run.bot.workerChan <- &update
		rw.WriteHeader(http.StatusOK)
	})
	server := &http.Server{
		Addr:    w.addr,
		Handler: mux,
	}

	// Create an error channel to catch startup server errors
	errChan := make(chan error, 1)

	// Run the HTTP/HTTPS server in the background
	go func() {
		var err error
		if w.insecure || (w.cert == "" && w.key == "") {
			if w.run.bot.loggerInstance != nil {
				w.run.bot.loggerInstance.Log(LevelWarn, "[GoBale Webhook Warn] ", "Webhook is running in INSECURE (HTTP-only) mode. This is strictly recommended for local development or tunnel testing (ngrok) only! Do NOT use this in Production servers.", nil)
			}
			err = server.ListenAndServe()
		} else {
			err = server.ListenAndServeTLS(w.cert, w.key)
		}

		if err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
		close(errChan)
	}()

	// Block until system interrupt signal is caught or the server crashes
	select {
	case <-ctx.Done():
		// System interrupt caught, proceeding with graceful cleanup
	case err := <-errChan:
		// Server failed to start
		return err
	}

	// 1. Shutdown the HTTP Webhook server with a safety timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = server.Shutdown(shutdownCtx)
	cancel()

	// 2. Safely drain workers to process remaining messages
	close(w.run.bot.workerChan)
	w.run.bot.workersWg.Wait()

	w.run.bot.stopDefense()

	// 3. Clean and release the dashboard HTTP server port immediately with a timeout context
	if w.run.bot.dashServer != nil {
		dashShutdownCtx, dashCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = w.run.bot.dashServer.Shutdown(dashShutdownCtx)
		dashCancel()
	}

	// 4. Forcefully stop and release all background cron tasks on Webhook shutdown
	w.run.bot.muTasks.Lock()
	for _, task := range w.run.bot.tasks {
		task.Stop()
	}
	w.run.bot.tasks = nil
	w.run.bot.muTasks.Unlock()

	// 5. Safely close and flush the main GOB database on Webhook shutdown
	if w.run.bot.dbInstance != nil {
		_ = w.run.bot.dbInstance.Close()
	}

	// 6. Safely close and flush the settings GOB database on Webhook shutdown
	if w.run.bot.settingsDB != nil {
		_ = w.run.bot.settingsDB.Close()
	}

	// 7. Safely close and flush the analytics database on shutdown
	if w.run.bot.analyticsDB != nil {
		_ = w.run.bot.analyticsDB.Close()
	}

	// 8. Safely close GOB store cleanup goroutine using io.Closer on Webhook shutdown
	_ = w.run.bot.Sessions.Close()

	// 9. Forcefully close and release all idle TCP socket connections to prevent socket leaks on Webhook shutdown
	w.run.bot.Client.httpClient.CloseIdleConnections()

	return nil
}

// fetchNgrokURL queries local ngrok API to resolve the active public forwarding URL
func fetchNgrokURL(apiURL string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ngrok agent is offline or api is unreachable: %w", err)
	}
	defer resp.Body.Close()

	var data struct {
		Tunnels []struct {
			PublicURL string `json:"public_url"`
		} `json:"tunnels"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	if len(data.Tunnels) == 0 {
		return "", fmt.Errorf("no active tunnels found in ngrok agent")
	}

	return data.Tunnels[0].PublicURL, nil
}

// StartWorkers starts processing update worker channels
func (b *Bot) StartWorkers(ctx context.Context) {
	for i := 0; i < b.numWorkers; i++ {
		b.workersWg.Add(1)
		go func() {
			defer b.workersWg.Done()
			for update := range b.workerChan {
				b.processUpdate(ctx, update)
			}
		}()
	}
}

// processUpdate coordinates update routing internally with dynamic on-the-fly digit normalization
func (b *Bot) processUpdate(ctx context.Context, u *Update) {
	atomic.AddUint64(&b.totalUpdates, 1)

	c := b.ctxPool.Get().(*Ctx)
	c.Bot = b
	c.Update = u
	c.Message = u.Message
	c.index = -1
	c.ctx = ctx
	if u.CallbackQuery != nil {
		c.Message = u.CallbackQuery.Message
	}

	// Publish successful invoice payment events to the central EventBus asynchronously
	if u.Message != nil && u.Message.SuccessfulPayment != nil {
		b.Bus.Publish("payment.success", u.Message.SuccessfulPayment)
	}

	// Query state before acquiring the read lock to prevent lock contention
	var state string
	if u.Message != nil {
		stateChan := b.Sessions.Get(u.Message.Chat.ID).State()
		state, _ = stateChan.Go()
	}

	// Publish successful invoice payment events to the central EventBus asynchronously
	if u.Message != nil && u.Message.SuccessfulPayment != nil {
		b.Bus.Publish("payment.success", u.Message.SuccessfulPayment)
	}

	// Acquire read lock immediately to protect slice copies
	b.mu.RLock()

	var chain []Handler
	chain = append(chain, b.middlewares...)

	if u.Message != nil {
		// Dynamically normalize message text on-the-fly to keep original raw field untouched
		text := ToEnDigits(u.Message.Text)

		// Saving a new message text to cache
		if u.Message.Text != "" {
			key := fmt.Sprintf("msg:%d:%d", u.Message.Chat.ID, u.Message.MessageID)
			b.cache.mu.Lock()
			b.cache.store[key] = &cacheItem{
				value:     u.Message.Text,
				expiresAt: time.Now().Add(24 * time.Hour),
			}
			b.cache.mu.Unlock()
		}

		// Automatically capture Deep Link parameter inside GOB store session on startup
		if text != "" && strings.HasPrefix(text, "/start ") {
			parts := strings.Fields(text)
			if len(parts) > 1 {
				_, _ = b.Sessions.Get(u.Message.Chat.ID).Data("deep_link", parts[1]).Go()

				// Publish deep link referral start events to the central EventBus asynchronously
				b.Bus.Publish("bot.start", map[string]any{
					"ChatID":   u.Message.Chat.ID,
					"DeepLink": parts[1],
					"Sender":   u.Message.From,
				})
			}
		}

		// Route member joins and exits to their registered system callbacks seamlessly
		if len(u.Message.NewChatMembers) > 0 {
			if h, ok := b.callbacks["_sys_join"]; ok {
				chain = append(chain, h...)
			}
		} else if u.Message.LeftChatMember != nil {
			if h, ok := b.callbacks["_sys_exit"]; ok {
				chain = append(chain, h...)
			}
		} else if text != "" && text[0] == '/' {
			parts := strings.Fields(text)
			if len(parts) > 0 {
				if h, ok := b.cmds[parts[0]]; ok {
					chain = append(chain, h...)
				} else {
					chain = append(chain, b.anyMsg...)
				}
			} else {
				chain = append(chain, b.anyMsg...)
			}
		} else if state != "" {
			if h, ok := b.states[state]; ok {
				chain = append(chain, h...)
			} else if h, ok := b.texts[text]; ok {
				chain = append(chain, h...)
			} else {
				chain = append(chain, b.anyMsg...)
			}
		} else if h, ok := b.texts[text]; ok {
			chain = append(chain, h...)
		} else {
			chain = append(chain, b.anyMsg...)
		}
	} else if u.CallbackQuery != nil {
		// Dynamically normalize callback query data to keep original raw payload untouched
		data := ToEnDigits(u.CallbackQuery.Data)
		parts := strings.Split(data, ":")
		prefix := parts[0]
		if h, ok := b.callbacks[data]; ok {
			chain = append(chain, h...)
		} else if h, ok := b.callbacks[prefix]; ok {
			chain = append(chain, h...)
		}
		// Auto answer the callback query at the end of execution to stop the spinner
		chain = append(chain, func(c *Ctx) {
			c.Next()
			if c.Keys == nil || c.Keys["_sys_cb_answered"] == nil {
				_ = c.Bot.BaseRequest(c.ctx, "answerCallbackQuery", map[string]any{
					"callback_query_id": u.CallbackQuery.ID,
				}, nil)
			}
		})
	} else if u.PreCheckoutQuery != nil {
		chain = append(chain, b.preCheckouts...)
	} else if u.EditedMessage != nil {
		c.Message = u.EditedMessage

		// Get cached original text and update with new edited text <<<
		key := fmt.Sprintf("msg:%d:%d", u.EditedMessage.Chat.ID, u.EditedMessage.MessageID)

		b.cache.mu.RLock()
		item, ok := b.cache.store[key]
		b.cache.mu.RUnlock()

		if ok && item != nil {
			c.prevText, _ = item.value.(string)
		}

		b.cache.mu.Lock()
		b.cache.store[key] = &cacheItem{
			value:     u.EditedMessage.Text,
			expiresAt: time.Now().Add(24 * time.Hour),
		}
		b.cache.mu.Unlock()

		chain = append(chain, b.editMsg...)
	}

	b.mu.RUnlock() // Release read lock safely

	if len(chain) > 0 {
		c.handlers = chain
		c.Next()
	}

	// Recycle context cleanly
	c.Reset()
	b.ctxPool.Put(c)
}

// ResolveChatID normalizes different chat references into a standard API request target with cache lookup
func (b *Bot) ResolveChatID(target any) any {
	switch t := target.(type) {
	case int64, int, int32:
		return t
	case string:
		t = strings.TrimSpace(t)
		if t == "" {
			return t
		}
		if strings.HasPrefix(t, "-") || (t[0] >= '0' && t[0] <= '9') {
			var num int64
			_, _ = fmt.Sscanf(t, "%d", &num)
			if num != 0 {
				return num
			}
		}
		if strings.Contains(t, "join/") {
			cleanLink := strings.TrimPrefix(t, "http://")
			cleanLink = strings.TrimPrefix(cleanLink, "https://")
			if cached, ok := b.inviteCache.Load(cleanLink); ok {
				return cached.(int64)
			}
			return t
		}
		if strings.HasPrefix(t, "@") {
			cleanUser := strings.TrimPrefix(t, "@")
			if rxUsername.MatchString(cleanUser) {
				return t
			}
			return t
		}
		if strings.Contains(t, "ble.ir/") {
			parts := strings.Split(t, "/")
			username := parts[len(parts)-1]
			if username != "" {
				cleanUser := strings.TrimPrefix(username, "@")
				if rxUsername.MatchString(cleanUser) {
					return "@" + cleanUser
				}
			}
			return t
		}
		// Only prepend @ if the raw string strictly matches the valid username regex pattern
		if rxUsername.MatchString(t) {
			return "@" + t
		}
		return t
	}
	return target
}

// ErrorTo configures bot OnError handler to automatically forward stacktraces to admin chat
func (o *OnChain) ErrorTo(adminID any, botName string) *OnChain {
	// Guard against nil OnChain or Bot pointers
	if o == nil || o.bot == nil {
		log.Println("[GoBale Error] Attempted to call ErrorTo() on a nil OnChain or Bot pointer.")
		return o
	}
	resolved := o.bot.ResolveChatID(adminID)
	o.bot.OnError = func(err error, c *Ctx) {
		var userInfo string
		if c != nil && c.Message != nil && c.Message.From != nil {
			userInfo = fmt.Sprintf("👤 فرستنده: %s (%d)\n💬 متن پیام: %q",
				c.Message.From.FirstName,
				c.Message.From.ID,
				c.Message.Text,
			)
		} else {
			userInfo = "🤖 خطای سیستمی خارج از کانتکست کاربر"
		}
		report := fmt.Sprintf("🤖 *[%s] گزارش خطای رانتایم*\n\n❌ خطا:\n`%v`\n\n%s",
			botName,
			err,
			userInfo,
		)
		go func() {
			_ = o.bot.BaseRequest(context.Background(), "sendMessage", map[string]any{
				"chat_id":    resolved,
				"text":       report,
				"parse_mode": "Markdown",
			}, nil)
		}()
	}
	return o
}

// Push injects an incoming update directly into the worker queue
func (b *Bot) Push(u *Update) {
	b.workerChan <- u
}

// Guard appends a custom validation closure to protect this specific route
func (r *RouteChain) Guard(fn func(*Ctx) bool) *RouteChain {
	r.guards = append(r.guards, fn)
	return r
}

// Cooldown configures isolated thread-safe call delay intervals specifically for this command
func (r *RouteChain) Cooldown(d time.Duration, alert string) *RouteChain {
	r.cooldown = d
	r.cooldownAlert = alert
	return r
}

// logErr automatically logs non-nil errors into the central logger instance safely
func logErr(b *Bot, prefix string, err error) {
	if err != nil && b.loggerInstance != nil {
		b.loggerInstance.Log(LevelError, prefix, "%v", []any{err})
	}
}

// Start registers a callback to execute immediately when the bot boots successfully
func (o *OnChain) Start() *LifecycleChain {
	return &LifecycleChain{on: o, isStart: true}
}

// Stop registers a callback to execute cleanly during the bot's shutdown sequence
func (o *OnChain) Stop() *LifecycleChain {
	return &LifecycleChain{on: o, isStart: false}
}

// LifecycleChain manages dynamic lifecycle callbacks for bot boot and shutdown
type LifecycleChain struct {
	on      *OnChain
	isStart bool
}

// Do attaches the finalized executable closure to the lifecycle event
func (l *LifecycleChain) Do(fn func()) *OnChain {
	l.on.bot.mu.Lock()
	defer l.on.bot.mu.Unlock()
	if l.isStart {
		l.on.bot.startHooks = append(l.on.bot.startHooks, fn)
	} else {
		l.on.bot.stopHooks = append(l.on.bot.stopHooks, fn)
	}
	return l.on
}
func (b *BotBuilder) Admin(id int64) *BotBuilder {
	b.adminID = id
	return b
}

// MemoryStats represents a snapshot of Go runtime memory statistics
type MemoryStats struct {
	AllocMegabytes     float64
	SysMegabytes       float64
	HeapAllocMegabytes float64
	NumGC              uint32
	MemoryLimitBytes   int64
}

// GetMemoryStats reads and returns raw memory metrics from the Go runtime
func (b *Bot) GetMemoryStats() MemoryStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	limit := debug.SetMemoryLimit(-1)
	debug.SetMemoryLimit(limit)
	return MemoryStats{
		AllocMegabytes:     float64(m.Alloc) / (1024 * 1024),
		SysMegabytes:       float64(m.Sys) / (1024 * 1024),
		HeapAllocMegabytes: float64(m.HeapAlloc) / (1024 * 1024),
		NumGC:              m.NumGC,
		MemoryLimitBytes:   limit,
	}
}

// WebhookChain provides a fluent API for manually managing webhook registrations on Bale servers
type WebhookChain struct {
	bot    *Bot
	ctx    context.Context
	action string
	url    string
}

// Webhook opens the manual webhook configuration dot system from Bot context
func (b *Bot) Webhook() *WebhookChain {
	return &WebhookChain{
		bot: b,
		ctx: context.Background(),
	}
}

// Set registers a custom secure HTTPS url endpoint for receiving updates from Bale
func (w *WebhookChain) Set(url string) *WebhookChain {
	w.action = "set"
	w.url = url
	return w
}

// Del deletes the currently active webhook registration on Bale servers
func (w *WebhookChain) Del() *WebhookChain {
	w.action = "del"
	return w
}

// Go executes the webhook transaction on Bale servers with auto error logging
func (w *WebhookChain) Go() (bool, error) {
	var res bool
	var err error
	switch w.action {
	case "set":
		err = w.bot.BaseRequest(w.ctx, "setWebhook", map[string]any{"url": w.url}, &res)
	case "del":
		err = w.bot.BaseRequest(w.ctx, "deleteWebhook", nil, &res)
	default:
		return false, fmt.Errorf("empty webhook action configuration")
	}
	if err != nil {
		logErr(w.bot, "[Webhook Action Error] ", err)
	}
	return res, err
}

// UpdatesChain handles manual requests to retrieve recent updates from Bale servers fluidly
type UpdatesChain struct {
	bot    *Bot
	ctx    context.Context
	offset int
	limit  int
}

// Updates opens the fluent manual updates retrieval chain from Bot context
func (b *Bot) Updates() *UpdatesChain {
	return &UpdatesChain{
		bot:   b,
		ctx:   context.Background(),
		limit: 100,
	}
}

// Offset registers target starting update ID for retrieval
func (u *UpdatesChain) Offset(o int) *UpdatesChain {
	u.offset = o
	return u
}

// Limit registers maximum update capacity to retrieve per single call
func (u *UpdatesChain) Limit(l int) *UpdatesChain {
	u.limit = l
	return u
}

// Go executes the manual updates query on Bale servers with auto error logging
func (u *UpdatesChain) Go() ([]Update, error) {
	var updates []Update
	err := u.bot.BaseRequest(u.ctx, "getUpdates", map[string]any{
		"offset":  u.offset,
		"limit":   u.limit,
		"timeout": 20,
	}, &updates)
	if err != nil {
		logErr(u.bot, "[Get Updates Error] ", err)
	}
	return updates, err
}

// WebhookInfoChain handles fluent querying of active webhook metadata on Bale servers
type WebhookInfoChain struct {
	wc *WebhookChain
}

// Info initiates a webhook information query chain
func (w *WebhookChain) Info() *WebhookInfoChain {
	return &WebhookInfoChain{wc: w}
}

// Go executes the webhook info query on Bale servers and returns WebhookInfo
func (wi *WebhookInfoChain) Go() (*WebhookInfo, error) {
	var info WebhookInfo
	err := wi.wc.bot.BaseRequest(wi.wc.ctx, "getWebhookInfo", nil, &info)
	if err != nil {
		logErr(wi.wc.bot, "[Get Webhook Info Error] ", err)
	}
	return &info, err
}

// Safir registers global enterprise Safir API access key and bot ID fluidly
func (b *BotBuilder) Safir(apiKey string, botID int64) *BotBuilder {
	b.safirKey = apiKey
	b.safirBotID = botID
	return b
}

// Socket opens or retrieves the singleton socket.io server instance
func (b *Bot) Socket() *SocketServer {
	b.socketMu.Lock()
	defer b.socketMu.Unlock()
	if b.socketInstance == nil {
		b.socketInstance = &SocketServer{
			bot:  b,
			addr: ":8081",
		}
	}
	return b.socketInstance
}

// RegisterGroupSetting registers a chat-isolated configuration toggle
func (b *Bot) RegisterGroupSetting(key, label string, defaultVal bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.groupSettings = append(b.groupSettings, GroupSetting{
		Key:     key,
		Label:   label,
		Default: defaultVal,
	})
}

// ClearQueue drains the unexported buffered worker channel instantly and returns the count of deleted updates
func (b *Bot) ClearQueue() int {
	drained := 0
Draining:
	for len(b.workerChan) > 0 {
		select {
		case <-b.workerChan:
			drained++
		default:
			// Breaks out of the labeled "Draining" loop safely
			break Draining
		}
	}
	return drained
}

// cpuTracker manages process load variables safely
type cpuTracker struct {
	mu       sync.Mutex
	lastTick int64
	lastTime time.Time
}

// globalTracker holds state for ongoing cpu calculations
var globalTracker = &cpuTracker{
	lastTime: time.Now(),
}

// GetCPU reads and returns the current process CPU execution load on the host
func (b *Bot) GetCPU() float64 {
	globalTracker.mu.Lock()
	defer globalTracker.mu.Unlock()

	now := time.Now()
	elapsed := float64(now.Sub(globalTracker.lastTime).Microseconds())
	if elapsed <= 0 {
		return 0.0
	}

	ticks := getOSProcessCPUTicks()
	delta := float64(ticks - globalTracker.lastTick)
	globalTracker.lastTick = ticks
	globalTracker.lastTime = now

	raw := (delta / elapsed) * 100.0
	num := float64(runtime.NumCPU())
	percent := raw / num
	if percent < 0 {
		return 0.0
	}
	return percent
}

// Event registers a central listener callback to a specific topic on the unified EventBus
func (o *OnChain) Event(topic string, fn EventListener) *OnChain {
	o.bot.Bus.Subscribe(topic, fn)
	return o
}
