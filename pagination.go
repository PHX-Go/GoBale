package gobale

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

// normalizeWidth stretches or truncates a string to match exactly width parameter using standard ASCII spaces
func normalizeWidth(s string, width int) string {
	r := []rune(s)
	if len(r) > width { // Only truncate if length strictly exceeds target width
		if width <= 1 {
			return string(r[:width])
		}
		return string(r[:width-1]) + "…"
	}
	return s + strings.Repeat(" ", width-len(r))
}

// PaginationBuilder configures zero-flicker, session-based paginated menus
type PaginationBuilder struct {
	bot            *Bot
	prefix         string
	items          []InlineKeyboardButton
	itemTitles     []string // Raw titles used for matrix mode
	isMatrix       bool
	perPage        int
	loop           bool // Optional infinite circular looping
	labelWidth     int  // Automatically calculated maximum label width
	textFunc       func(page, totalPages int) string
	matrixTextFunc func(page, totalPages int, pageTitles []string) string
	onMatrixSelect func(c *Ctx, globalIndex int, title string)

	// Cached nav-row geometry
	navPageDigits  int
	navArrowWidth  int
	navPrevLabel   string
	navNextLabel   string
	navPlaceholder string

	// Optional custom nav captions
	navCustomPrev string
	navCustomNext string

	// Precomputed page cache
	cacheMu   sync.RWMutex
	kbCache   map[int]*InlineKeyboardMarkup
	textCache map[int]string

	// Optional custom stretch config
	stretch bool
}

// Paginate opens the fluent pagination registration chain from OnChain
func (o *OnChain) Paginate(prefix string) *PaginationBuilder {
	return &PaginationBuilder{
		bot:     o.bot,
		prefix:  prefix,
		perPage: 5,    // Default items per page
		stretch: true, // Default to true to stabilize widths
	}
}

// Items registers the list of inline buttons to be paginated (Standard Mode)
func (pb *PaginationBuilder) Items(items []InlineKeyboardButton) *PaginationBuilder {
	pb.items = items
	pb.isMatrix = false

	// Automatically calculate the maximum label width
	maxW := 0
	for _, item := range items {
		if l := len([]rune(item.Text)); l > maxW {
			maxW = l
		}
	}
	pb.labelWidth = maxW
	return pb
}

// Matrix registers the raw titles list and activates static selections (Matrix Mode)
func (pb *PaginationBuilder) Matrix(titles []string) *PaginationBuilder {
	pb.itemTitles = titles
	pb.isMatrix = true
	return pb
}

// PerPage registers the maximum items shown per page
func (pb *PaginationBuilder) PerPage(n int) *PaginationBuilder {
	if n > 0 {
		pb.perPage = n
	}
	return pb
}

// Loop configures the pagination to cycle back to the first page when reaching the end
func (pb *PaginationBuilder) Loop(v bool) *PaginationBuilder {
	pb.loop = v
	return pb
}

// Stretch configures if stretching is enabled for this pagination (defaults to true)
func (pb *PaginationBuilder) Stretch(v bool) *PaginationBuilder {
	pb.stretch = v
	return pb
}

// NavLabels overrides the prev/next button captions
func (pb *PaginationBuilder) NavLabels(prev, next string) *PaginationBuilder {
	pb.navCustomPrev = prev
	pb.navCustomNext = next
	return pb
}

// Text registers a dynamic function to generate text based on page index (Standard Mode)
func (pb *PaginationBuilder) Text(fn func(page, totalPages int) string) *PaginationBuilder {
	pb.textFunc = fn
	return pb
}

// MatrixText registers a dynamic text generator receiving sliced page items (Matrix Mode)
func (pb *PaginationBuilder) MatrixText(fn func(page, totalPages int, pageTitles []string) string) *PaginationBuilder {
	pb.matrixTextFunc = fn
	return pb
}

// OnMatrixSelect registers a callback triggered when a static selection key is clicked
func (pb *PaginationBuilder) OnMatrixSelect(fn func(c *Ctx, globalIndex int, title string)) *PaginationBuilder {
	pb.onMatrixSelect = fn
	return pb
}

// totalPages computes total pages for Standard mode based on the current item count
func (pb *PaginationBuilder) totalPages() int {
	tp := (len(pb.items) + pb.perPage - 1) / pb.perPage
	if tp == 0 {
		tp = 1
	}
	return tp
}

