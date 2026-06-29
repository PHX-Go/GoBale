# Conversational Flows & FSM Reflection Mapping

GoBale features an advanced Finite State Machine (FSM) that simplifies multi-step user conversations. It supports sequential step-by-step guided flows (`WizardChain`), automated structural form mapping utilizing Go reflection (`FormChain`), and automated bot verification overlays (`CaptchaChain`) [GoBale_v3.txt].

---

## Conversational Wizards (`WizardChain`)

The `WizardChain` guides users sequentially through a series of questions. 
* **State Isolation:** Session states are registered on the fly using unique random tokens to prevent race conditions.
* **Auto-Cleanup:** Inactive states are automatically cleared from the memory after 15 minutes to prevent memory leaks if a user abandons the conversation [GoBale_v3.txt].

```go
bot.On().Cmd("survey").Do(func(c *gobale.Ctx) {
	// Initialize and start a conversational wizard
	c.NewWizard().
		Step("What is your age?", func(activeCtx *gobale.Ctx) {
			_ = activeCtx.Session().Data("age", activeCtx.Text()).Go()
		}).
		StepWithValidation("What is your phone (09xxxxxxxxx)?",
			func(text string) bool {
				_, ok := gobale.NormalizePhone(text)
				return ok
			},
			"⚠️ Invalid phone number format! Try again:",
			func(activeCtx *gobale.Ctx) {
				_ = activeCtx.Session().Data("phone", activeCtx.Text()).Go()
			},
		).
		OnComplete(func(activeCtx *gobale.Ctx) {
			// Retrieve captured variables safely upon completion
			age, _ := activeCtx.Session().Data("age").Go()
			phone, _ := activeCtx.Session().Data("phone").Go()

			_, _ = activeCtx.Send().
				Text(fmt.Sprintf("Survey Complete!\nAge: %v\nPhone: %v", age, phone)).
				Go()
		}).
		Go()
})
```

---

## Reflective Struct Forms (`FormChain`)

Instead of writing verbose step-by-step wizard logic, you can map conversation states directly to a custom Go struct. GoBale uses reflection to read field tags (`prompt`, `validate`, `error`), automatically compile the wizard prompts, validate inputs, and cast types safely [GoBale_v3.txt].

### Supported Field Tags:
* `prompt`: The text message sent to the user requesting input [GoBale_v3.txt].
* `validate`: Automated validation rule. Supported: `"phone"` (Iranian phone format), `"nationalcode"` (Iranian national code validation), and `"numeric"` [GoBale_v3.txt].
* `error`: Custom error message dispatched to the user upon validation failure [GoBale_v3.txt].

```go
type UserRegistration struct {
	Name string `prompt:"Please enter your name:"`
	Age  int    `prompt:"Please enter your age:" validate:"numeric" error:"⚠️ Age must be a valid number!"`
}

bot.On().Cmd("register").Do(func(c *gobale.Ctx) {
	var form UserRegistration

	// Automatically map conversational steps to the struct fields using reflection
	c.Form(&form).OnComplete(func(activeCtx *gobale.Ctx) {
		// Fields are automatically populated and type-cast upon completion
		_, _ = activeCtx.Send().
			Text(fmt.Sprintf("Profile Saved!\nName: %s\nAge: %d", form.Name, form.Age)).
			Go()
	}).Go()
})
```

---

## Automated Verification Captchas (`CaptchaChain`)

`CaptchaChain` provides a fully automated self-bot verification loop to guard group chats. It mutes new members, generates a verification captcha overlay, and kicks unverified users automatically upon timeout [GoBale_v3.txt].

```go
func SetupCaptcha(bot *gobale.Bot) {
	// Register a secure group joining verification loop
	bot.Captcha().
		Timeout(60 * time.Second).
		Prompt("👋 {name}, click the button below to verify your identity:").
		Button("✅ I am human").
		KickMsg("⛔ User {name} was kicked for failing captcha verification.").
		Go()
}
```
