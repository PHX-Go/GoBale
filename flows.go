package gobale

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WizardStep represents a single question in conversational flows with optional validation rules
type WizardStep struct {
	Prompt     string
	Validation func(string) bool
	OnError    string
	Handler    func(*Ctx)
}

// WizardChain manages multi-step conversation flows fluently
type WizardChain struct {
	c      *Ctx
	steps  []WizardStep
	finish func(*Ctx)
}

// Wizard opens the fluent conversational flow builder
func (c *Ctx) Wizard() *Ctx {
	// Returns a direct Ctx for helper integration, but let's keep the chain builder:
	return c
}

// NewWizard opens the fluent conversational flow builder
func (c *Ctx) NewWizard() *WizardChain {
	return &WizardChain{c: c}
}

// Step registers a single conversational prompt step
func (w *WizardChain) Step(prompt string, h func(*Ctx)) *WizardChain {
	w.steps = append(w.steps, WizardStep{Prompt: prompt, Handler: h})
	return w
}

// StepWithValidation registers a single conversational step with strict validation rules
func (w *WizardChain) StepWithValidation(prompt string, validator func(string) bool, onError string, h func(*Ctx)) *WizardChain {
	w.steps = append(w.steps, WizardStep{
		Prompt:     prompt,
		Validation: validator,
		OnError:    onError,
		Handler:    h,
	})
	return w
}

// OnComplete registers the final completion closure
func (w *WizardChain) OnComplete(h func(*Ctx)) *WizardChain {
	w.finish = h
	return w
}

// Go runs the conversational wizard step-by-step
func (w *WizardChain) Go() {
	w.runStep(w.c, 0)
}

// runStep coordinates step transitions and handles automatic step-retries on validation failure
func (w *WizardChain) runStep(c *Ctx, idx int) {
	if idx >= len(w.steps) {
		if w.finish != nil {
			w.finish(c)
		}
		return
	}
	step := w.steps[idx]
	_, _ = c.Send().Text(step.Prompt).Go()

	// Generate unique state name using a random token to prevent timer race conditions
	token, _ := Token(4)
	stateName := fmt.Sprintf("_wizard_state_%d_%d_%s", c.SenderID(), idx, token)

	c.Bot.mu.RLock()
	_, alreadyRegistered := c.Bot.states[stateName]
	c.Bot.mu.RUnlock()
	if alreadyRegistered {
		_, _ = c.Session().State(stateName).Go()
		return
	}

	// Schedule cleanup only for this specific unique state token
	c.Bot.Task().In(15*time.Minute, func() {
		c.Bot.mu.Lock()
		delete(c.Bot.states, stateName)
		c.Bot.mu.Unlock()
		s, _ := c.Session().State().Go()
		if s == stateName {
			_, _ = c.Session().State("").Go()
		}
	})

	c.Bot.On().State(stateName).Do(func(activeCtx *Ctx) {
		activeCtx.Bot.mu.Lock()
		delete(activeCtx.Bot.states, stateName)
		activeCtx.Bot.mu.Unlock()

		if step.Validation != nil && !step.Validation(activeCtx.Text()) {
			warn := step.OnError
			if warn == "" {
				warn = "⚠️ مقدار وارد شده نامعتبر است. لطفاً مجدداً ارسال کنید."
			}
			_, _ = activeCtx.Send().Text(warn).Temp(5 * time.Second).Go()
			w.runStep(activeCtx, idx)
			return
		}

		_, _ = activeCtx.Session().State("").Go()
		step.Handler(activeCtx)
		w.runStep(activeCtx, idx+1)
	})

	_, _ = c.Session().State(stateName).Go()
}

// FormChain dynamically populates structs using reflections, custom tags and automated validations
type FormChain struct {
	c      *Ctx
	form   any
	finish func(*Ctx)
}

// Form initiates a dynamic reflections based form filling sequence
func (c *Ctx) Form(form any) *FormChain {
	return &FormChain{c: c, form: form}
}

// OnComplete registers final success handler
func (f *FormChain) OnComplete(h func(*Ctx)) *FormChain {
	f.finish = h
	return f
}

