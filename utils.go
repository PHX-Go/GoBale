package gobale

import (
	"bufio"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// rxPhone compiles a fast regex to validate Iranian mobile phone number structures
var rxPhone = regexp.MustCompile(`^(0|98|\+98|0098)?(9\d{9})$`)

// Bold formats input string into Markdown bold style
func Bold(t string) string {
	return fmt.Sprintf(" *%s* ", t)
}

// Italic formats input string into Markdown italic style
func Italic(t string) string {
	return fmt.Sprintf(" _%s_ ", t)
}

// Link generates Markdown hyperlink text
func Link(t, u string) string {
	return fmt.Sprintf("[%s](%s)", t, u)
}

// Tooltip generates a monospace formatting with description
func Tooltip(t, d string) string {
	return fmt.Sprintf("```[%s]%s```", t, d)
}

// Money formats numeric amount into currency separated string
func Money(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var res []string
	for len(s) > 3 {
		res = append([]string{s[len(s)-3:]}, res...)
		s = s[:len(s)-3]
	}
	res = append([]string{s}, res...)
	out := strings.Join(res, ",")
	if neg {
		return "-" + out
	}
	return out
}

// ToEnDigits replaces Persian and Arabic numerals with English equivalents safely
func ToEnDigits(s string) string {
	repl := strings.NewReplacer(
		"۰", "0", "۱", "1", "۲", "2", "۳", "3", "۴", "4",
		"۵", "5", "۶", "6", "۷", "7", "۸", "8", "۹", "9",
		"٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4",
		"٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9",
	)
	return repl.Replace(s)
}

// OTP generates secure crypto random numerical OTP codes
func OTP(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("invalid length")
	}
	var sb strings.Builder
	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		sb.WriteString(num.String())
	}
	return sb.String(), nil
}

