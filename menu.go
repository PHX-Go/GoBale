package gobale

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// MenuButton represents a single action inside the dynamic menu builder
type MenuButton struct {
	Text     string
	TargetID string
	Handler  Handler
}

// MenuNode represents a single menu page containing layout and buttons
type MenuNode struct {
	ID             string
	TextVal        string
	ParentID       string
	IsInline       bool
	Rows           [][]MenuButton
	backLabel      string
	stretch        bool
	closeLabel     string
	CachedKeyboard any
	CachedText     string
	// Dynamic key generator based on chat identifier
	DynamicKey func(chatID int64) string
	// Dynamic keyboard builder function
	DynamicKeyboard func(chatID int64) any
	// Dynamic text builder function
	DynamicText func(chatID int64) string
	// Custom TTL duration for dynamic menu entries
	TTL time.Duration
}

// MenuBtn initiates a menu button fluent configuration
func MenuBtn(text string) MenuButton {
	return MenuButton{Text: text}
}

// Target configures the button to transition to another menu ID
func (b MenuButton) Target(menuID string) MenuButton {
	b.TargetID = menuID
	return b
}

// Do attaches a custom execution handler to this button click
func (b MenuButton) Do(h Handler) MenuButton {
	b.Handler = h
	return b
}

// MenuChain manages the fluent building of menu nodes
type MenuChain struct {
	bot  *Bot
	node *MenuNode
}

// Menu opens the dynamic nested menu configuration chain
func (b *Bot) Menu(id string) *MenuChain {
	node := &MenuNode{
		ID:        id,
		IsInline:  true,
		backLabel: "🔙 بازگشت",
		stretch:   true,
	}
	b.mu.Lock()
	if b.menus == nil {
		b.menus = make(map[string]*MenuNode)
	}
	b.menus[id] = node
	b.mu.Unlock()

	return &MenuChain{bot: b, node: node}
}

// Text sets the description text shown above the menu keyboard
func (m *MenuChain) Text(t string) *MenuChain {
	m.node.TextVal = t
	return m
}

// Parent registers a parent menu ID to auto-generate back button navigation
func (m *MenuChain) Parent(parentID string) *MenuChain {
	m.node.ParentID = parentID
	return m
}

// BackLabel overrides the default back button label
func (m *MenuChain) BackLabel(label string) *MenuChain {
	m.node.backLabel = label
	return m
}

// CloseLabel registers a custom close button to collapse and delete the menu
func (m *MenuChain) CloseLabel(label string) *MenuChain {
	m.node.closeLabel = label
	return m
}

// Stretch enables or disables framework text stretching for this menu
func (m *MenuChain) Stretch(v bool) *MenuChain {
	m.node.stretch = v
	return m
}

// Reply configures the menu to render as a Reply Keyboard instead of Inline
func (m *MenuChain) Reply() *MenuChain {
	m.node.IsInline = false
	return m
}

// Inline configures the menu to render as an Inline Glass Keyboard
func (m *MenuChain) Inline() *MenuChain {
	m.node.IsInline = true
	return m
}

// Row appends a single row of buttons to this menu node
func (m *MenuChain) Row(buttons ...MenuButton) *MenuChain {
	m.node.Rows = append(m.node.Rows, buttons)
	return m
}

// normalizeMenuWidth wraps normalizeWidth using standard spaces to prevent formatting corruption
func normalizeMenuWidth(s string, width int) string {
	r := []rune(s)
	if len(r) > width {
		if width <= 1 {
			return string(r[:width])
		}
		return string(r[:width-1]) + "…"
	}

	diff := width - len(r)
	leftPad := diff / 2
	rightPad := diff - leftPad

	// Render using standard compatible spaces to ensure zero character corruption
	return strings.Repeat(" ", leftPad) + s + strings.Repeat(" ", rightPad)
}

