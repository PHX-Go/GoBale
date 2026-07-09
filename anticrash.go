package gobale

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Default high-risk sensitive keywords protected automatically when using "all"
var defaultSensitiveWords = []string{
	"admin", "administrator", "owner", "support", "moderator",
	"bot", "bale", "telegram", "pay", "paypal", "gift", "money",
	"coin", "token", "free", "premium", "vip", "login", "password",
}

// DetectionResult holds the details of a single triggered threat
type DetectionResult struct {
	Detected bool   // True if the rule flagged the message
	RuleName string // Name of the triggered rule
	Severity int    // Threat level from 1 (minor) to 10 (critical)
	Cleaned  string // Sanitized version of the text
	Reason   string // Human-readable explanation of the violation
	Evidence []rune // Extracted suspicious characters or tokens
}

// DetectionRule defines the plugin interface for threat detectors
type DetectionRule interface {
	Detect(text string) DetectionResult
	Name() string
}

// AntiCrashEngine orchestrates the active detection rules
type AntiCrashEngine struct {
	rules []DetectionRule
}

// ScanText evaluates all registered rules against the text and returns violations
func (e *AntiCrashEngine) ScanText(text string) []DetectionResult {
	var results []DetectionResult
	for _, rule := range e.rules {
		res := rule.Detect(text)
		if res.Detected {
			results = append(results, res)
		}
	}
	return results
}

// BUILT-IN SECURITY RULES (Multi-Dimensional Unicode Threat Shield)

// ZalgoRule: Detects combining marks, zero-width chars, RTL overrides, and foreign crash scripts
type ZalgoRule struct {
	MaxCombiningPerChar int
}

func (z *ZalgoRule) Name() string { return "ZalgoShield" }

// Detect: Pure Zalgo threat detection (combining density, enclosing symbols, invisible bombs, foreign scripts)
func (z *ZalgoRule) Detect(text string) DetectionResult {
	runes := []rune(text)
	totalRunes := len(runes)
	if totalRunes == 0 {
		return DetectionResult{Detected: false, RuleName: z.Name(), Severity: 1, Reason: "Empty text (no Zalgo threat)"}
	}

	consecutiveCombining := 0
	totalCombining := 0
	enclosingOrSymbolCombining := 0
	invisibleControlCount := 0
	foreignComplexCount := 0
	var suspiciousChars []rune

	for _, r := range runes {
		if isZeroWidth(r) || isRTLOverride(r) {
			invisibleControlCount++
			if invisibleControlCount > 4 {
				suspiciousChars = append(suspiciousChars, r)
			}
			continue
		}

		if isForeignComplexScript(r) {
			foreignComplexCount++
		}

		if isCombiningMark(r) {
			totalCombining++
			consecutiveCombining++

			if isSymbolCombiningMark(r) {
				enclosingOrSymbolCombining++
				if enclosingOrSymbolCombining > 2 {
					suspiciousChars = append(suspiciousChars, r)
				}
			}

			if consecutiveCombining > z.MaxCombiningPerChar {
				suspiciousChars = append(suspiciousChars, r)
			}
		} else {
			consecutiveCombining = 0
		}
	}

	densityRatio := float64(totalCombining) / float64(totalRunes)
	highDensity := totalCombining > 3 && densityRatio > 0.10
	foreignCrashAttempt := foreignComplexCount > 2

	detected := len(suspiciousChars) > 0 || highDensity || enclosingOrSymbolCombining > 2 || invisibleControlCount > 4 || foreignCrashAttempt

	var reason string
	severity := 1
	if highDensity {
		reason = fmt.Sprintf("High combining mark density detected (%.2f%% exceeds 10%% threshold)", densityRatio*100)
		severity = 9
	} else if enclosingOrSymbolCombining > 2 {
		reason = fmt.Sprintf("Excessive symbolic enclosing marks (%d detected, max 2 allowed) - client crash payload", enclosingOrSymbolCombining)
		severity = 10
	} else if invisibleControlCount > 4 {
		reason = fmt.Sprintf("Invisible bidi/control bomb detected (%d zero-width chars, max 4 allowed)", invisibleControlCount)
		severity = 10
	} else if foreignCrashAttempt {
		reason = fmt.Sprintf("Foreign complex script crash payload detected (%d chars from dangerous scripts)", foreignComplexCount)
		severity = 9
	} else if detected {
		reason = fmt.Sprintf("Excessive consecutive combining marks (max %d allowed)", z.MaxCombiningPerChar)
		severity = 8
	} else {
		reason = "Text passes Zalgo detection (combining marks, controls within safe limits)"
		severity = 1
	}

	cleanedRunes := make([]rune, 0, totalRunes)
	comb := 0
	encCount := 0
	for _, r := range runes {
		if isZeroWidth(r) || isRTLOverride(r) {
			continue
		}
		if isForeignComplexScript(r) {
			continue
		}
		if isCombiningMark(r) {
			comb++
			isSymbol := isSymbolCombiningMark(r)
			if isSymbol {
				encCount++
			}
			if comb <= z.MaxCombiningPerChar && (!isSymbol || encCount <= 2) {
				cleanedRunes = append(cleanedRunes, r)
			}
		} else {
			cleanedRunes = append(cleanedRunes, r)
			comb = 0
		}
	}

	return DetectionResult{
		Detected: detected,
		RuleName: z.Name(),
		Severity: severity,
		Cleaned:  string(cleanedRunes),
		Reason:   reason,
		Evidence: suspiciousChars,
	}
}