// Go parses struct tags, registers automated validation callbacks, and starts conversational prompts
func (f *FormChain) Go() {
	val := reflect.ValueOf(f.form).Elem()
	t := val.Type()
	numFields := val.NumField()
	w := f.c.NewWizard()
	for i := 0; i < numFields; i++ {
		sf := t.Field(i)
		prompt := sf.Tag.Get("prompt")
		if prompt == "" {
			continue
		}
		validationType := sf.Tag.Get("validate")
		errorMsg := sf.Tag.Get("error")
		fieldName := sf.Name

		// Automated validation mapping based on struct tags
		var validator func(string) bool
		switch validationType {
		case "phone":
			validator = func(text string) bool {
				_, ok := NormalizePhone(text)
				return ok
			}
		case "nationalcode":
			validator = func(text string) bool {
				return ValidateNationalCode(text)
			}
		case "numeric":
			validator = func(text string) bool {
				_, err := strconv.Atoi(ToEnDigits(text))
				return err == nil
			}
		}

		stepHandler := func(c *Ctx) {
			_, _ = c.Session().Data("form_field_"+fieldName, c.Text()).Go()
		}

		if validator != nil {
			w.StepWithValidation(prompt, validator, errorMsg, stepHandler)
		} else {
			w.Step(prompt, stepHandler)
		}
	}
	w.OnComplete(func(c *Ctx) {
		for i := 0; i < numFields; i++ {
			sf := t.Field(i)
			fVal := val.Field(i)
			fieldName := sf.Name

			cachedVal, err := c.Session().Data("form_field_" + fieldName).Go()
			if err == nil && cachedVal != nil {
				strVal, ok := cachedVal.(string)
				if !ok {
					continue
				}
				switch fVal.Kind() {
				case reflect.String:
					// If it is a phone number, save the standardized 09xxxxxxxxx version instead of raw
					if sf.Tag.Get("validate") == "phone" {
						phone, _ := NormalizePhone(strVal)
						fVal.SetString(phone)
					} else {
						fVal.SetString(strVal)
					}
				case reflect.Int:
					intVal, _ := strconv.Atoi(strVal)
					fVal.SetInt(int64(intVal))
				case reflect.Int64:
					intVal, _ := strconv.ParseInt(strVal, 10, 64)
					fVal.SetInt(intVal)
				}
			}
		}
		if f.finish != nil {
			f.finish(c)
		}
	})
	w.Go()
}

// SurveyStep holds a single question with its choices
type SurveyStep struct {
	Question string
	Choices  []string
}

// SurveyChain manages multi-step inline keyboard surveys
type SurveyChain struct {
	c       *Ctx
	steps   []SurveyStep
	answers []string
	finish  func(*Ctx, []string)
}

// NewSurvey opens the fluent survey builder from Handler context
func (c *Ctx) Survey() *SurveyChain {
	return &SurveyChain{c: c}
}

// Q registers a single question step with its answer choices
func (s *SurveyChain) Q(question string, choices []string) *SurveyChain {
	s.steps = append(s.steps, SurveyStep{
		Question: question,
		Choices:  choices,
	})
	return s
}

// OnComplete registers the completion handler receiving all answers in order
func (s *SurveyChain) OnComplete(fn func(*Ctx, []string)) *SurveyChain {
	s.finish = fn
	return s
}

// Go starts the survey from step zero
func (s *SurveyChain) Go() {
	s.runStep(s.c, 0, []string{})
}