// CompileMenus automatically registers stateful middleware, callback routers, and caches all menu resources
func (b *Bot) CompileMenus() {
	b.menusOnce.Do(func() {
		// Encapsulate caching under a safe, local lock scope with defer to prevent deadlocks on panics
		func() {
			b.mu.Lock()
			defer b.mu.Unlock()

			b.replyButtons = make(map[string]bool)
			for _, node := range b.menus {
				if node == nil {
					continue
				}
				if !node.IsInline {
					for _, row := range node.Rows {
						for _, btn := range row {
							b.replyButtons[strings.TrimSpace(btn.Text)] = true
						}
					}
					if node.ParentID != "" {
						b.replyButtons[strings.TrimSpace(node.backLabel)] = true
					}
					if node.closeLabel != "" {
						b.replyButtons[strings.TrimSpace(node.closeLabel)] = true
					}
				}
			}

			// Precompute and Cache both Keyboard and stretched Text for every menu node (Zero Transition Latency!)
			for _, node := range b.menus {
				if node == nil {
					continue
				}
				node.CachedKeyboard = b.BuildMenuKeyboard(node)
				node.CachedText = node.TextVal
				if node.stretch {
					node.CachedText = stretchText(node.TextVal)
				}
			}
		}()

		// 1. Stateful global middleware to intercept Reply Keyboards without route collisions
		b.On().Use(func(c *Ctx) {
			if c.Message != nil && c.Message.Text != "" {
				text := strings.Trim(c.Message.Text, " \t\n\r\u2800\u00a0")

				// Retrieve user's current menu ID from their active Session
				raw, err := c.Session().Data("_current_menu").Go()
				if err == nil && raw != nil {
					if menuID, ok := raw.(string); ok && menuID != "" {
						b.mu.RLock()
						node, ok := b.menus[menuID]
						b.mu.RUnlock()

						if ok && node != nil && !node.IsInline {
							// Dynamically match and handle Close Menu event
							if node.closeLabel != "" && text == strings.TrimSpace(node.closeLabel) {
								c.Abort()
								_ = c.Del().Go()
								// Delete all previous menu messages asynchronously
								var msgIDs []int64
								if rawIDs, errIDs := c.Session().Data("_menu_msg_ids").Go(); errIDs == nil && rawIDs != nil {
									if ids, okSlice := rawIDs.([]int64); okSlice {
										msgIDs = ids
									}
								}
								if len(msgIDs) > 0 {
									botInstance := c.Bot
									ctx := c.ctx
									chatID, _ := c.ChatID()
									for _, id := range msgIDs {
										if id > 0 {
											c.Go(func() {
												_ = botInstance.BaseRequest(ctx, "deleteMessage", map[string]any{
													"chat_id":    chatID,
													"message_id": id,
												}, nil)
											})
										}
									}
								}

								// Collapse user's keyboard and send a self-destroying message
								_, _ = c.Send().Text("🔒 منو با موفقیت بسته شد.").MarkupRemove().Temp(3 * time.Second).Go()

								// Reset session states
								_, _ = c.Session().Data("_current_menu", "").Go()
								_, _ = c.Session().Data("_menu_msg_ids", []int64{}).Go()
								_, _ = c.Session().Data("_menu_msg_id", 0).Go()
								_, _ = c.Session().Data("_menu_history", []string{}).Go()
								return
							}

							// Dynamically match the customized back button label
							if node.ParentID != "" && text == strings.TrimSpace(node.backLabel) {
								c.Abort()
								_ = c.SendMenu("back")
								return
							}

							// Match and route standard Reply Keyboard menu buttons
							for _, row := range node.Rows {
								for _, btn := range row {
									if text == strings.TrimSpace(btn.Text) {
										c.Abort()
										if btn.Handler != nil {
											btn.Handler(c)
										}
										if btn.TargetID != "" {
											_ = c.SendMenu(btn.TargetID)
										}
										return
									}
								}
							}
						}
					}
				}
			}
			c.Next()
		})

		// 2. Register inline callback transition router with dynamic back-stack support
		b.On().Callback("_menu").Do(func(c *Ctx) {
			var targetID string
			_ = c.ScanCallbackArgs(&targetID)

			// Answer callback immediately to stop the button loading spinner instantly (Smooth UI)
			_ = c.Answer().Go()

			// Handle Dynamic Close Menu Event using standard context delete (100% bug-free)
			if targetID == "close" {
				_ = c.Del().Go()

				// Reset session states
				_, _ = c.Session().Data("_current_menu", "").Go()
				_, _ = c.Session().Data("_menu_msg_ids", []int64{}).Go()
				_, _ = c.Session().Data("_menu_msg_id", 0).Go()
				_, _ = c.Session().Data("_menu_history", []string{}).Go()
				return
			}

			// Standard forward menu transition
			_ = c.SendMenu(targetID)
		})

		// 3. Register inline custom action callback router
		b.On().Callback("_menu_act").Do(func(c *Ctx) {
			var menuID string
			var r, col int
			_ = c.ScanCallbackArgs(&menuID, &r, &col)

			b.mu.RLock()
			node, ok := b.menus[menuID]
			b.mu.RUnlock()
			if !ok {
				return
			}

			_ = c.Answer().Go()
			if r < len(node.Rows) && col < len(node.Rows[r]) {
				btn := node.Rows[r][col]
				if btn.Handler != nil {
					btn.Handler(c)
				}
			}
		})
		b.menusAtomic.Store(b.menus)
		b.replyButtonsAtomic.Store(b.replyButtons)
	})
}