// AlternatingPatternRule: Detects low-entropy repetitive sequences (ABABAB... or Mongolian+Circle patterns)
type AlternatingPatternRule struct {
	MinLength int
	MinRatio  float64
}

func (ap *AlternatingPatternRule) Name() string { return "AlternatingPatternSpam" }

// Detect: Pure low-entropy pattern detection (character diversity analysis)
func (ap *AlternatingPatternRule) Detect(text string) DetectionResult {
	runes := []rune(text)
	totalRunes := len(runes)

	if totalRunes < ap.MinLength {
		return DetectionResult{
			Detected: false,
			RuleName: ap.Name(),
			Severity: 1,
			Reason:   fmt.Sprintf("Text length %d is below minimum %d (no pattern analysis needed)", totalRunes, ap.MinLength),
		}
	}

	uniqueRunes := make(map[rune]bool)
	countedRunes := 0

	for _, r := range runes {
		if r < 128 && (unicode.IsPunct(r) || unicode.IsSpace(r)) {
			continue
		}
		uniqueRunes[r] = true
		countedRunes++
	}

	if countedRunes < ap.MinLength {
		return DetectionResult{
			Detected: false,
			RuleName: ap.Name(),
			Severity: 1,
			Reason:   fmt.Sprintf("Counted characters %d below threshold %d (insufficient for diversity check)", countedRunes, ap.MinLength),
		}
	}

	ratio := float64(len(uniqueRunes)) / float64(countedRunes)
	detected := ratio < ap.MinRatio

	var reason string
	var severity int
	if detected {
		reason = fmt.Sprintf("Abnormally low character diversity (%.2f%% unique) - below %.2f%% threshold (spam pattern detected)", ratio*100, ap.MinRatio*100)
		severity = 8
	} else {
		reason = fmt.Sprintf("Text character diversity is healthy (%.2f%% unique, above %.2f%% threshold)", ratio*100, ap.MinRatio*100)
		severity = 1
	}

	return DetectionResult{
		Detected: detected,
		RuleName: ap.Name(),
		Severity: severity,
		Cleaned:  "",
		Reason:   reason,
		Evidence: []rune{},
	}
}

// RepeatRule: Detects excessive single-character runs (aaaaaaa, llllll, etc.)
type RepeatRule struct {
	MaxRuns int
}

func (rr *RepeatRule) Name() string { return "ExcessiveRepeat" }

// Detect: Pure character repetition detection (run-length encoding analysis)
func (rr *RepeatRule) Detect(text string) DetectionResult {
	runes := []rune(text)
	out := make([]rune, 0, len(runes))
	var last rune
	runs := 0
	var maxRunChar rune
	maxRunCount := 0

	for _, r := range runes {
		if r == last {
			runs++
			if runs > maxRunCount {
				maxRunCount = runs
				maxRunChar = r
			}
			if runs <= rr.MaxRuns {
				out = append(out, r)
			}
		} else {
			out = append(out, r)
			last = r
			runs = 1
		}
	}

	detected := runs > rr.MaxRuns

	var reason string
	var severity int
	if detected {
		reason = fmt.Sprintf("Excessive character repetition detected (max run: %d consecutive '%c', limit: %d)", maxRunCount, maxRunChar, rr.MaxRuns)
		severity = 4
	} else {
		reason = fmt.Sprintf("Character repetition within safe limits (max consecutive run: %d, limit: %d)", maxRunCount, rr.MaxRuns)
		severity = 1
	}

	return DetectionResult{
		Detected: detected,
		RuleName: rr.Name(),
		Severity: severity,
		Cleaned:  string(out),
		Reason:   reason,
		Evidence: []rune{last},
	}
}

