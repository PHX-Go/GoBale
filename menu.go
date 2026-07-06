package gobale

import (
	"fmt"
	"strings"
	"time"
)

// MenuButton represents a single action inside the dynamic menu builder
type MenuButton struct {
	Text     string
	TargetID string  // Next sub-menu ID to transition to
	Handler  Handler // Optional custom logic to execute on click
}

// MenuNode represents a single menu page containing layout and buttons
type MenuNode struct {
	ID         string
	TextVal    string
	ParentID   string
	IsInline   bool
	Rows       [][]MenuButton
	backLabel  string
	stretch    bool   // Support framework's text stretching
	closeLabel string // Custom close menu button label
}

// MenuBtn initiates a menu button fluent configuration
func MenuBtn(text string) MenuButton {
	return MenuButton{Text: text}
}

// Target configures the button to transition to another menu ID
func (b MenuButton) Target(menuID string) MenuButton {
	b.TargetID = menuID // Update TargetID field safely
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
		IsInline:  true, // Default to inline glass keyboards
		backLabel: "🔙 بازگشت",
		stretch:   true, // Default to true to enable framework's alignment
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

// CompileMenus automatically registers stateful middleware and callback routers
func (b *Bot) CompileMenus() {
	b.menusOnce.Do(func() {
		// 1. Stateful global middleware to intercept Reply Keyboards without route collisions
		b.On().Use(func(c *Ctx) {
			if c.Message != nil && c.Message.Text != "" {
				text := c.Message.Text

				// Retrieve user's current menu ID from their active Session
				raw, err := c.Session().Data("_current_menu").Go()
				if err == nil && raw != nil {
					if menuID, ok := raw.(string); ok && menuID != "" {
						b.mu.RLock()
						node, ok := b.menus[menuID]
						b.mu.RUnlock()

						if ok && !node.IsInline {
							// Dynamically match and handle Close Menu event
							if node.closeLabel != "" && text == node.closeLabel {
								c.Abort()

								// Safely delete user's input message (Only allowed in groups/supergroups)
								if !c.IsPrivate() {
									_ = c.Del().Go()
								}

								// Delete the menu message
								if prevMsgID := c.Session().Int64("_menu_msg_id"); prevMsgID > 0 {
									_ = b.BaseRequest(c.ctx, "deleteMessage", map[string]any{
										"chat_id":    c.Message.Chat.ID,
										"message_id": prevMsgID,
									}, nil)
								}

								// Collapse user's keyboard and send a self-destroying message
								_, _ = c.Send().Text("🔒 منو با موفقیت بسته شد.").MarkupRemove().Temp(3 * time.Second).Go()

								// Reset session states
								_, _ = c.Session().Data("_current_menu", "").Go()
								_, _ = c.Session().Data("_menu_msg_id", 0).Go()
								_, _ = c.Session().Data("_menu_history", []string{}).Go()
								return
							}

							// Dynamically match the customized back button label
							if node.ParentID != "" && text == node.backLabel {
								c.Abort() // Abort downstream handlers
								_ = c.SendMenu("back")
								return
							}

							// Match and route standard Reply Keyboard menu buttons
							for _, row := range node.Rows {
								for _, btn := range row {
									if text == btn.Text {
										c.Abort() // Abort downstream handlers
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
	})
}

// BuildMenuKeyboard creates the correct markup interface for inline or reply menu nodes with symmetrical centering
func (b *Bot) BuildMenuKeyboard(node *MenuNode) any {
	// Calculate max button label length for perfect ASCII centering across the entire keyboard
	maxLen := 0
	if node.stretch {
		maxLen = 45 // Enforce baseline minimum width of 45 to strictly align with the stretched message bubble
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
			builder.Row(Btn(closeText).Callback("_menu:close")) // Trigger dynamic close router
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

// SendMenu displays a configured menu directly, automatically deleting the previous menu message to prevent clutter
func (c *Ctx) SendMenu(menuID string) error {
	targetID := menuID

	// Resolve virtual back-navigation target
	if menuID == "back" {
		var history []string
		rawHist, errHist := c.Session().Data("_menu_history").Go()
		if errHist == nil && rawHist != nil {
			if h, okSlice := rawHist.([]string); okSlice {
				history = h
			}
		}

		if len(history) > 0 {
			targetID = history[len(history)-1]
			history = history[:len(history)-1]
			_, _ = c.Session().Data("_menu_history", history).Go()
		} else {
			// Fallback to static ParentID of the current active menu
			rawCurrent, errCurrent := c.Session().Data("_current_menu").Go()
			if errCurrent == nil && rawCurrent != nil {
				if currentID, ok := rawCurrent.(string); ok && currentID != "" {
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
	}

	c.Bot.mu.RLock()
	node, ok := c.Bot.menus[targetID]
	c.Bot.mu.RUnlock()
	if !ok {
		return fmt.Errorf("menu %q not found", targetID)
	}

	// 1. Maintain dynamic back-stack navigation history (only for forward movements)
	var currentMenu string
	raw, err := c.Session().Data("_current_menu").Go()
	if err == nil && raw != nil {
		currentMenu, _ = raw.(string)
	}

	if currentMenu != "" && currentMenu != targetID && menuID != "back" {
		var history []string
		rawHist, errHist := c.Session().Data("_menu_history").Go()
		if errHist == nil && rawHist != nil {
			if h, okSlice := rawHist.([]string); okSlice {
				history = h
			}
		}

		// Detect if we are navigating backward through history
		isBack := false
		for i, hID := range history {
			if hID == targetID {
				history = history[:i] // trim future forward stack
				isBack = true
				break
			}
		}

		// Push to history only if navigating forward to a non-parent page
		if !isBack && node.ParentID != "" && currentMenu != node.ParentID {
			history = append(history, currentMenu)
		}
		_, _ = c.Session().Data("_menu_history", history).Go()
	}

	isInlineEdit := c.Update != nil && c.Update.CallbackQuery != nil && node.IsInline

	// 2. Automatically delete previous menu message only if NOT editing in-place (Anti-Flicker)
	if !isInlineEdit {
		if prevMsgID := c.Session().Int64("_menu_msg_id"); prevMsgID > 0 {
			_ = c.Bot.BaseRequest(c.ctx, "deleteMessage", map[string]any{
				"chat_id":    c.Message.Chat.ID,
				"message_id": prevMsgID,
			}, nil)
		}
	}

	// Save active menu ID to session
	_, _ = c.Session().Data("_current_menu", targetID).Go()

	kb := c.Bot.BuildMenuKeyboard(node)

	// 3. If already inside a callback query (Inline transition), edit in-place to prevent flicker
	if isInlineEdit {
		msg, err := c.Edit().Text(node.TextVal).Markup(kb).Stretch(node.stretch).Go()
		if err != nil && !strings.Contains(err.Error(), "message is not modified") {
			// Suppress annoying "message is not modified" API error safely
			return err
		}
		if err == nil && msg != nil {
			_, _ = c.Session().Data("_menu_msg_id", msg.MessageID).Go()
		}
		return nil
	}

	// 4. Send as a fresh message and save its ID to session
	msg, err := c.Send().Text(node.TextVal).Markup(kb).Stretch(node.stretch).Go()
	if err == nil && msg != nil {
		_, _ = c.Session().Data("_menu_msg_id", msg.MessageID).Go()
	}
	return err
}