// BuildMenuKeyboard creates the correct markup interface for inline or reply menu nodes with symmetrical centering
func (b *Bot) BuildMenuKeyboard(node *MenuNode) any {
	// Calculate max button label length for perfect ASCII centering across the entire keyboard
	maxLen := 0
	if node.stretch {
		maxLen = 40 // Enforce baseline minimum width of 40 to strictly align with the stretched message bubble
		for _, row := range node.Rows {
			for _, btn := range row {
				if l := len([]rune(btn.Text)); l > maxLen {
					maxLen = l
				}
			}
		}
		// Include the dynamic back label to prevent truncation
		if node.ParentID != "" {
			if l := len([]rune(node.backLabel)); l > maxLen {
				maxLen = l
			}
		}
		// Include the dynamic close label to prevent truncation
		if node.closeLabel != "" {
			if l := len([]rune(node.closeLabel)); l > maxLen {
				maxLen = l
			}
		}
	}

	if node.IsInline {
		builder := InlineMarkup()
		for rIdx, row := range node.Rows {
			var inlineRow []any
			for cIdx, btn := range row {
				btnText := btn.Text
				if node.stretch && maxLen > 0 {
					btnText = normalizeMenuWidth(btnText, maxLen)
				}

				if btn.TargetID != "" {
					inlineRow = append(inlineRow, Btn(btnText).Callback(fmt.Sprintf("_menu:%s", btn.TargetID)))
				} else {
					inlineRow = append(inlineRow, Btn(btnText).Callback(fmt.Sprintf("_menu_act:%s:%d:%d", node.ID, rIdx, cIdx)))
				}
			}
			builder.Row(inlineRow...)
		}
		if node.ParentID != "" {
			backText := node.backLabel
			if node.stretch && maxLen > 0 {
				backText = normalizeMenuWidth(backText, maxLen)
			}
			builder.Row(Btn(backText).Callback("_menu:back"))
		}
		if node.closeLabel != "" {
			closeText := node.closeLabel
			if node.stretch && maxLen > 0 {
				closeText = normalizeMenuWidth(closeText, maxLen)
			}
			builder.Row(Btn(closeText).Callback("_menu:close"))
		}
		return builder.Build()
	}

	builder := ReplyMarkup()
	for _, row := range node.Rows {
		var replyRow []any
		for _, btn := range row {
			btnText := btn.Text
			if node.stretch && maxLen > 0 {
				btnText = normalizeMenuWidth(btnText, maxLen)
			}
			replyRow = append(replyRow, btnText)
		}
		builder.Row(replyRow...)
	}
	if node.ParentID != "" {
		backText := node.backLabel
		if node.stretch && maxLen > 0 {
			backText = normalizeMenuWidth(backText, maxLen)
		}
		builder.Row(backText)
	}
	if node.closeLabel != "" {
		closeText := node.closeLabel
		if node.stretch && maxLen > 0 {
			closeText = normalizeMenuWidth(closeText, maxLen)
		}
		builder.Row(closeText)
	}
	return builder.Build()
}