// LengthRule: Prevents client memory exhaustion from oversized messages
type LengthRule struct {
	MaxLen int
}

func (l *LengthRule) Name() string { return "LengthLimit" }

// Detect: Pure message length validation (memory exhaustion prevention)
func (l *LengthRule) Detect(text string) DetectionResult {
	runes := []rune(text)
	detected := len(runes) > l.MaxLen
	cleaned := text
	if detected {
		cleaned = string(runes[:l.MaxLen])
	}

	var reason string
	var severity int
	if detected {
		reason = fmt.Sprintf("Message exceeds length limit (%d chars, max allowed: %d) - truncated for client stability", len(runes), l.MaxLen)
		severity = 5
	} else {
		reason = fmt.Sprintf("Message length is safe (%d chars, limit: %d)", len(runes), l.MaxLen)
		severity = 1
	}

	return DetectionResult{
		Detected: detected,
		RuleName: l.Name(),
		Severity: severity,
		Cleaned:  cleaned,
		Reason:   reason,
	}
}

// HomoglyphRule: Detects phishing via visual character substitution, IDN Punycode, mixed-script tokens
type HomoglyphRule struct {
	protectedWords []string
	confusablesMap map[rune]string
	blockMixed     bool
}

func (h *HomoglyphRule) Name() string { return "HomoglyphPhishingShield" }