// prepareNavGeometry computes the fixed widths used by every nav row
func (pb *PaginationBuilder) prepareNavGeometry() {
	if pb.navArrowWidth > 0 {
		return // already computed
	}
	pb.navPageDigits = len(strconv.Itoa(pb.totalPages()))

	prev := pb.navCustomPrev
	if prev == "" {
		prev = "قبلی »"
	}
	next := pb.navCustomNext
	if next == "" {
		next = "« بعدی"
	}
	w := len([]rune(prev))
	if nw := len([]rune(next)); nw > w {
		w = nw
	}
	pb.navArrowWidth = w
	pb.navPrevLabel = normalizeWidth(prev, w)
	pb.navNextLabel = normalizeWidth(next, w)
	pb.navPlaceholder = strings.Repeat(" ", w)
}

// buildStandardKeyboard is the SINGLE source of truth for rendering standard keyboard
func (pb *PaginationBuilder) buildStandardKeyboard(page int) *InlineKeyboardMarkup {
	pb.prepareNavGeometry()
	totalPages := pb.totalPages()

	if page < 0 {
		page = 0
	}
	if page > totalPages-1 {
		page = totalPages - 1
	}

	var navRow []InlineKeyboardButton

	showPrev := pb.loop || page > 0
	showNext := pb.loop || page < totalPages-1

	if showNext {
		navRow = append(navRow, NewInlineKeyboardButtonData(pb.navNextLabel, pb.prefix+"_next"))
	} else {
		navRow = append(navRow, NewInlineKeyboardButtonData(pb.navPlaceholder, "noop"))
	}

	pageLabel := fmt.Sprintf("%0*d / %0*d", pb.navPageDigits, page+1, pb.navPageDigits, totalPages)
	navRow = append(navRow, NewInlineKeyboardButtonData(pageLabel, "noop"))

	if showPrev {
		navRow = append(navRow, NewInlineKeyboardButtonData(pb.navPrevLabel, pb.prefix+"_prev"))
	} else {
		navRow = append(navRow, NewInlineKeyboardButtonData(pb.navPlaceholder, "noop"))
	}

	navBuilder := InlineMarkup()
	navBuilder.markup.InlineKeyboard = append(navBuilder.markup.InlineKeyboard, navRow)
	navKb := navBuilder.Build()

	return pb.bot.buildCombinedPaginationKeyboard(pb.items, page, pb.perPage, navKb, pb.labelWidth)
}

// renderStandardText resolves the message body for a given Standard-mode page
func (pb *PaginationBuilder) renderStandardText(page int) string {
	totalPages := pb.totalPages()
	if pb.textFunc != nil {
		return pb.textFunc(page, totalPages)
	}
	return fmt.Sprintf("Page %d of %d", page+1, totalPages)
}

// buildCache precomputes every page's keyboard + text once
func (pb *PaginationBuilder) buildCache() {
	tp := pb.totalPages()
	kb := make(map[int]*InlineKeyboardMarkup, tp)
	txt := make(map[int]string, tp)
	for i := 0; i < tp; i++ {
		kb[i] = pb.buildStandardKeyboard(i)
		txt[i] = pb.renderStandardText(i)
	}

	pb.cacheMu.Lock()
	pb.kbCache = kb
	pb.textCache = txt
	pb.cacheMu.Unlock()
}

// getPage returns the (keyboard, text) pair for a page from cache
func (pb *PaginationBuilder) getPage(page int) (*InlineKeyboardMarkup, string) {
	pb.cacheMu.RLock()
	kb, okKb := pb.kbCache[page]
	txt, okTxt := pb.textCache[page]
	pb.cacheMu.RUnlock()

	if okKb && okTxt {
		return kb, txt
	}

	kb = pb.buildStandardKeyboard(page)
	txt = pb.renderStandardText(page)

	pb.cacheMu.Lock()
	if pb.kbCache == nil {
		pb.kbCache = make(map[int]*InlineKeyboardMarkup)
		pb.textCache = make(map[int]string)
	}
	pb.kbCache[page] = kb
	pb.textCache[page] = txt
	pb.cacheMu.Unlock()

	return kb, txt
}

// InvalidateCache forces a rebuild of the precomputed page cache
func (pb *PaginationBuilder) InvalidateCache() {
	pb.buildCache()
}