// SendMenu displays a configured menu directly, automatically deleting the previous menu message asynchronously
func (c *Ctx) SendMenu(menuID string) error {
	targetID := menuID
	sess := c.Session()

	// Resolve virtual back-navigation target using native fast in-memory SessionGet
	if menuID == "back" {
		var history []string
		if rawHist, ok := SessionGet[[]string](sess, "_menu_history"); ok {
			history = rawHist
		}

		if len(history) > 0 {
			targetID = history[len(history)-1]
			history = history[:len(history)-1]
			SessionSet(sess, "_menu_history", history)
		} else {
			// Fallback to static ParentID of the current active menu
			if currentID, ok := SessionGet[string](sess, "_current_menu"); ok && currentID != "" {
				c.Bot.mu.RLock()
				curNode, okNode := c.Bot.menus[currentID]
				c.Bot.mu.RUnlock()
				if okNode && curNode != nil && curNode.ParentID != "" {
					targetID = curNode.ParentID
				} else {
					return fmt.Errorf("no parent menu or history found to go back")
				}
			}
		}
	}

	c.Bot.mu.RLock()
	node, ok := c.Bot.menus[targetID]
	c.Bot.mu.RUnlock()
	if !ok {
		return fmt.Errorf("menu %q not found", targetID)
	}

	// 1. Maintain dynamic back-stack navigation history in-memory (only for forward movements)
	currentMenu, _ := SessionGet[string](sess, "_current_menu")

	// Detect if transitioning from a Reply Keyboard to an Inline Keyboard
	wasReply := false
	if currentMenu != "" {
		c.Bot.mu.RLock()
		curNode, okCur := c.Bot.menus[currentMenu]
		c.Bot.mu.RUnlock()
		if okCur && curNode != nil && !curNode.IsInline {
			wasReply = true
		}
	}

	isToInline := node.IsInline
	isInlineEdit := c.Update != nil && c.Update.CallbackQuery != nil && node.IsInline

	// Resolve the safe, robust chat ID using c.ChatID() instead of raw c.Message.Chat.ID
	chatID, errChat := c.ChatID()
	if errChat != nil {
		return errChat
	}

	// Fetch dynamic multi-deletions list from session in-memory
	var msgIDs []int64
	if rawIDs, ok := SessionGet[[]int64](sess, "_menu_msg_ids"); ok {
		msgIDs = rawIDs
	}

	// 2. Automatically delete previous menu messages asynchronously if NOT editing in-place (Anti-Flicker)
	if !isInlineEdit && len(msgIDs) > 0 {
		botInstance := c.Bot
		for _, id := range msgIDs {
			if id > 0 {
				c.Go(func() {
					_ = botInstance.BaseRequest(context.Background(), "deleteMessage", map[string]any{
						"chat_id":    chatID,
						"message_id": id,
					}, nil)
				})
			}
		}
		msgIDs = []int64{}
	}

	// Save active menu ID to session in-memory
	SessionSet(sess, "_current_menu", targetID)

	// Load static precompiled keyboard and text as default
	kb := node.CachedKeyboard
	text := node.CachedText

	// Resolve user-specific dynamic text if generator is defined
	if node.DynamicText != nil {
		textKey := "menu:txt:" + node.ID
		if node.DynamicKey != nil {
			textKey = "menu:txt:" + node.DynamicKey(chatID)
		}
		if cached, ok := c.Cache().Get(textKey).Go(); ok {
			if str, okStr := cached.(string); okStr {
				text = str
			}
		} else {
			text = node.DynamicText(chatID)
			if node.stretch {
				text = stretchText(text)
			}
			ttl := node.TTL
			if ttl == 0 {
				ttl = 5 * time.Minute
			}
			c.Cache().Set(textKey, text, ttl).Go()
		}
	}

	// Resolve user-specific dynamic keyboard if generator is defined
	if node.DynamicKeyboard != nil {
		kbKey := "menu:kb:" + node.ID
		if node.DynamicKey != nil {
			kbKey = "menu:kb:" + node.DynamicKey(chatID)
		}
		if cached, ok := c.Cache().Get(kbKey).Go(); ok {
			kb = cached
		} else {
			kb = node.DynamicKeyboard(chatID)
			ttl := node.TTL
			if ttl == 0 {
				ttl = 5 * time.Minute
			}
			c.Cache().Set(kbKey, kb, ttl).Go()
		}
	}

	// 3. Reply → Inline transition: send ONE real message with the actual menu text + ReplyKeyboardRemove
	if wasReply && isToInline {
		msg, err := c.Bot.Send(chatID).Text(text).MarkupRemove().Context(c.ctx).Go()
		if err != nil {
			return err
		}
		if msg != nil && msg.MessageID > 0 {
			if kb != nil {
				_, _ = c.Bot.Edit(chatID, msg.MessageID).Markup(kb).Go()
			}
			msgIDs = append(msgIDs, msg.MessageID)
			SessionSet(sess, "_menu_msg_ids", msgIDs)
			SessionSet(sess, "_menu_msg_id", msg.MessageID)
		}
		return nil
	}

	// 4. If already inside a callback query (Inline transition), edit in-place to prevent flicker
	if isInlineEdit {
		msg, err := c.Edit().Text(text).Markup(kb).Go()
		if err != nil && !strings.Contains(err.Error(), "message is not modified") {
			// Suppress annoying "message is not modified" API error safely
			return err
		}
		if msg != nil && msg.MessageID > 0 {
			// Update active messages stack for subsequent cleanup
			found := false
			for _, id := range msgIDs {
				if id == msg.MessageID {
					found = true
					break
				}
			}
			if !found {
				msgIDs = append(msgIDs, msg.MessageID)
			}
			SessionSet(sess, "_menu_msg_ids", msgIDs)
			SessionSet(sess, "_menu_msg_id", msg.MessageID)
		}
		return nil
	}

	// 5. Send as a fresh message and save its ID to session active stack (Robust multi-cleanup)
	msg, err := c.Send().Text(text).Markup(kb).Go()
	if msg != nil && msg.MessageID > 0 {
		msgIDs = append(msgIDs, msg.MessageID)
		SessionSet(sess, "_menu_msg_ids", msgIDs)
		SessionSet(sess, "_menu_msg_id", msg.MessageID)
	}
	return err
}