// skeletonize: Maps lookalike characters to canonical forms (Cyrillic 'a' → Latin 'a')
func (h *HomoglyphRule) skeletonize(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if repl, exists := h.confusablesMap[r]; exists {
			sb.WriteString(repl)
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// Detect: Pure phishing/homoglyph detection (Punycode decode, mixed-script flagging, skeleton edit-distance)
func (h *HomoglyphRule) Detect(text string) DetectionResult {
	words := strings.Fields(strings.ToLower(text))
	var suspicious []rune
	var reasons []string
	severity := 1

	for _, word := range words {
		word = strings.Trim(word, "!?,.:;\"'()[]{}<>*#@")
		if word == "" {
			continue
		}

		// A. Decode IDN/Punycode if present
		decoded := word
		if strings.HasPrefix(word, "xn--") {
			dec, err := decodePunycode(word)
			if err == nil {
				decoded = dec
				reasons = append(reasons, fmt.Sprintf("IDN/Punycode obfuscation detected: '%s' → '%s' (phishing risk)", word, dec))
				if severity < 7 {
					severity = 7
				}
			}
		}

		// B. Detect mixed-script tokens (Latin + Cyrillic in one word = phishing red flag)
		if h.blockMixed {
			mixedScripts := detectMixedScript(decoded)
			if len(mixedScripts) > 1 {
				reasons = append(reasons, fmt.Sprintf("Mixed-script token detected: '%s' (scripts: %s) - typosquatting risk", decoded, strings.Join(mixedScripts, ", ")))
				if severity < 8 {
					severity = 8
				}
				for _, r := range decoded {
					if getScript(r) != "Latin" && getScript(r) != "Common" {
						suspicious = append(suspicious, r)
					}
				}
			}
		}

		// C. Skeleton normalization: map all lookalikes to canonical forms
		skel := h.skeletonize(decoded)

		// D. Levenshtein distance against protected keywords
		for _, protected := range h.protectedWords {
			protectedLower := strings.ToLower(protected)
			protectedSkel := h.skeletonize(protectedLower)

			variants := applyMultiCharLookalikes(skel)

			for v := range variants {
				dist := levenshtein(v, protectedSkel)

				if decoded != protectedLower && dist <= 2 {
					reasons = append(reasons, fmt.Sprintf("Brand spoofing threat: '%s' ≈ '%s' (edit distance: %d) - critical phishing attempt", decoded, protected, dist))
					if severity < 9 {
						severity = 9
					}
					for _, r := range decoded {
						if _, exists := h.confusablesMap[r]; exists {
							suspicious = append(suspicious, r)
						}
					}
				}
			}
		}
	}

	detected := len(reasons) > 0

	var reason string
	if detected {
		reason = strings.Join(reasons, " | ")
	} else {
		reason = "Text passes homoglyph/phishing detection (no Punycode, mixed-script, or brand spoofing detected)"
	}

	return DetectionResult{
		Detected: detected,
		RuleName: h.Name(),
		Severity: severity,
		Cleaned:  text,
		Reason:   reason,
		Evidence: suspicious,
	}
}

// FLUENT BUILDER PIPELINE

// AntiCrashChain: Fluent builder for configuring security rules independently
type AntiCrashChain struct {
	bot            *Bot
	maxZalgo       int
	maxRepeat      int
	maxLen         int
	useHomo        bool
	blockMixed     bool
	protectedWords []string
	warnEngine     *WarnEngine
	onViolation    func(c *Ctx, results []DetectionResult)
}

// AntiCrash: Opens fluent security configuration from Bot context
func (b *Bot) AntiCrash() *AntiCrashChain {
	return &AntiCrashChain{
		bot:        b,
		maxZalgo:   3,
		maxRepeat:  5,
		maxLen:     4096,
		useHomo:    true,
		blockMixed: false,
	}
}

// BlockMixed: Toggle active rejection of mixed-script tokens (disabled by default for tolerance)
func (a *AntiCrashChain) BlockMixed(v bool) *AntiCrashChain {
	a.blockMixed = v
	return a
}

// ZalgoLimit: Set max consecutive combining marks per base character (0 to disable)
func (a *AntiCrashChain) ZalgoLimit(limit int) *AntiCrashChain {
	a.maxZalgo = limit
	return a
}

// MaxRepeat: Set max allowed runs of identical characters (0 to disable)
func (a *AntiCrashChain) MaxRepeat(limit int) *AntiCrashChain {
	a.maxRepeat = limit
	return a
}

// MaxLength: Set maximum allowed message length in runes (0 to disable)
func (a *AntiCrashChain) MaxLength(limit int) *AntiCrashChain {
	a.maxLen = limit
	return a
}

// Homoglyph: Enable/disable homoglyph & phishing detection (enabled by default)
func (a *AntiCrashChain) Homoglyph(v bool) *AntiCrashChain {
	a.useHomo = v
	return a
}

// Protect: Register protected keywords/brands ("all" expands to default sensitive word list)
func (a *AntiCrashChain) Protect(words ...string) *AntiCrashChain {
	a.protectedWords = append(a.protectedWords, words...)
	return a
}

// WarnEngine: Inject dynamic warning/punishment engine for violations
func (a *AntiCrashChain) WarnEngine(engine *WarnEngine) *AntiCrashChain {
	a.warnEngine = engine
	return a
}

// OnViolation: Register custom callback to handle flagged threats (overrides WarnEngine)
func (a *AntiCrashChain) OnViolation(fn func(c *Ctx, results []DetectionResult)) *AntiCrashChain {
	a.onViolation = fn
	return a
}

// Go: Compile active security rules into unified high-performance middleware Handler
func (a *AntiCrashChain) Go() Handler {
	engine := &AntiCrashEngine{}

	// Register each rule independently (rules only added if enabled)
	if a.maxZalgo > 0 {
		engine.rules = append(engine.rules, &ZalgoRule{MaxCombiningPerChar: a.maxZalgo})
	}
	if a.maxRepeat > 0 {
		engine.rules = append(engine.rules, &RepeatRule{MaxRuns: a.maxRepeat})
	}
	if a.maxLen > 0 {
		engine.rules = append(engine.rules, &LengthRule{MaxLen: a.maxLen})
	}

	// Always append low-entropy pattern detection (structural threat defense)
	engine.rules = append(engine.rules, &AlternatingPatternRule{MinLength: 12, MinRatio: 0.20})

	// Expand "all" keyword to default sensitive word list for protected brands
	var finalProtected []string
	hasAll := false
	for _, w := range a.protectedWords {
		if strings.ToLower(w) == "all" {
			hasAll = true
		} else {
			finalProtected = append(finalProtected, w)
		}
	}
	if hasAll {
		finalProtected = append(finalProtected, defaultSensitiveWords...)
	}

	// Homoglyph rule: independently configured with resolved protected words
	if a.useHomo {
		engine.rules = append(engine.rules, &HomoglyphRule{
			protectedWords: finalProtected,
			blockMixed:     a.blockMixed,
			confusablesMap: map[rune]string{
				// Cyrillic homoglyphs
				'а': "a", 'А': "a", 'е': "e", 'Е': "e", 'о': "o", 'О': "o",
				'р': "p", 'Р': "p", 'с': "c", 'С': "c", 'у': "y", 'У': "y",
				'х': "x", 'Х': "x", 'і': "i", 'І': "i", 'ѕ': "s", 'Ѕ': "s",
				'ј': "j", 'Ј': "j", 'ԁ': "d", 'ԛ': "q", 'һ': "h", 'Һ': "h",
				'ԝ': "w", 'ѵ': "v", 'к': "k", 'К': "k", 'м': "m", 'М': "m",
				'н': "h", 'Н': "h", 'т': "t", 'Т': "t", 'в': "b", 'В': "b",
				'ц': "u", 'г': "r", 'ѓ': "r", 'ё': "e", 'ъ': "a", 'ы': "bl",
				'ю': "io", 'я': "r", 'ж': "x", 'з': "3", 'п': "n", 'ш': "w",
				'щ': "w", 'э': "e", 'ч': "4", 'ф': "o", 'л': "n", 'б': "6",
				'д': "d",
				// Greek homoglyphs
				'α': "a", 'Α': "a", 'β': "b", 'ο': "o", 'Ο': "o", 'ρ': "p",
				'Ρ': "p", 'ν': "v", 'Ν': "n", 'κ': "k", 'Κ': "k", 'τ': "t",
				'Τ': "t", 'χ': "x", 'Χ': "x", 'υ': "u", 'Υ': "y", 'ι': "i",
				'Ι': "i", 'η': "n", 'Η': "h", 'ε': "e", 'Ε': "e", 'γ': "y",
				// Armenian homoglyphs
				'օ': "o", 'ց': "g", 'ա': "w", 'ո': "n", 'ս': "u", 'լ': "l",
				// Latin extended homoglyphs
				'ⅼ': "l", 'ı': "i", 'ł': "l", 'ɡ': "g", 'ɑ': "a", 'ѡ': "w",
				'ⓞ': "o", 'ⓐ': "a",
				// Numeric homoglyphs
				'0': "o", '1': "l", '3': "e", '4': "a", '5': "s", '7': "t",
				'@': "a", '$': "s",
			},
		})
	}

	// Middleware handler: Scan text against all rules, report violations, optionally delete/warn
	return func(c *Ctx) {
		// Extract message safely (handles both new messages and edited messages)
		var msg *Message
		if c.Message != nil {
			msg = c.Message
		} else if c.Update != nil && c.Update.EditedMessage != nil {
			msg = c.Update.EditedMessage
		}

		if msg == nil {
			c.Next()
			return
		}

		// BYPASS: Never scan/block messages from bots (prevent self-loops)
		if msg.From != nil && msg.From.IsBot {
			c.Next()
			return
		}

		// Extract scannable text (prioritize message text, fallback to caption)
		textToScan := msg.Text
		if textToScan == "" && msg.Caption != "" {
			textToScan = msg.Caption
		}

		if textToScan == "" {
			c.Next()
			return
		}

		// Trim whitespace (including ZWNJ, braille blanks, NBSP) for exact button match
		trimmedText := strings.Trim(textToScan, " \t\n\r\u2800\u00a0")

		// FAST PATH: O(1) map lookup to skip reply button clicks (reply buttons bypass scanning)
		c.Bot.mu.RLock()
		isReplyButton := c.Bot.replyButtons[trimmedText]
		c.Bot.mu.RUnlock()

		if isReplyButton {
			c.Next()
			return
		}

		// Scan text against all enabled rules
		results := engine.ScanText(textToScan)

		// If threats detected, handle via callback or WarnEngine
		if len(results) > 0 {
			if a.onViolation != nil {
				a.onViolation(c, results)
			} else if a.warnEngine != nil {
				// Delete malicious message immediately
				_ = c.Bot.BaseRequest(c.ctx, "deleteMessage", map[string]any{
					"chat_id":    msg.Chat.ID,
					"message_id": msg.MessageID,
				}, nil)

				// Trigger WarnEngine with primary threat reason
				_ = a.warnEngine.Warn(c, results[0].Reason)
				c.Abort()
			} else {
				// Fallback: Delete message + send Persian warning
				_ = c.Bot.BaseRequest(c.ctx, "deleteMessage", map[string]any{
					"chat_id":    msg.Chat.ID,
					"message_id": msg.MessageID,
				}, nil)

				warnText := "🚨 *[سپر امنیتی]* پیام شما حاوی کاراکترهای مخرب، اسپم یا نویسه‌های غیرمجاز بود و حذف گردید."
				_, _ = c.Send().Text(warnText).Markdown().Temp(5 * time.Second).Go()
				c.Abort()
			}
			return
		}

		c.Next()
	}
}

// HIGH-PERFORMANCE SECURITY UTILITIES

// decodePunycode decodes RFC 3492 Punycode IDN domains natively without external dependencies
func decodePunycode(s string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(s), "xn--") {
		return s, nil
	}
	input := s[4:]

	const (
		base        = 36
		tmin        = 1
		tmax        = 26
		skew        = 38
		damp        = 700
		initialBias = 72
		initialN    = 128
	)

	lastDelim := strings.LastIndex(input, "-")
	var output []rune
	var encoded string
	if lastDelim >= 0 {
		output = []rune(input[:lastDelim])
		encoded = input[lastDelim+1:]
	} else {
		encoded = input
	}

	n := initialN
	i := 0
	bias := initialBias

	for pos := 0; pos < len(encoded); {
		oldi := i
		w := 1
		for k := base; ; k += base {
			if pos >= len(encoded) {
				return "", fmt.Errorf("unexpected end of punycode")
			}
			char := encoded[pos]
			pos++

			var digit int
			if char >= 'a' && char <= 'z' {
				digit = int(char - 'a')
			} else if char >= 'A' && char <= 'Z' {
				digit = int(char - 'A')
			} else if char >= '0' && char <= '9' {
				digit = int(char - '0' + 26)
			} else {
				return "", fmt.Errorf("invalid punycode character")
			}

			i += digit * w
			var t int
			if k <= bias+tmin {
				t = tmin
			} else if k >= bias+tmax {
				t = tmax
			} else {
				t = k - bias
			}
			if digit < t {
				break
			}
			w *= (base - t)
		}

		numPoints := len(output) + 1
		firstTime := oldi == 0
		delta := i - oldi
		if firstTime {
			delta /= damp
		} else {
			delta /= 2
		}
		delta += delta / numPoints
		k := 0
		for delta > ((base-tmin)*tmax)/2 {
			delta /= (base - tmin)
			k += base
		}
		bias = k + ((base-tmin+1)*delta)/(delta+skew)

		n += i / (len(output) + 1)
		i %= (len(output) + 1)

		output = append(output, 0)
		copy(output[i+1:], output[i:])
		output[i] = rune(n)
		i++
	}

	return string(output), nil
}

// scriptRanges maps Unicode script blocks for fast script identification
var scriptRanges = []struct {
	name   string
	ranges [][2]rune
}{
	{"Latin", [][2]rune{{0x0041, 0x007A}, {0x00C0, 0x024F}, {0x1E00, 0x1EFF}}},
	{"Cyrillic", [][2]rune{{0x0400, 0x04FF}, {0x0500, 0x052F}}},
	{"Greek", [][2]rune{{0x0370, 0x03FF}}},
	{"Armenian", [][2]rune{{0x0530, 0x058F}}},
	{"Hebrew", [][2]rune{{0x0590, 0x05FF}}},
	{"Arabic", [][2]rune{{0x0600, 0x06FF}}},
	{"CJK", [][2]rune{{0x4E00, 0x9FFF}, {0x3040, 0x30FF}}},
}

func getScript(r rune) string {
	if (r >= '0' && r <= '9') || r == '-' || r == '.' || unicode.IsSpace(r) {
		return "Common"
	}
	for _, sr := range scriptRanges {
		for _, rg := range sr.ranges {
			if r >= rg[0] && r <= rg[1] {
				return sr.name
			}
		}
	}
	if r < 128 {
		return "Latin"
	}
	return "Other"
}

func detectMixedScript(s string) []string {
	scripts := make(map[string]bool)
	for _, r := range s {
		scr := getScript(r)
		if scr != "Common" {
			scripts[scr] = true
		}
	}
	var list []string
	for scr := range scripts {
		list = append(list, scr)
	}
	return list
}

func applyMultiCharLookalikes(text string) map[string]bool {
	variants := map[string]bool{text: true}
	multiCharLookalikes := [][2]string{
		{"rn", "m"}, {"vv", "w"}, {"cl", "d"}, {"ii", "u"},
		{"nn", "m"}, {"lJ", "u"}, {"VV", "W"}, {"ln", "in"},
	}
	for _, pair := range multiCharLookalikes {
		pattern, repl := pair[0], pair[1]
		newVariants := make(map[string]bool)
		for v := range variants {
			if strings.Contains(v, pattern) {
				newVariants[strings.ReplaceAll(v, pattern, repl)] = true
			}
		}
		for nv := range newVariants {
			variants[nv] = true
		}
	}
	return variants
}

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	for i := 0; i <= lb; i++ {
		prev[i] = i
	}
	for i, ca := range a {
		cur := make([]int, lb+1)
		cur[0] = i + 1
		for j, cb := range b {
			cost := 1
			if ca == cb {
				cost = 0
			}
			cur[j+1] = minInt32(prev[j+1]+1, cur[j]+1, prev[j]+cost)
		}
		prev = cur
	}
	return prev[lb]
}