// Go registers all required static callbacks and navigation routes automatically
func (pb *PaginationBuilder) Go() {
	pb.bot.pagMu.Lock()
	pb.bot.paginations[pb.prefix] = pb
	pb.bot.pagMu.Unlock()

	prefix := pb.prefix

	// 1. REGISTRATION FOR STATIC MATRIX MODE
	if pb.isMatrix {
		perPage := pb.perPage
		totalItems := len(pb.itemTitles)
		totalPages := (totalItems + perPage - 1) / perPage
		if totalPages == 0 {
			totalPages = 1
		}

		// Register PREVIOUS page callback
		pb.bot.On().Callback(prefix + "_prev").Do(func(c *Ctx) {
			_ = c.Answer().Go()

			chatID, _ := c.ChatID()
			msgID := c.Message.MessageID

			stateKey := fmt.Sprintf("page_state:%s:%d:%d", prefix, chatID, msgID)
			var curPage int
			if val, ok := c.Cache().Get(stateKey).Go(); ok {
				curPage = val.(int)
			} else {
				curPage = 0
			}

			var newPage int
			if curPage <= 0 {
				if pb.loop {
					newPage = totalPages - 1
				} else {
					_ = c.Answer().Text("شما در صفحه اول هستید.").Go()
					return
				}
			} else {
				newPage = curPage - 1
			}

			c.Cache().Set(stateKey, newPage, 30*time.Minute).Go()

			kb := pb.bot.buildStaticMatrixKeyboard(prefix, perPage)
			text := pb.renderMatrixText(newPage)

			_, _ = c.Edit().Text(text).Markup(kb).Markdown().Stretch(pb.stretch).Go()
		})

		// Register NEXT page callback
		pb.bot.On().Callback(prefix + "_next").Do(func(c *Ctx) {
			_ = c.Answer().Go()

			chatID, _ := c.ChatID()
			msgID := c.Message.MessageID

			stateKey := fmt.Sprintf("page_state:%s:%d:%d", prefix, chatID, msgID)
			var curPage int
			if val, ok := c.Cache().Get(stateKey).Go(); ok {
				curPage = val.(int)
			} else {
				curPage = 0
			}

			var newPage int
			if curPage >= totalPages-1 {
				if pb.loop {
					newPage = 0
				} else {
					_ = c.Answer().Text("شما در صفحه آخر هستید.").Go()
					return
				}
			} else {
				newPage = curPage + 1
			}

			c.Cache().Set(stateKey, newPage, 30*time.Minute).Go()

			kb := pb.bot.buildStaticMatrixKeyboard(prefix, perPage)
			text := pb.renderMatrixText(newPage)

			_, _ = c.Edit().Text(text).Markup(kb).Markdown().Stretch(pb.stretch).Go()
		})

		// Register MATRIX ITEM SELECT callback
		pb.bot.On().Callback(prefix + "_select").Do(func(c *Ctx) {
			var selectedIndex int
			_ = c.ScanCallbackArgs(&selectedIndex)

			chatID, _ := c.ChatID()
			msgID := c.Message.MessageID

			stateKey := fmt.Sprintf("page_state:%s:%d:%d", prefix, chatID, msgID)
			var curPage int
			if val, ok := c.Cache().Get(stateKey).Go(); ok {
				curPage = val.(int)
			} else {
				curPage = 0
			}

			globalIndex := (curPage * perPage) + (selectedIndex - 1)
			if globalIndex >= totalItems {
				_ = c.Answer().Text("محصولی در این شماره وجود ندارد.").Go()
				return
			}

			_ = c.Answer().Go()

			if pb.onMatrixSelect != nil {
				pb.onMatrixSelect(c, globalIndex, pb.itemTitles[globalIndex])
			}
		})

		return
	}

	// 2. REGISTRATION FOR STANDARD MODE
	pb.prepareNavGeometry()
	pb.buildCache()

	pb.bot.On().Callback(prefix + "_prev").Do(func(c *Ctx) {
		msgID := c.Message.MessageID
		chatID, _ := c.ChatID()
		stateKey := fmt.Sprintf("page_state_%s_%d_%d", prefix, chatID, msgID)
		curPage := c.Session().Int(stateKey, 0)

		totalPages := pb.totalPages()
		var newPage int
		if curPage <= 0 {
			if pb.loop {
				newPage = totalPages - 1
			} else {
				_ = c.Answer().Text("شما در صفحه اول هستید.").Go()
				return
			}
		} else {
			newPage = curPage - 1
		}

		_, _ = c.Session().Data(stateKey, newPage).Go()

		finalKb, desc := pb.getPage(newPage)

		// 1. EDIT FIRST (Sequential, non-race)
		_, _ = c.Edit().Text(desc).Markup(finalKb).Markdown().Stretch(pb.stretch).Go()
		// 2. ANSWER SECOND (Sequential, non-race)
		_ = c.Answer().Go()
	})

	pb.bot.On().Callback(prefix + "_next").Do(func(c *Ctx) {
		msgID := c.Message.MessageID
		chatID, _ := c.ChatID()
		stateKey := fmt.Sprintf("page_state_%s_%d_%d", prefix, chatID, msgID)
		curPage := c.Session().Int(stateKey, 0)

		totalPages := pb.totalPages()
		var newPage int
		if curPage >= totalPages-1 {
			if pb.loop {
				newPage = 0
			} else {
				_ = c.Answer().Text("شما در صفحه آخر هستید.").Go()
				return
			}
		} else {
			newPage = curPage + 1
		}

		_, _ = c.Session().Data(stateKey, newPage).Go()

		finalKb, desc := pb.getPage(newPage)

		// 1. EDIT FIRST (Sequential, non-race)
		_, _ = c.Edit().Text(desc).Markup(finalKb).Markdown().Stretch(pb.stretch).Go()
		// 2. ANSWER SECOND (Sequential, non-race)
		_ = c.Answer().Go()
	})
}