// runStep sends a question (or edits the existing one in-place) and registers its callback handler
func (s *SurveyChain) runStep(c *Ctx, idx int, answers []string) {
	// Check if all steps are completed
	if idx >= len(s.steps) {
		// Delete the final survey question message before executing finish callback
		_ = c.Del().Go()
		if s.finish != nil {
			s.finish(c, answers)
		}
		return
	}

	step := s.steps[idx]
	senderID := c.SenderID()
	callbackPrefix := fmt.Sprintf("_survey_%d_%d_%d", senderID, idx, len(s.steps))

	builder := InlineMarkup()
	for i, choice := range step.Choices {
		cb := fmt.Sprintf("%s:%d", callbackPrefix, i)
		builder.Row(Btn(choice).Callback(cb))
	}

	// Edit the message in-place for step > 0 to eliminate menu jumping/flickering
	if idx == 0 {
		_, _ = c.Send().Text(step.Question).Markup(builder.Build()).Go()
	} else {
		_, _ = c.Edit().Text(step.Question).Markup(builder.Build()).Go()
	}

	// Store current answers in session to survive the callback round-trip
	answerKey := fmt.Sprintf("_survey_answers_%d", senderID)
	_, _ = c.Session().Data(answerKey, strings.Join(answers, "||")).Go()

	// Schedule background cleanup to prevent memory leaks if abandoned
	c.Bot.Task().In(15*time.Minute, func() {
		c.Bot.mu.Lock()
		delete(c.Bot.callbacks, callbackPrefix)
		c.Bot.mu.Unlock()
	})

	c.Bot.On().Callback(callbackPrefix).Do(func(activeCtx *Ctx) {
		// Clean up this step's callback handler
		activeCtx.Bot.mu.Lock()
		delete(activeCtx.Bot.callbacks, callbackPrefix)
		activeCtx.Bot.mu.Unlock()

		// Parse which choice was picked
		var choiceIdx int
		fmt.Sscanf(
			activeCtx.Update.CallbackQuery.Data,
			callbackPrefix+":%d",
			&choiceIdx,
		)

		choiceText := step.Choices[0]
		if choiceIdx >= 0 && choiceIdx < len(step.Choices) {
			choiceText = step.Choices[choiceIdx]
		}

		// Load previous answers from session
		raw, _ := activeCtx.Session().Data(answerKey).Go()
		var prevAnswers []string
		if rawStr, ok := raw.(string); ok && rawStr != "" {
			prevAnswers = strings.Split(rawStr, "||")
		}
		newAnswers := append(prevAnswers, choiceText)

		// Acknowledge callback query to stop loading spinner (Removed the old activeCtx.Del().Go()!)
		_ = activeCtx.Answer().Go()

		s.runStep(activeCtx, idx+1, newAnswers)
	})
}

// captchaEntry holds pending verification state for a single user
type captchaEntry struct {
	mu        sync.Mutex
	messageID int64
	chatID    int64
	timer     *time.Timer
	done      bool
}

// CaptchaChain manages fluent captcha verification configuration
type CaptchaChain struct {
	bot     *Bot
	timeout time.Duration
	prompt  string
	kickMsg string
	button  string
	onPass  Handler
	onFail  Handler
	pending sync.Map // key: "chatID_userID" -> *captchaEntry
}

// Captcha opens the fluent captcha dot system from Bot context
func (b *Bot) Captcha() *CaptchaChain {
	return &CaptchaChain{
		bot:     b,
		timeout: 60 * time.Second,
		prompt:  "👋 {name} عزیز، برای تایید هویت روی دکمه زیر کلیک کنید.",
		kickMsg: "⛔ کاربر {name} به دلیل عدم تایید هویت از گروه حذف شد.",
		button:  "✅ منم، ربات نیستم!",
	}
}

// Timeout sets the verification deadline duration
func (cc *CaptchaChain) Timeout(d time.Duration) *CaptchaChain {
	cc.timeout = d
	return cc
}

// Prompt sets the welcome verification message text
// از {name} برای نام کاربر استفاده کن
func (cc *CaptchaChain) Prompt(text string) *CaptchaChain {
	cc.prompt = text
	return cc
}

// KickMsg sets the message sent after kicking an unverified user
func (cc *CaptchaChain) KickMsg(text string) *CaptchaChain {
	cc.kickMsg = text
	return cc
}

// Button sets the verification button label text
func (cc *CaptchaChain) Button(label string) *CaptchaChain {
	cc.button = label
	return cc
}

// OnPass registers a handler to execute after successful verification
func (cc *CaptchaChain) OnPass(h Handler) *CaptchaChain {
	cc.onPass = h
	return cc
}

// OnFail registers a handler to execute after verification timeout and kick
func (cc *CaptchaChain) OnFail(h Handler) *CaptchaChain {
	cc.onFail = h
	return cc
}