func minInt32(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

func isCombiningMark(r rune) bool {
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Mc, r) {
		return true
	}
	if (r >= 0x0300 && r <= 0x036F) || // Combining Diacritical Marks
		(r >= 0x1AB0 && r <= 0x1AFF) || // Combining Diacritical Marks Extended
		(r >= 0x1DC0 && r <= 0x1DFF) || // Combining Diacritical Marks Supplement
		(r >= 0x20D0 && r <= 0x20FF) || // Combining Diacritical Marks for Symbols
		(r >= 0xFE20 && r <= 0xFE2F) { // Combining Half Marks
		return true
	}
	return false
}

// isZeroWidth detects invisible Unicode control characters while excluding standard Persian ZWNJ and ZWJ
func isZeroWidth(r rune) bool {
	switch r {
	case '\u200B', '\uFEFF', '\u061C', '\u200E', '\u200F', '\u202A', '\u202B', '\u202C', '\u202D', '\u202E':
		return true
	}
	return false
}

func isRTLOverride(r rune) bool {
	switch r {
	case '\u202A', '\u202B', '\u202C', '\u202D', '\u202E':
		return true
	}
	return false
}

func isSymbolCombiningMark(r rune) bool {
	if unicode.Is(unicode.Me, r) {
		return true
	}
	// Combining Diacritical Marks for Symbols
	if r >= 0x20D0 && r <= 0x20FF {
		return true
	}
	return false
}