// SendPage construct and sends the initial paginated menu smoothly from Context
func (c *Ctx) SendPage(prefix string, page int) *SendChain {
	id, _ := c.ChatID()
	return c.Bot.SendPage(id, prefix, page)
}

// SendPage construct and sends the initial paginated menu smoothly from Bot
func (b *Bot) SendPage(chatID any, prefix string, page int) *SendChain {
	b.pagMu.RLock()
	pb, ok := b.paginations[prefix]
	b.pagMu.RUnlock()

	if !ok {
		log.Printf("[GoBale Pagination Error] Prefix %q is not registered", prefix)
		return b.Send(chatID).Text("Error: Pagination is not configured")
	}

	var kb *InlineKeyboardMarkup
	var desc string

	if pb.isMatrix {
		kb = b.buildStaticMatrixKeyboard(pb.prefix, pb.perPage)
		desc = pb.renderMatrixText(page)
	} else {
		kb, desc = pb.getPage(page)
	}

	return b.Send(chatID).Text(desc).Markup(kb).Markdown().Stretch(pb.stretch)
}

// buildCombinedPaginationKeyboard merges static page items with the static navigation row using customized label widths
func (b *Bot) buildCombinedPaginationKeyboard(items []InlineKeyboardButton, page, perPage int, navKb *InlineKeyboardMarkup, labelWidth int) *InlineKeyboardMarkup {
	start := page * perPage
	end := start + perPage
	totalItems := len(items)

	builder := InlineMarkup()

	for i := start; i < end && i < totalItems; i++ {
		btn := items[i]
		btn.Text = normalizeWidth(btn.Text, labelWidth)
		builder.Row(btn)
	}

	for _, row := range navKb.InlineKeyboard {
		builder.markup.InlineKeyboard = append(builder.markup.InlineKeyboard, row)
	}

	return builder.Build()
}

// buildStaticMatrixKeyboard creates the 100% static, non-shifting selection matrix keyboard natively
func (b *Bot) buildStaticMatrixKeyboard(prefix string, perPage int) *InlineKeyboardMarkup {
	builder := InlineMarkup()

	// 1. Build static digit selection row
	var selectRow []InlineKeyboardButton
	for i := 1; i <= perPage; i++ {
		numLabel := ToEnDigits(strconv.Itoa(i))
		selectRow = append(selectRow, NewInlineKeyboardButtonData(fmt.Sprintf(" %s ", numLabel), fmt.Sprintf("%s_select:%d", prefix, i)))
	}
	builder.markup.InlineKeyboard = append(builder.markup.InlineKeyboard, selectRow)

	// 2. Build static navigation row
	prevBtn := NewInlineKeyboardButtonData("صفحه قبل", prefix+"_prev")
	nextBtn := NewInlineKeyboardButtonData("صفحه بعد", prefix+"_next")
	builder.markup.InlineKeyboard = append(builder.markup.InlineKeyboard, []InlineKeyboardButton{prevBtn, nextBtn})

	return builder.Build()
}

// renderMatrixText slices the titles list and formats them into a structured text list with static line paddings
func (pb *PaginationBuilder) renderMatrixText(page int) string {
	start := page * pb.perPage
	end := start + pb.perPage
	totalItems := len(pb.itemTitles)
	if end > totalItems {
		end = totalItems
	}

	pageTitles := pb.itemTitles[start:end]
	totalPages := (totalItems + pb.perPage - 1) / pb.perPage
	if totalPages == 0 {
		totalPages = 1
	}

	if pb.matrixTextFunc != nil {
		return pb.matrixTextFunc(page, totalPages, pageTitles)
	}

	text := Text().Line(fmt.Sprintf("📦 Items List (Page %d of %d):", page+1, totalPages)).Line()
	for i, title := range pageTitles {
		text.Line(fmt.Sprintf("%d. %s", i+1, title))
	}
	return text.Go()
}