// Go registers the captcha join handler into the bot routing system safely without pool reference leaks
func (cc *CaptchaChain) Go() {
	callbackPrefix := "_captcha_verify"

	cc.bot.On().Join().Do(func(c *Ctx) {
		for _, user := range c.Message.NewChatMembers {
			if user.IsBot {
				continue
			}

			chatID, err := c.ChatID()
			if err != nil {
				continue
			}

			userID := user.ID
			callbackData := fmt.Sprintf("%s:%d:%d", callbackPrefix, chatID, userID)

			// use unique captcha_mute prefix to prevent conflict with admin mute
			muteKey := fmt.Sprintf("captcha_mute_%d_%d", chatID, userID)
			_ = c.DB().Set(muteKey, true).Go()

			name := user.Mention()
			text := replaceVars(cc.prompt, map[string]string{"name": name})
			markup := InlineMarkup().
				Row(Btn(cc.button).Callback(callbackData)).
				Build()

			msg, err := c.Bot.Send(chatID).Text(text).Markup(markup).Markdown().Go()
			if err != nil {
				continue
			}

			captchaMsgKey := fmt.Sprintf("captcha_msg_%d_%d", chatID, userID)
			_ = c.DB().Set(captchaMsgKey, msg.MessageID).Go()

			// Capture safe local variables to avoid long-running sync.Pool references
			botInstance := c.Bot
			dbInstance := c.Bot.dbInstance
			msgID := msg.MessageID
			userName := name
			onFailHandler := cc.onFail

			time.AfterFunc(cc.timeout, func() {
				val, ok := dbInstance.Get(muteKey)
				if !ok {
					return
				}
				if isMuted, ok := val.(bool); !ok || !isMuted {
					return
				}

				_ = botInstance.BaseRequest(context.Background(), "deleteMessage", map[string]any{
					"chat_id":    chatID,
					"message_id": msgID,
				}, nil)

				_ = botInstance.Chat(chatID).Ban(userID).Go()
				_ = botInstance.Chat(chatID).Unban(userID).OnlyIfBanned(true).Go()

				_ = dbInstance.Del(muteKey)
				_ = dbInstance.Del(captchaMsgKey)

				kickText := replaceVars(cc.kickMsg, map[string]string{"name": userName})
				kickMsg, _ := botInstance.Send(chatID).Text(kickText).Go()
				if kickMsg != nil {
					kickMsgID := kickMsg.MessageID
					botInstance.Task().In(5*time.Second, func() {
						_ = botInstance.BaseRequest(context.Background(), "deleteMessage", map[string]any{
							"chat_id":    chatID,
							"message_id": kickMsgID,
						}, nil)
					})
				}

				if onFailHandler != nil {
					fc := &Ctx{Bot: botInstance, ctx: context.Background()}
					onFailHandler(fc)
				}
			})
		}
	}).Go()

	cc.bot.On().Callback(callbackPrefix).Do(func(c *Ctx) {
		if c.Update == nil || c.Update.CallbackQuery == nil {
			return
		}

		parts := strings.Split(c.Update.CallbackQuery.Data, ":")
		if len(parts) < 3 {
			return
		}

		chatID, err1 := strconv.ParseInt(parts[1], 10, 64)
		userID, err2 := strconv.ParseInt(parts[2], 10, 64)
		if err1 != nil || err2 != nil {
			return
		}

		if c.Update.CallbackQuery.From.ID != userID {
			_ = c.Answer().Text("این دکمه مخصوص شما نیست!").Alert().Go()
			return
		}

		// use unique captcha_mute prefix to verify and delete the captcha key
		muteKey := fmt.Sprintf("captcha_mute_%d_%d", chatID, userID)
		_, okMute := c.DB().Get(muteKey).Go()

		if !okMute {
			_ = c.Answer().Text("هویت شما قبلاً تایید شده است.").Go()
			return
		}

		_ = c.DB().Del(muteKey).Go()
		captchaMsgKey := fmt.Sprintf("captcha_msg_%d_%d", chatID, userID)
		valMsg, okMsg := c.DB().Get(captchaMsgKey).Go()
		_ = c.DB().Del(captchaMsgKey).Go()

		var captchaMsgID int64
		if okMsg {
			if id, ok := valMsg.(int64); ok {
				captchaMsgID = id
			} else if id, ok := valMsg.(int); ok {
				captchaMsgID = int64(id)
			}
		}

		if captchaMsgID > 0 {
			_ = c.Bot.BaseRequest(c.ctx, "deleteMessage", map[string]any{
				"chat_id":    chatID,
				"message_id": captchaMsgID,
			}, nil)
		}

		_ = c.Answer().Text("✅ تایید شدید! خوش آمدید.").Go()

		if cc.onPass != nil {
			cc.onPass(c)
		}
	})
}

// replaceVars replaces {key} placeholders in text with given values
func replaceVars(text string, vars map[string]string) string {
	for k, v := range vars {
		text = strings.ReplaceAll(text, "{"+k+"}", v)
	}
	return text
}
