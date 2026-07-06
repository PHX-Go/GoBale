# Dynamic Menu & Navigation Engine

The GoBale Dynamic Menu & Navigation Engine is a stateful, fluent, and highly optimized UI orchestration library for creating tree-like nested menus on the Bale Bot platform. It eliminates boilerplate routing and state handling, dynamically generating **symmetrical 40-character wide keyboards** and managing automatic previous-menu deletions for a pristine chat history.

## High-Level Features

* **Fluent Declarative API:** Configure multi-tier keyboards (Inline or Reply) on top of the dot-chained builder pattern.
* **Dynamic Back-Stack (FSM):** Tracks visited paths inside GOB session variables to return users to their exact preceding screen dynamically upon clicking a generic Back button.
* **Auto-Deleting Clutter-Free UX:** Cleans up previous menu messages automatically whenever a new menu is requested, ensuring the user's direct messages or groups are perfectly tidy.
* **No-Break ASCII Centering:** Employs non-breaking space padding (`\u00a0`) to prevent mobile and desktop clients from collapsing consecutive button spaces, guaranteeing full-width keyboards.
* **Settings Engine Integration:** Merges custom configurations with GoBale's built-in `Settings()` panel, auto-appending navigation back-routes natively.

---

## Fluent API Reference

### `(*Bot).Menu(id string) *MenuChain`
Initializes a new dynamic menu page inside the bot's global registry.
* **Default values:** `IsInline = true` (Inline Keyboard), `stretch = true`, `backLabel = "🔙 Return"`.

### `(*MenuChain).Text(t string) *MenuChain`
Sets the description text or caption to render above the keyboard.

### `(*MenuChain).Parent(parentID string) *MenuChain`
Registers a static parent menu ID. Used as a robust fallback navigation route if the session's dynamic back-stack is empty.

### `(*MenuChain).BackLabel(label string) *MenuChain`
Overrides the default back button text with a custom label (e.g., `BackLabel("🔙 Return to Shop")`).

### `(*MenuChain).CloseLabel(label string) *MenuChain`
Appends a dynamic "Close" button to the bottom row of the keyboard. Upon clicking, it safely deletes the message and flushes active navigation states.

### `(*MenuChain).Stretch(v bool) *MenuChain`
Enables or disables standard symmetrical 45-character alignment on text captions and button paddings.

### `(*MenuChain).Reply() *MenuChain`
Configures the menu to render as a standard **Reply Keyboard** instead of an inline markup.

### `(*MenuChain).Inline() *MenuChain`
Configures the menu to render as an **Inline Glass Keyboard** (Default).

### `(*MenuChain).Row(buttons ...MenuButton) *MenuChain`
Appends a single row of buttons. Buttons placed on separate `Row()` calls will automatically span the full-width of the chat window.

---

### `MenuBtn(text string) MenuButton`
Initializes a new button inside a menu row.
* `Target(menuID string)`: Points the button click to transition to another menu node.
* `Do(h Handler)`: Attaches a custom execution closure to be executed on click.

---

### `(*Ctx).SendMenu(menuID string) error`
Triggers a menu page directly from any handler context.
* If called within a `CallbackQuery`, it automatically **edits the message in-place** (Zero Flicker).
* If called from a text message or command, it **deletes the previous menu message** and sends a fresh one.

---

## Production-Ready Integration Example (`main.go`)

Save the following code inside your `main.go` file to see the comprehensive engine in action:

```go
package main

import (
	"fmt"
	"log"

	gobale "github.com/PHX-Go/GoBale"
)

func main() {
	// 1. Load environment configurations
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	// 2. Build GoBale bot instance
	bot, err := gobale.New(token).Admin(adminID).Workers(4).Go()
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// REGISTER SWITCHES INTO CORE SETTINGS ENGINE
	bot.Settings().
		RegisterLocal("antispam", "🛡️ Anti-Spam Shield", false).
		RegisterLocal("nightmode", "🌙 Night Mode", false)

	// DECLARE NESTED FLUENT MENUS

	// 1. Define Main Menu (Inline Glass Keyboard)
	bot.Menu("main").
		Text("🛍️ Welcome to our Smart Store!\nPlease select a section:").
		CloseLabel("❌ Close Menu"). // Dynamic customizable close button
		Stretch(true).              // Enforces symmetric 40-character width
		Row(gobale.MenuBtn("🛍️ Enter Store").Target("shop")).
		Row(gobale.MenuBtn("⚙️ System Settings").Target("settings")).
		Row(gobale.MenuBtn("👤 Account Settings (Reply)").Target("profile"))

	// 2. Define Shop Sub-Menu (Inline, automatically appends customized back button)
	bot.Menu("shop").
		Parent("main").
		BackLabel("🔙 Back to Home").
		Text("📦 List of Available Products:").
		Stretch(true).
		Row(gobale.MenuBtn("📚 Audio Book").Do(func(c *gobale.Ctx) {
			c.Send().Text("📚 Audio Book added to your shopping cart.").Go()
		})).
		Row(gobale.MenuBtn("💻 Go Course").Do(func(c *gobale.Ctx) {
			c.Send().Text("💻 Course License has been reserved for you.").Go()
		}))

	// 3. Define Settings Sub-Menu (Integrates with core Settings switches)
	bot.Menu("settings").
		Parent("main").
		BackLabel("🔙 Back to Home").
		Text("⚙️ Settings Management Panel:\nClick to toggle any module:").
		Stretch(true).
		Row(gobale.MenuBtn("⚙️ Manage Swicthes").Do(func(c *gobale.Ctx) {
			// Edit message in-place to show settings panel with 40-char stretch
			c.Edit().Text("⚙️ Active Bot Modules:").Settings().Stretch(true).Go()
		}))

	// 4. Define Profile Sub-Menu (Reply Keyboard with dynamic back-stack)
	bot.Menu("profile").
		Parent("main").
		Reply(). // Configures this node to render as a Reply Keyboard
		BackLabel("🔙 Back to Home").
		Text("⚙️ Your Profile Information:").
		Stretch(true).
		Row(gobale.MenuBtn("📊 Display User ID").Do(func(c *gobale.Ctx) {
			name := c.Message.From.FirstName
			uid := c.Message.From.ID
			report := fmt.Sprintf("👤 Name: %s\n🆔 User ID: %d", name, uid)
			c.Send().Text(report).Go()
		})).
		Row(gobale.MenuBtn("📂 Clouds Folder").Target("sub_profile"))

	// 5. Define Nested Profile Sub-Menu (Reply Keyboard with dynamic back-stack)
	bot.Menu("sub_profile").
		Parent("profile").
		Reply(). // Configured as a Reply Keyboard
		BackLabel("🔙 Back to Profile").
		Text("📂 Cloud Files Management:").
		Stretch(true).
		Row(gobale.MenuBtn("📝 Edit Profile").Do(func(c *gobale.Ctx) {
			c.Send().Text("📝 Edit profile module will be active soon.").Go()
		}))

	// SYSTEM ROUTERS

	// Route start command to display the Entrypoint menu
	bot.On().Cmd("start").Do(func(c *gobale.Ctx) {
		c.SendMenu("main")
	})

	log.Println("Bot is running...")
	bot.Run().Polling().Go()
}
```