// GetMenuNode retrieves a compiled menu node in a completely lock-free, high-performance manner
func (b *Bot) GetMenuNode(id string) (*MenuNode, bool) {
	val := b.menusAtomic.Load()
	if val == nil {
		// Fallback to read-lock if the atomic container is not populated yet
		b.mu.RLock()
		defer b.mu.RUnlock()
		node, ok := b.menus[id]
		return node, ok
	}
	node, ok := val.(map[string]*MenuNode)[id]
	return node, ok
}

// IsReplyButton checks if a text matches any reply button in a completely lock-free manner
func (b *Bot) IsReplyButton(text string) bool {
	val := b.replyButtonsAtomic.Load()
	if val == nil {
		// Fallback to read-lock if the atomic container is not populated yet
		b.mu.RLock()
		defer b.mu.RUnlock()
		return b.replyButtons[text]
	}
	return val.(map[string]bool)[text]
}

// InvalidateMenu clears dynamic menu cache entries for a specific chat
func (c *Ctx) InvalidateMenu(menuID string, chatID int64) {
	chatIDStr := strconv.FormatInt(chatID, 10)
	c.Cache().Del("menu:kb:" + menuID + ":" + chatIDStr).Go()
	c.Cache().Del("menu:txt:" + menuID + ":" + chatIDStr).Go()
}