func isForeignComplexScript(r rune) bool {
	if (r >= 0x1800 && r <= 0x18AF) || // Mongolian
		(r >= 0x0C00 && r <= 0x0C7F) || // Telugu
		(r >= 0x0980 && r <= 0x09FF) || // Bengali
		(r >= 0x0B80 && r <= 0x0BFF) || // Tamil
		(r >= 0x0C80 && r <= 0x0CFF) || // Kannada
		(r >= 0x0D00 && r <= 0x0D7F) || // Malayalam
		(r >= 0x0D80 && r <= 0x0DFF) || // Sinhala
		(r >= 0x19E0 && r <= 0x19FF) { // Khmer symbols
		return true
	}
	return false
}

// PVClutterShield automatically deletes reply keyboard button clicks in PV to keep menus clean,
// while preserving standard text commands and conversational inputs in background safely.
// It also auto-collapses any open reply keyboards when a new text command is executed.
func (b *Bot) PVClutterShield() Handler {
	return func(c *Ctx) {
		// Only run on fresh user messages in private chats (PV)
		if c.IsPrivate() && c.Update != nil && c.Update.Message != nil && c.Message != nil && c.Message.MessageID > 0 {
			text := c.Message.Text
			trimmedText := strings.Trim(text, " \t\n\r\u2800\u00a0")

			// Perform an ultra-fast O(1) map lookup (Zero Lag!)
			c.Bot.mu.RLock()
			isReplyClick := c.Bot.replyButtons[trimmedText]
			c.Bot.mu.RUnlock()

			botInstance := c.Bot
			chatID := c.Message.Chat.ID
			msgID := c.Message.MessageID

			if isReplyClick {
				// Delete the reply button click message in the background
				c.Go(func() {
					_ = botInstance.BaseRequest(context.Background(), "deleteMessage", map[string]any{
						"chat_id":    chatID,
						"message_id": msgID,
					}, nil)
				})
			} else if strings.HasPrefix(trimmedText, "/") {
				sess := c.Session()
				hasOpenReplyMenu := false
				if currentID, ok := SessionGet[string](sess, "_current_menu"); ok && currentID != "" {
					c.Bot.mu.RLock()
					curNode, okNode := c.Bot.menus[currentID]
					c.Bot.mu.RUnlock()
					if okNode && curNode != nil && !curNode.IsInline {
						hasOpenReplyMenu = true
					}
				}

				if hasOpenReplyMenu {
					c.Go(func() {
						tempMsg, err := botInstance.Send(chatID).Text("\u200C").MarkupRemove().Context(context.Background()).Go()
						if err == nil && tempMsg != nil && tempMsg.MessageID > 0 {
							time.Sleep(150 * time.Millisecond)
							_ = botInstance.BaseRequest(context.Background(), "deleteMessage", map[string]any{
								"chat_id":    chatID,
								"message_id": tempMsg.MessageID,
							}, nil)
						}
					})
				}
			}
		}
		c.Next()
	}
}

