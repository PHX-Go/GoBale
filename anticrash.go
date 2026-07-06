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

// ZalgoRule detects consecutive/dense combining marks, enclosing symbols, and invisible control bombs
type ZalgoRule struct {
	MaxCombiningPerChar int
}

func (z *ZalgoRule) Name() string { return "ZalgoShield" }

// Detect evaluates combining marks, zero-width characters, RTL overrides, and foreign scripts
func (z *ZalgoRule) Detect(text string) DetectionResult {
	runes := []rune(text)
	totalRunes := len(runes)
	if totalRunes == 0 {
		return DetectionResult{Detected: false}
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
			// Strict bidi limit to prevent client lags
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
	highDensity := totalCombining > 3 && densityRatio > 0.10 // High sensitivity

	foreignCrashAttempt := foreignComplexCount > 2

	detected := len(suspiciousChars) > 0 || highDensity || enclosingOrSymbolCombining > 2 || invisibleControlCount > 4 || foreignCrashAttempt

	var reason string
	if highDensity {
		reason = fmt.Sprintf("High combining mark density detected (%.2f%%)", densityRatio*100)
	} else if enclosingOrSymbolCombining > 2 {
		reason = "Excessive symbolic enclosing combining marks detected (potential client-crash payload)"
	} else if invisibleControlCount > 4 {
		reason = "Excessive invisible bidi/control characters detected (potential invisible text bomb)"
	} else if foreignCrashAttempt {
		reason = "Foreign complex script commonly used in crash payloads detected (quarantine block)"
	} else {
		reason = "Excessive consecutive combining marks or zero-width obfuscation detected"
	}

	cleanedRunes := make([]rune, 0, totalRunes)
	comb := 0
	encCount := 0
	for _, r := range runes {
		if isZeroWidth(r) || isRTLOverride(r) {
			continue
		}
		if isForeignComplexScript(r) {
			continue // Strip foreign crash scripts entirely
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
		Severity: 10,
		Cleaned:  string(cleanedRunes),
		Reason:   reason,
		Evidence: suspiciousChars,
	}
}

// AlternatingPatternRule detects highly repetitive, low-entropy sequences (e.g. ABABAB... or Mongolian+Circle)
type AlternatingPatternRule struct {
	MinLength int
	MinRatio  float64
}

func (ap *AlternatingPatternRule) Name() string { return "AlternatingPatternSpam" }

func (ap *AlternatingPatternRule) Detect(text string) DetectionResult {
	runes := []rune(text)
	totalRunes := len(runes)

	if totalRunes < ap.MinLength {
		return DetectionResult{Detected: false}
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
		return DetectionResult{Detected: false}
	}

	ratio := float64(len(uniqueRunes)) / float64(countedRunes)
	detected := ratio < ap.MinRatio

	return DetectionResult{
		Detected: detected,
		RuleName: ap.Name(),
		Severity: 8,
		Cleaned:  "",
		Reason:   fmt.Sprintf("Abnormally low character diversity detected (%.2f%% unique characters)", ratio*100),
		Evidence: []rune{},
	}
}

// RepeatRule detects excessive single-character repetitions
type RepeatRule struct {
	MaxRuns int
}

func (rr *RepeatRule) Name() string { return "ExcessiveRepeat" }

func (rr *RepeatRule) Detect(text string) DetectionResult {
	runes := []rune(text)
	out := make([]rune, 0, len(runes))
	var last rune
	runs := 0

	for _, r := range runes {
		if r == last {
			runs++
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
	return DetectionResult{
		Detected: detected,
		RuleName: rr.Name(),
		Severity: 4,
		Cleaned:  string(out),
		Reason:   "Abnormal character repetition detected",
		Evidence: []rune{last},
	}
}

// LengthRule prevents client memory exhaustion from long messages
type LengthRule struct {
	MaxLen int
}

func (l *LengthRule) Name() string { return "LengthLimit" }

func (l *LengthRule) Detect(text string) DetectionResult {
	runes := []rune(text)
	detected := len(runes) > l.MaxLen
	cleaned := text
	if detected {
		cleaned = string(runes[:l.MaxLen])
	}
	return DetectionResult{
		Detected: detected,
		RuleName: l.Name(),
		Severity: 5,
		Cleaned:  cleaned,
		Reason:   "Message text size exceeds system limit",
	}
}

// HomoglyphRule detects phishing, typosquatting, and obfuscated keywords using skeletons
type HomoglyphRule struct {
	protectedWords []string
	confusablesMap map[rune]string
}

func (h *HomoglyphRule) Name() string { return "HomoglyphPhishingShield" }

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

// Detect inspects words to flag mixed-scripts, Punycode, or close mimics of protected keywords
func (h *HomoglyphRule) Detect(text string) DetectionResult {
	words := strings.Fields(strings.ToLower(text))
	var suspicious []rune
	var reasons []string
	severity := 0

	for _, word := range words {
		word = strings.Trim(word, "!?,.:;\"'()[]{}<>*#@")
		if word == "" {
			continue
		}

		// A. Parse Punycode if applicable
		decoded := word
		if strings.HasPrefix(word, "xn--") {
			dec, err := decodePunycode(word)
			if err == nil {
				decoded = dec
				reasons = append(reasons, fmt.Sprintf("IDN/Punycode obfuscation detected: '%s' decodes to '%s'", word, dec))
				if severity < 7 {
					severity = 7
				}
			}
		}

		// B. Mixed-script detection (e.g., Latin mixed with Cyrillic inside one token)
		mixedScripts := detectMixedScript(decoded)
		if len(mixedScripts) > 1 {
			reasons = append(reasons, fmt.Sprintf("Mixed-script token detected: '%s' (scripts: %s)", decoded, strings.Join(mixedScripts, ", ")))
			if severity < 8 {
				severity = 8
			}
			for _, r := range decoded {
				if getScript(r) != "Latin" && getScript(r) != "Common" {
					suspicious = append(suspicious, r)
				}
			}
		}

		// C. Create uniform skeleton using confusables mapping
		skel := h.skeletonize(decoded)

		// D. Edit Distance against protected sensitive keywords/brands (Blocks visual mimics like 'adm1n' safely)
		for _, protected := range h.protectedWords {
			protectedLower := strings.ToLower(protected)
			protectedSkel := h.skeletonize(protectedLower)

			// Generate visual multi-character replacement variants
			variants := applyMultiCharLookalikes(skel)

			for v := range variants {
				dist := levenshtein(v, protectedSkel)

				// Flag only if highly similar to a protected brand but not exactly the brand
				if decoded != protectedLower && dist <= 2 {
					reasons = append(reasons, fmt.Sprintf("Brand spoofing threat: '%s' is highly similar to protected keyword '%s'", decoded, protected))
					if severity < 9 {
						severity = 9 // Critical phishing threat
					}
					// Collect actual confusable characters used in the spoofed word
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
	return DetectionResult{
		Detected: detected,
		RuleName: h.Name(),
		Severity: severity,
		Cleaned:  text,
		Reason:   strings.Join(reasons, " | "),
		Evidence: suspicious,
	}
}

// FLUENT BUILDER PIPELINE

// AntiCrashChain manages fluent configuration of the dynamic shield middleware
type AntiCrashChain struct {
	bot            *Bot
	maxZalgo       int
	maxRepeat      int
	maxLen         int
	useHomo        bool
	protectedWords []string
	warnEngine     *WarnEngine
	onViolation    func(c *Ctx, results []DetectionResult)
}

// AntiCrash opens the fluent security configuration chain from Bot context
func (b *Bot) AntiCrash() *AntiCrashChain {
	return &AntiCrashChain{
		bot:       b,
		maxZalgo:  3,
		maxRepeat: 5,
		maxLen:    4096,
		useHomo:   true,
	}
}

// ZalgoLimit configures max allowed consecutive combining marks
func (a *AntiCrashChain) ZalgoLimit(limit int) *AntiCrashChain {
	a.maxZalgo = limit
	return a
}

// MaxRepeat configures max allowed runs of identical characters
func (a *AntiCrashChain) MaxRepeat(limit int) *AntiCrashChain {
	a.maxRepeat = limit
	return a
}

// MaxLength configures maximum allowed message characters
func (a *AntiCrashChain) MaxLength(limit int) *AntiCrashChain {
	a.maxLen = limit
	return a
}

// Homoglyph enables or disables visual character substitution detection
func (a *AntiCrashChain) Homoglyph(v bool) *AntiCrashChain {
	a.useHomo = v
	return a
}

// Protect registers sensitive keywords/brands or "all" keyword to safeguard against typosquatting
func (a *AntiCrashChain) Protect(words ...string) *AntiCrashChain {
	a.protectedWords = append(a.protectedWords, words...)
	return a
}

// WarnEngine injects a dynamic warning/punishment engine to handle security violations
func (a *AntiCrashChain) WarnEngine(engine *WarnEngine) *AntiCrashChain {
	a.warnEngine = engine
	return a
}

// OnViolation registers a custom callback to handle flagged security threats
func (a *AntiCrashChain) OnViolation(fn func(c *Ctx, results []DetectionResult)) *AntiCrashChain {
	a.onViolation = fn
	return a
}

// Go compiles the active security rules and returns a high-performance middleware Handler
func (a *AntiCrashChain) Go() Handler {
	engine := &AntiCrashEngine{}

	if a.maxZalgo > 0 {
		engine.rules = append(engine.rules, &ZalgoRule{MaxCombiningPerChar: a.maxZalgo})
	}
	if a.maxRepeat > 0 {
		engine.rules = append(engine.rules, &RepeatRule{MaxRuns: a.maxRepeat})
	}
	if a.maxLen > 0 {
		engine.rules = append(engine.rules, &LengthRule{MaxLen: a.maxLen})
	}
	// Always append the dynamic low-entropy pattern rule for structural threat shield
	engine.rules = append(engine.rules, &AlternatingPatternRule{MinLength: 12, MinRatio: 0.20})

	// Resolve "all" keyword to expand into default sensitive keywords list
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

	if a.useHomo {
		engine.rules = append(engine.rules, &HomoglyphRule{
			protectedWords: finalProtected,
			confusablesMap: map[rune]string{
				// Cyrillic look-alikes
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
				// Greek look-alikes
				'α': "a", 'Α': "a", 'β': "b", 'ο': "o", 'Ο': "o", 'ρ': "p",
				'Ρ': "p", 'ν': "v", 'Ν': "n", 'κ': "k", 'Κ': "k", 'τ': "t",
				'Τ': "t", 'χ': "x", 'Χ': "x", 'υ': "u", 'Υ': "y", 'ι': "i",
				'Ι': "i", 'η': "n", 'Η': "h", 'ε': "e", 'Ε': "e", 'γ': "y",
				// Armenian
				'օ': "o", 'ց': "g", 'ա': "w", 'ո': "n", 'ս': "u", 'լ': "l",
				// Latin extended
				'ⅼ': "l", 'ı': "i", 'ł': "l", 'ɡ': "g", 'ɑ': "a", 'ѡ': "w",
				'ⓞ': "o", 'ⓐ': "a",
				// Obfuscated Digits/Symbols
				'0': "o", '1': "l", '3': "e", '4': "a", '5': "s", '7': "t",
				'@': "a", '$': "s",
			},
		})
	}

	return func(c *Ctx) {
		// Capture either Message or EditedMessage safely to protect against edit bypasses
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

		// SECURITY BYPASS: Never scan or block messages sent by bots (prevent self-blocking & infinite loops)
		if msg.From != nil && msg.From.IsBot {
			c.Next()
			return
		}

		// Scan both message text and media captions to prevent bypass attempts
		textToScan := msg.Text
		if textToScan == "" && msg.Caption != "" {
			textToScan = msg.Caption
		}

		if textToScan == "" {
			c.Next()
			return
		}

		// Trim standard spaces, non-breaking spaces (\u00a0), and braille blanks (\u2800) for exact match
		trimmedText := strings.Trim(textToScan, " \t\n\r\u2800\u00a0")

		// SECURITY BYPASS: Perform a lightning-fast O(1) map lookup (Zero Lag!)
		c.Bot.mu.RLock()
		isReplyButton := c.Bot.replyButtons[trimmedText]
		c.Bot.mu.RUnlock()

		if isReplyButton {
			c.Next()
			return
		}

		results := engine.ScanText(textToScan)
		if len(results) > 0 {
			if a.onViolation != nil {
				a.onViolation(c, results)
			} else if a.warnEngine != nil {
				// Delete the malicious message/edit immediately
				_ = c.Bot.BaseRequest(c.ctx, "deleteMessage", map[string]any{
					"chat_id":    msg.Chat.ID,
					"message_id": msg.MessageID,
				}, nil)

				// Trigger WarnEngine with the exact violation reason
				_ = a.warnEngine.Warn(c, results[0].Reason)
				c.Abort()
			} else {
				// Use the correct message ID to delete (works for both new and edited messages)
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

func isZeroWidth(r rune) bool {
	switch r {
	case '\u200B', '\u200C', '\u200D', '\uFEFF', '\u061C', '\u200E', '\u200F', '\u202A', '\u202B', '\u202C', '\u202D', '\u202E':
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
func (b *Bot) PVClutterShield() Handler {
	return func(c *Ctx) {
		// Only run on fresh user messages in private chats (PV)
		if c.IsPrivate() && c.Update != nil && c.Update.Message != nil && c.Message != nil && c.Message.MessageID > 0 {
			// Trim standard spaces and invisible braille blanks for exact match
			trimmedText := strings.Trim(c.Message.Text, " \t\n\r\u2800\u00a0")

			// Perform an ultra-fast O(1) map lookup (Zero Lag!)
			c.Bot.mu.RLock()
			isReplyClick := c.Bot.replyButtons[trimmedText]
			c.Bot.mu.RUnlock()

			if isReplyClick {
				// Capture safe local copies of the parameters to prevent nil-pointer dereferences after Context recycling
				botInstance := c.Bot
				chatID := c.Message.Chat.ID
				msgID := c.Message.MessageID

				// Use context.Background() to prevent the cancellation of background delete requests
				c.Go(func() {
					_ = botInstance.BaseRequest(context.Background(), "deleteMessage", map[string]any{
						"chat_id":    chatID,
						"message_id": msgID,
					}, nil)
				})
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
