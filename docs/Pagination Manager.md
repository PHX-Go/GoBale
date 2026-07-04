# Core Pagination Manager: Zero-Flicker & Butter-Smooth UI/UX

The Core Pagination Manager is a high-performance, production-grade pagination engine built natively into the framework. It is specifically engineered to eliminate **Layout Shifts (flickering, shivering, and vertical/horizontal jumping)** in mobile and desktop chat clients (such as React Native, Web, and Native applications).

By encapsulating complex mathematical calculations, proportional font spacing, session tracking, and speculative layout stabilization, it allows developers to render smooth, dynamic paginated menus with a single fluent declaration.

---

## 1. Architectural Design & Technical Pillars

A standard pagination implementation in chat bots often suffers from visual jitter during page transitions. This framework resolves these visual anomalies through four core engineering pillars:

```
+--------------------------------------------------------+
|                      Bubble Header                     |
|           "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"         |  <-- Stretched to Max Width (Static Boundary)
|                                                        |
+--------------------------------------------------------+
|                      Product Rows                      |
|       [ Product Item Label padded to Max Width ]       |  <-- Single Button per Row (100% Width)
|                                                        |
+--------------------------------------------------------+
|                     Navigation Row                     |
|       [ Prev Page (Static) ] [ Page num ] [ Next ]     |  <-- Symmetrical Columns & Byte-Identical Labels
|                                                        |
+--------------------------------------------------------+
```

### I. Symmetrical Column Widths (Zero Horizontal Jitter)
In multi-column rows (such as the navigation row `[ Prev ] [ Page Num ] [ Next ]`), the client rendering engine distributes horizontal percentages based on proportional text widths. 
* **The Problem:** If the "Previous" button is hidden or replaced by a different text (like `"-"` or space `" "`) on Page 1, the column widths shrink or expand, causing the navigation buttons to "shiver" or "jump" horizontally.
* **The Solution:** The navigation buttons maintain **100% byte-for-byte identical text labels** across all edits (e.g. `"صفحه قبل"` and `"صفحه بعد"`). When a button is logically inactive (such as "Previous" on Page 1), its text remains identical, but its callback data is mapped to a silent, non-blocking `noop` action. Since the label remains identical, the client-side percentage layout calculations never change, keeping the buttons perfectly still.

### II. Standard ASCII Space Padding (Zero Font-Fallback Delay)
Using invisible Unicode space characters (like Braille Pattern Blank `\u2800`) to pad string lengths often triggers **Font Fallback lookups** on mobile devices. The text engine has to load a secondary font file to render the missing glyph, introducing a 1-frame layout-pass latency (visual flicker).
* **The Solution:** We pad shorter product names to match the longest item's length (`labelWidth`) using standard **ASCII whitespace characters (`" "`)**. Standard spaces are natively mapped in the primary Persian/Arabic system fonts (e.g., Vazirmatn), executing layout calculations instantly in a single render pass without any snapping.

### III. Message Bubble Width Locking (Zero Vertical Shaking)
The message bubble's width in chat clients dynamically resizes to fit the longest line of text or the widest button on the current page. If Page 1 has a longer title than Page 2, the entire bubble resizes, throwing off the keyboard coordinates.
* **The Solution:** The framework couples the core pagination engine with the native `.Stretch(true)` utility. This appends invisible spacing to the description text, locking the message bubble to a constant minimum width of 35 characters across all pages. Even when the active button row count decreases on the last page, the remaining buttons stay perfectly centered, and the layout transitions smoothly.

### IV. Sequential Execution Pipeline (Zero Animation Clashing)
When a user clicks a button, the button entering the loading state triggers an animation. If the bot concurrently sends `editMessageText` and `answerCallbackQuery` in parallel, the client-side state machine might receive the updates out-of-order or process them during active animation frames, causing a violent re-rendering flash.
* **The Solution:** The core engine strictly sequences the updates:
  1. It executes `editMessageText` synchronously to apply the new text and keyboard layout.
  2. It executes `answerCallbackQuery` synchronously right after, smoothly turning off the button spinner.
  This sequential execution removes any rendering animation clashes on the client-side React Native engine.

---

## 2. API Reference

The following fluent chain methods are available on `On().Paginate(prefix)` builder:

| Method | Signature | Description |
| :--- | :--- | :--- |
| **`Items`** | `Items(items []InlineKeyboardButton)` | Registers the complete list of buttons to be paginated (Standard Mode). |
| **`Matrix`** | `Matrix(titles []string)` | Registers the raw list of titles and activates the static selection digit row `[ 1 ] [ 2 ] ...` (Matrix Mode). |
| **`PerPage`** | `PerPage(n int)` | Sets the maximum number of items displayed per single page (defaults to 5). |
| **`Loop`** | `Loop(v bool)` | Enables infinite circular looping. Page $N \to 1$ on Next, and Page $1 \to N$ on Prev. |
| **`NavLabels`** | `NavLabels(prev, next string)` | Overrides the default previous/next button captions (defaults to `"قبلی »"` and `"« بعدی"`). |
| **`Text`** | `Text(fn func(page, totalPages int) string)` | Sets a dynamic text generator that provides the description text for each page. |
| **`Go`** | `Go()` | Finalizes the registration and automatically binds the required internal callback routes. |

---

## 3. Advanced Integration Example

The following example demonstrates how to configure and run the native, butter-smooth pagination manager for an e-commerce catalog, utilizing advanced framework features such as custom navigation labels, infinite looping, context logging, and GOB sharded sessions:

```go
package main

import (
	"fmt"
	"log"

	gobale "github.com/PHX-Go/GoBale"
)

// Product represents our e-commerce data model
type Product struct {
	ID    int64
	Title string
	Price int64
}

func main() {
	// Load configurations from environment file
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	// Initialize high-performance bot instance
	bot, err := gobale.New(token).Admin(adminID).Gzip().Go()
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// Products database
	products := []Product{
		{ID: 1, Title: "MacBook Pro M3", Price: 120000000},
		{ID: 2, Title: "iPhone 15 Pro", Price: 75000000},
		{ID: 3, Title: "iPad Pro 12.9", Price: 62000000},
		{ID: 4, Title: "Sony WH-1000XM5", Price: 18000000},
		{ID: 5, Title: "Keychron K2 Pro", Price: 8500000},
	}

	// Convert products list to raw unpadded buttons
	var buttons []gobale.InlineKeyboardButton
	for _, p := range products {
		label := fmt.Sprintf("%s — %s تومان", p.Title, gobale.Money(p.Price))
		buttons = append(buttons, gobale.Btn(label).Callback(fmt.Sprintf("prod:%d", p.ID)).Build())
	}

	// 1. REGISTER THE PAGINATION CATALOG NATIVELY IN BOOTSTRAP
	bot.On().Paginate("store_catalog").
		Items(buttons).
		PerPage(2).                      // 2 items per page
		Loop(true).                      // Enable circular endless scrolling
		NavLabels("قبلی ⬅️", "➡️ بعدی"). // Configure custom labels for previous and next buttons
		Text(func(page, totalPages int) string {
			// Plain text template (No markdown parsing latency)
			return fmt.Sprintf("📦 لیست محصولات فروشگاه بله (صفحه %d از %d)\n\nمحصول مورد نظر خود را جهت خرید انتخاب کنید:", page+1, totalPages)
		}).
		Go()

	// 2. Dispatch initial page smoothly on /start command
	bot.On().Cmd("start").Do(func(c *gobale.Ctx) {
		// Log action using context-linked logger
		c.Log().Info("Initializing store catalog menu for user: %d", c.SenderID())

		// Initialize session state for first page smoothly
		chatID, _ := c.ChatID()
		stateKey := fmt.Sprintf("page_state_store_catalog_%d", chatID)
		_, _ = c.Session().Data(stateKey, 0).Go()

		// Send page 0 utilizing the native Stretch automatic padding
		_, _ = c.SendPage("store_catalog", 0).Go()
	})

	// 3. Product details select handler
	bot.On().Callback("prod").Do(func(c *gobale.Ctx) {
		var prodID int64
		_ = c.ScanCallbackArgs(&prodID)

		var title string
		for _, p := range products {
			if p.ID == prodID {
				title = p.Title
				break
			}
		}

		c.Log().Info("Product selected: %s by user: %d", title, c.SenderID())
		_ = c.Answer().Text(fmt.Sprintf("🛒 %s به سبد خرید اضافه شد.", title)).Alert().Go()
	})

	// Dummy callback handler for zero-width placeholders
	bot.On().Callback("noop").Do(func(c *gobale.Ctx) {
		_ = c.Answer().Go()
	})

	log.Println("E-Commerce Bot is running with Golden Standard Pagination UI/UX...")
	bot.Run().Polling().Go()
}
```