// GroupHistoryPruner deletes the previous message of a non-admin user when they send a new one
// asynchronously to prevent edit-obfuscation bypasses safely without introducing lag.
func (b *Bot) GroupHistoryPruner() Handler {
	return func(c *Ctx) {
		// Ignore private chats, inline button clicks, service messages, or empty messages
		if c.Message == nil || c.IsPrivate() || c.Message.Text == "" || (c.Update != nil && c.Update.CallbackQuery != nil) {
			c.Next()
			return
		}

		// Bypass group administrators and owner
		isAdmin, err := c.Chat().IsAdmin().Go()
		if err == nil && isAdmin {
			c.Next()
			return
		}

		chatID, _ := c.ChatID()
		userID := c.SenderID()
		key := fmt.Sprintf("last_user_msg:%d:%d", chatID, userID)

		// Fetch and delete their previous active message asynchronously (Zero Lag!)
		if val, ok := c.DB().Get(key).Go(); ok {
			var prevMsgID int64
			if id, okInt := val.(int64); okInt {
				prevMsgID = id
			} else if id, okInt := val.(int); okInt {
				prevMsgID = int64(id)
			}

			if prevMsgID > 0 {
				// Capture safe local copies of the parameters to prevent nil-pointer dereferences after Context recycling
				botInstance := c.Bot

				// Use context.Background() to prevent the cancellation of background delete requests
				c.Go(func() {
					_ = botInstance.BaseRequest(context.Background(), "deleteMessage", map[string]any{
						"chat_id":    chatID,
						"message_id": prevMsgID,
					}, nil)
				})
			}
		}

		// Store the current message ID as the new last message reference
		_ = c.DB().Set(key, c.Message.MessageID).Go()
		c.Next()
	}
}