// Token generates a secure hexadecimal cryptographically random token string
func Token(bytesCount int) (string, error) {
	b := make([]byte, bytesCount)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SecureCompare compares two string inputs in constant time
func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// ParseDuration converts text duration including days and weeks into time.Duration
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if strings.HasSuffix(s, "d") {
		dStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(dStr)
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	if strings.HasSuffix(s, "w") {
		wStr := strings.TrimSuffix(s, "w")
		weeks, err := strconv.Atoi(wStr)
		if err != nil {
			return 0, err
		}
		return time.Duration(weeks) * 7 * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// GetEnv reads system environment variable casting into given generic type
func GetEnv[T any](k string) T {
	val := os.Getenv(k)
	var res T
	switch ptr := any(&res).(type) {
	case *string:
		*ptr = val
	case *int:
		if v, err := strconv.Atoi(val); err == nil {
			*ptr = v
		}
	case *int64:
		if v, err := strconv.ParseInt(val, 10, 64); err == nil {
			*ptr = v
		}
	case *bool:
		if v, err := strconv.ParseBool(val); err == nil {
			*ptr = v
		}
	case *time.Duration:
		if v, err := ParseDuration(val); err == nil {
			*ptr = v
		}
	}
	return res
}

// JalaliChain provides a fluent API for Jalali date formatting
type JalaliChain struct {
	t      time.Time
	layout string
}

// Jalali opens the fluent Jalali calendar converter dot system
func Jalali(t time.Time) *JalaliChain {
	return &JalaliChain{
		t:      t,
		layout: "yyyy/mm/dd",
	}
}

// Compact configures layout to two-digit format (yy/mm/dd)
func (j *JalaliChain) Compact() *JalaliChain {
	j.layout = "yy/mm/dd"
	return j
}

// Short configures layout to standard four-digit format (yyyy/mm/dd)
func (j *JalaliChain) Short() *JalaliChain {
	j.layout = "yyyy/mm/dd"
	return j
}

// Medium configures layout to human readable day and month name (d M yyyy)
func (j *JalaliChain) Medium() *JalaliChain {
	j.layout = "d M yyyy"
	return j
}

// Long configures layout to complete format with weekday names (W d M yyyy)
func (j *JalaliChain) Long() *JalaliChain {
	j.layout = "W d M yyyy"
	return j
}

// Format registers custom layout for Jalali date conversion
func (j *JalaliChain) Format(layout string) *JalaliChain {
	j.layout = layout
	return j
}

// Go executes Gregorian to Jalali conversion with selected layout
func (j *JalaliChain) Go() string {
	gDays := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	jDays := []int{31, 31, 31, 31, 31, 31, 30, 30, 30, 30, 30, 29}
	gy := j.t.Year() - 1600
	gm := int(j.t.Month()) - 1
	gd := j.t.Day() - 1
	gDayNo := 365*gy + (gy+3)/4 - (gy+99)/100 + (gy+399)/400
	for i := 0; i < gm; i++ {
		gDayNo += gDays[i]
	}
	if gm > 1 && ((j.t.Year()%4 == 0 && j.t.Year()%100 != 0) || j.t.Year()%400 == 0) {
		gDayNo++
	}
	gDayNo += gd
	jDayNo := gDayNo - 79
	jNp := jDayNo / 12053
	jDayNo %= 12053
	jy := 979 + 33*jNp + 4*(jDayNo/1461)
	jDayNo %= 1461
	if jDayNo >= 366 {
		jy += (jDayNo - 1) / 365
		jDayNo = (jDayNo - 1) % 365
	}
	var i int
	for i = 0; i < 11 && jDayNo >= jDays[i]; i++ {
		jDayNo -= jDays[i]
	}
	jm := i + 1
	jd := jDayNo + 1
	persianMonths := []string{
		"فروردین", "اردیبهشت", "خرداد", "تیر", "مرداد", "شهریور",
		"مهر", "آبان", "آذر", "دی", "بهمن", "اسفند",
	}
	persianWeekdays := map[time.Weekday]string{
		time.Saturday:  "شنبه",
		time.Sunday:    "یکشنبه",
		time.Monday:    "دوشنبه",
		time.Tuesday:   "سه‌شنبه",
		time.Wednesday: "چهارشنبه",
		time.Thursday:  "پنجشنبه",
		time.Friday:    "جمعه",
	}
	monthName := persianMonths[jm-1]
	weekdayName := persianWeekdays[j.t.Weekday()]
	res := j.layout
	res = strings.ReplaceAll(res, "yyyy", fmt.Sprintf("%04d", jy))
	res = strings.ReplaceAll(res, "yy", fmt.Sprintf("%02d", jy%100))
	res = strings.ReplaceAll(res, "mm", fmt.Sprintf("%02d", jm))
	res = strings.ReplaceAll(res, "m", fmt.Sprintf("%d", jm))
	res = strings.ReplaceAll(res, "dd", fmt.Sprintf("%02d", jd))
	res = strings.ReplaceAll(res, "d", fmt.Sprintf("%d", jd))
	res = strings.ReplaceAll(res, "M", monthName)
	res = strings.ReplaceAll(res, "W", weekdayName)
	return ToEnDigits(res)
}

// TextChain implements unified fluent formatted string builders ending with Go
type TextChain struct {
	sb   strings.Builder
	vars map[string]string
}

// Text opens fluent TextChain utility builder
func Text() *TextChain {
	return &TextChain{}
}

// Line appends a formatted string sequence directly into builders
func (t *TextChain) Line(parts ...string) *TextChain {
	for _, p := range parts {
		t.sb.WriteString(p)
	}
	t.sb.WriteString("\n")
	return t
}

// Bind registers a dynamic variable key-value pair to replace inside templates
func (t *TextChain) Bind(k string, v any) *TextChain {
	if t.vars == nil {
		t.vars = make(map[string]string)
	}
	t.vars[k] = fmt.Sprintf("%v", v)
	return t
}

// Go executes and compiles the finalized text layout replacing bound variables
func (t *TextChain) Go() string {
	res := t.sb.String()
	for k, v := range t.vars {
		res = strings.ReplaceAll(res, "{"+k+"}", v)
	}
	return res
}

// EnvChain manages environment variables loading
type EnvChain struct {
	path string
}

// Env opens the fluent environment config chain
func Env() *EnvChain {
	return &EnvChain{path: ".env"}
}

// Path registers custom file path for .env loading
func (e *EnvChain) Path(p string) *EnvChain {
	e.path = p
	return e
}

// Go executes the .env file loading and registers keys into system env
func (e *EnvChain) Go() error {
	file, err := os.Open(e.path)
	if err != nil {
		return fmt.Errorf("failed to open env file: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			_ = os.Setenv(k, v)
		}
	}
	return scanner.Err()
}

// NormalizeLetters normalizes Arabic characters (like ي and ك) to Persian equivalents (ی and ک)
func NormalizeLetters(s string) string {
	repl := strings.NewReplacer(
		"ي", "ی",
		"ك", "ک",
		"ۀ", "ه",
		"ة", "ه",
	)
	return repl.Replace(s)
}

// NormalizePhone validates and normalizes Iranian mobile numbers to the standard 09xxxxxxxxx format
func NormalizePhone(s string) (string, bool) {
	s = ToEnDigits(strings.TrimSpace(s))
	matches := rxPhone.FindStringSubmatch(s)
	if len(matches) < 3 {
		return "", false
	}
	return "0" + matches[2], true
}

// ValidateNationalCode verifies Iranian National Code (کد ملی) rejecting identical repetitions
func ValidateNationalCode(code string) bool {
	code = ToEnDigits(strings.TrimSpace(code))
	if len(code) != 10 {
		return false
	}
	if _, err := strconv.Atoi(code); err != nil {
		return false
	}

	// Reject mock codes with completely identical repetitive digits
	isIdentical := true
	for i := 1; i < 10; i++ {
		if code[i] != code[0] {
			isIdentical = false
			break
		}
	}
	if isIdentical {
		return false
	}

	sum := 0
	for i := 0; i < 9; i++ {
		sum += int(code[i]-'0') * (10 - i)
	}
	rem := sum % 11
	ctrl := int(code[9] - '0')
	if rem < 2 {
		return ctrl == rem
	}
	return ctrl == 11-rem
}

// FormatBytes converts raw byte sizes into human-readable strings (e.g., MB, KB)
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// NormalizeSafirPhone formats any Iranian mobile number to the strict Safir-compliant 989xxxxxxxxx format
func NormalizeSafirPhone(s string) (string, bool) {
	phone, ok := NormalizePhone(s)
	if !ok {
		return "", false
	}
	// Replaces leading 0 with 98 compliant format
	return "98" + phone[1:], true
}

// WebappChain handles fluent validation and parsing of Bale Mini App initData
type WebappChain struct {
	bot      *Bot
	initData string
	expire   time.Duration
}

// Webapp opens the fluent Mini App configuration dot system from Bot context
func (b *Bot) Webapp() *WebappChain {
	return &WebappChain{
		bot:    b,
		expire: 24 * time.Hour, // Default expiration duration threshold
	}
}

// Verify registers the raw WebApp initData string to be validated
func (w *WebappChain) Verify(initData string) *WebappChain {
	w.initData = initData
	return w
}

// Expire configures the maximum allowed age of the authentication data (prevent replay attacks)
func (w *WebappChain) Expire(d time.Duration) *WebappChain {
	w.expire = d
	return w
}

// Go executes the HMAC-SHA-256 validation and checks auth_date expiration
func (w *WebappChain) Go() (bool, error) {
	if w.initData == "" {
		return false, errors.New("empty initData")
	}
	params, err := url.ParseQuery(w.initData)
	if err != nil {
		return false, err
	}
	hash := params.Get("hash")
	if hash == "" {
		return false, errors.New("missing hash in initData")
	}
	var keys []string
	for k := range params {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var checkArr []string
	for _, k := range keys {
		checkArr = append(checkArr, fmt.Sprintf("%s=%s", k, params.Get(k)))
	}
	checkStr := strings.Join(checkArr, "\n")

	// Perform HMAC-SHA256 signature verification securely
	macKey := hmac.New(sha256.New, []byte("WebAppData"))
	macKey.Write([]byte(w.bot.Client.token))
	secret := macKey.Sum(nil)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(checkStr))
	expected := hex.EncodeToString(mac.Sum(nil))

	isValid := subtle.ConstantTimeCompare([]byte(hash), []byte(expected)) == 1
	if !isValid {
		return false, errors.New("invalid signature hash")
	}

	// Validate auth_date expiration to prevent replay attacks on outdated data
	authDateStr := params.Get("auth_date")
	if authDateStr != "" {
		authDateUnix, errParse := strconv.ParseInt(authDateStr, 10, 64)
		if errParse == nil {
			authTime := time.Unix(authDateUnix, 0)
			if time.Since(authTime) > w.expire {
				return false, errors.New("authentication data has expired")
			}
		}
	}

	return true, nil
}

// DataPath ensures the "data" directory exists and returns an OS-independent path
func DataPath(filename string) string {
	// Auto-creates "data" directory recursively if it does not exist
	_ = os.MkdirAll("data", 0755)
	// Joins path using OS-specific path separators safely
	return filepath.Join("data", filename)
}
