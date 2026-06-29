# Advanced Layouts, Pagination & Jalali Calendar

GoBale provides native presentation utilities designed to build clean user interfaces, paginate large keyboard menus automatically, and handle Persian date formatting with zero external dependencies.

---

## Gregorian to Jalali Calendar Converter

The `Jalali` converter provides Gregorian-to-Jalali date formatting. It calculates Persian dates natively and formats them based on your selected layout.

### Formatting Layouts
* **`.Compact()`**: Formats into a compact two-digit representation (`yy/mm/dd`).
* **`.Short()`**: Formats into the standard four-digit representation (`yyyy/mm/dd`).
* **`.Medium()`**: Formats into a human-readable day and Persian month name (`d M yyyy`).
* **`.Long()`**: Formats into a complete representation with Persian weekdays (`W d M yyyy`).

```go
bot.On().Cmd("date").Do(func(c *gobale.Ctx) {
	// Convert Gregorian time to long Persian Jalali date representation
	pDate := gobale.Jalali(time.Now()).Long().Go() // Format: W d M yyyy
	
	_, _ = c.Send().
		Text("Today is: " + pDate).
		Go()
})
```

---

## Keyboard Pagination Matrix

Displaying many options in a single keyboard causes poor user experience and scroll fatigue. GoBale solves this with `NewPaginatedKeyboard()`, which takes a flat slice of buttons and structures them into a paginated grid.

* **Dynamic Navigation Rows:** Calculates total pages and automatically appends navigating buttons ("Next" and "Prev") linked to your designated callback prefix.
* **Layout Isolation:** Keeps item rows separate from navigation rows.

```go
bot.On().Cmd("catalog").Do(func(c *gobale.Ctx) {
	// Generate 10 inline buttons dynamically using format patterns
	buttons := gobale.BtnMap("Product %d", "prod:%d", 1, 10)

	// Create a paginated inline keyboard grid (Page 1, showing 3 items per page)
	paginatedMenu := gobale.NewPaginatedKeyboard(buttons, 1, 3, "catalog_nav")

	_, _ = c.Send().
		Text("Choose a product from our catalog:").
		Markup(paginatedMenu).
		Go()
})
```

---

## Multi-Line Text Builder (`TextChain`)

Building complex, multi-line messages with variable outputs often results in messy string concatenations. GoBale's `TextChain` provides a clean, fluent string builder to compile messages with placeholder bindings.

```go
bot.On().Cmd("profile").Do(func(c *gobale.Ctx) {
	// Compile multi-line text dynamically and bind variables safely
	profileReport := gobale.Text().
		Line("👤 ", gobale.Bold("User Profile Report")).
		Line().
		Line("🔹 Name: {name}").
		Line("🔹 Account Status: ", gobale.Italic("{status}")).
		Line("📅 Last Login: {date}").
		Bind("name", c.Message.From.FirstName).
		Bind("status", "Active Member").
		Bind("date", gobale.Jalali(time.Now()).Short().Go()).
		Go()

	_, _ = c.Send().
		Text(profileReport).
		Markdown().
		Go()
})
```