// PreScanUpdate performs an ultra-fast raw scan on incoming updates to intercept critical threats instantly
func (b *Bot) PreScanUpdate(u *Update) bool {
	if u == nil {
		return false
	}

	var msg *Message
	if u.Message != nil {
		msg = u.Message
	} else if u.EditedMessage != nil {
		msg = u.EditedMessage
	}

	if msg == nil {
		return false
	}

	// SECURITY BYPASS: Never scan or block messages sent by bots
	if msg.From != nil && msg.From.IsBot {
		return false
	}

	textToScan := msg.Text
	if textToScan == "" && msg.Caption != "" {
		textToScan = msg.Caption
	}

	if textToScan == "" {
		return false
	}

	runes := []rune(textToScan)
	totalCombining := 0
	enclosingCombining := 0
	foreignComplexCount := 0

	for _, r := range runes {
		// Instant block on zero-width bidi bombs
		if isZeroWidth(r) || isRTLOverride(r) {
			return true
		}
		if isForeignComplexScript(r) {
			foreignComplexCount++
		}
		if isCombiningMark(r) {
			totalCombining++
			if isSymbolCombiningMark(r) {
				enclosingCombining++
			}
		}
	}

	// Calculate combining marks density
	densityRatio := float64(totalCombining) / float64(len(runes))

	// Severe threat triggers (based on our upgraded 3D shield)
	highDensity := totalCombining > 3 && densityRatio > 0.10
	isSymbolCrash := enclosingCombining > 2
	isForeignCrash := foreignComplexCount > 2

	if highDensity || isSymbolCrash || isForeignCrash {
		return true
	}

	return false
}
