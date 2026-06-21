package utils

import (
	"bufio"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func Bold(text string) string {
	return fmt.Sprintf(" *%s* ", text)
}

func Italic(text string) string {
	return fmt.Sprintf(" _%s_ ", text)
}

func Link(text, url string) string {
	return fmt.Sprintf("[%s](%s)", text, url)
}

func Tooltip(text, description string) string {
	return fmt.Sprintf("```[%s]%s```", text, description)
}

func FormatMoney(n int64) string {
	isNegative := n < 0
	if isNegative {
		n = -n
	}

	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		if isNegative {
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

	result := strings.Join(res, ",")
	if isNegative {
		return "-" + result
	}
	return result
}

func LoadConfig(filePath string, target any) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewDecoder(file).Decode(target)
}

func ResolveBaleChatID(target any, privateLinksMap map[string]int64) string {
	switch t := target.(type) {
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case string:
		t = strings.TrimSpace(t)
		if t == "" {
			return t
		}
		if strings.HasPrefix(t, "-") || (t[0] >= '0' && t[0] <= '9') {
			return t
		}
		if strings.HasPrefix(t, "@") {
			return t
		}
		if strings.Contains(t, "join/") && privateLinksMap != nil {
			cleanLink := strings.TrimPrefix(t, "http://")
			cleanLink = strings.TrimPrefix(cleanLink, "https://")
			if id, ok := privateLinksMap[cleanLink]; ok {
				return strconv.FormatInt(id, 10)
			}
			return t
		}
		if strings.Contains(t, "ble.ir/") {
			parts := strings.Split(t, "/")
			username := parts[len(parts)-1]
			if username != "" {
				return "@" + username
			}
		}
		return "@" + t
	}
	return fmt.Sprintf("%v", target)
}

func GregorianToJalali(gy, gm, gd int) (jy, jm, jd int) {
	gDaysInMonth := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	jDaysInMonth := []int{31, 31, 31, 31, 31, 31, 30, 30, 30, 30, 30, 29}

	gy2 := gy - 1600
	gm2 := gm - 1
	gd2 := gd - 1

	gDayNo := 365*gy2 + (gy2+3)/4 - (gy2+99)/100 + (gy2+399)/400
	for i := 0; i < gm2; i++ {
		gDayNo += gDaysInMonth[i]
	}
	if gm2 > 1 && ((gy%4 == 0 && gy%100 != 0) || gy%400 == 0) {
		gDayNo++
	}
	gDayNo += gd2

	jDayNo := gDayNo - 79

	jNp := jDayNo / 12053
	jDayNo %= 12053

	jy = 979 + 33*jNp + 4*(jDayNo/1461)
	jDayNo %= 1461

	if jDayNo >= 366 {
		jy += (jDayNo - 1) / 365
		jDayNo = (jDayNo - 1) % 365
	}

	var i int
	for i = 0; i < 11 && jDayNo >= jDaysInMonth[i]; i++ {
		jDayNo -= jDaysInMonth[i]
	}
	jm = i + 1
	jd = jDayNo + 1

	return jy, jm, jd
}

func TimeToJalali(t time.Time) (jy, jm, jd int) {
	return GregorianToJalali(t.Year(), int(t.Month()), t.Day())
}

func FormatJalali(t time.Time, layout string) string {
	jy, jm, jd := TimeToJalali(t)

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
		time.Thursday:  "پنج‌شنبه",
		time.Friday:    "جمعه",
	}

	monthName := persianMonths[jm-1]
	weekdayName := persianWeekdays[t.Weekday()]

	res := layout
	res = strings.ReplaceAll(res, "yyyy", fmt.Sprintf("%04d", jy))
	res = strings.ReplaceAll(res, "yy", fmt.Sprintf("%02d", jy%100))
	res = strings.ReplaceAll(res, "mm", fmt.Sprintf("%02d", jm))
	res = strings.ReplaceAll(res, "m", fmt.Sprintf("%d", jm))
	res = strings.ReplaceAll(res, "dd", fmt.Sprintf("%02d", jd))
	res = strings.ReplaceAll(res, "d", fmt.Sprintf("%d", jd))
	res = strings.ReplaceAll(res, "M", monthName)
	res = strings.ReplaceAll(res, "W", weekdayName)

	return res
}

func JalaliShort(t time.Time) string {
	return FormatJalali(t, "yyyy/mm/dd")
}

func JalaliMedium(t time.Time) string {
	return FormatJalali(t, "d M yyyy")
}

func JalaliLong(t time.Time) string {
	return FormatJalali(t, "W d M yyyy")
}

func JalaliCompact(t time.Time) string {
	return FormatJalali(t, "yy/mm/dd")
}

var configMutex sync.Mutex

func SaveConfigAtomic(filePath string, target any) {
	dataCopy, _ := json.Marshal(target)

	go func() {
		configMutex.Lock()
		defer configMutex.Unlock()

		tmpPath := filePath + ".tmp"
		file, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return
		}

		var decoded any
		_ = json.Unmarshal(dataCopy, &decoded)

		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(decoded)
		if err != nil {
			_ = file.Close()
			_ = os.Remove(tmpPath)
			return
		}

		_ = file.Sync()
		_ = file.Close()

		_ = os.Rename(tmpPath, filePath)
	}()
}

func LoadEnv(filePath string) error {
	file, err := os.Open(filePath)
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
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `"'`)
			_ = os.Setenv(key, val)
		}
	}
	return scanner.Err()
}

var digitReplacer = strings.NewReplacer(
	"۰", "0", "۱", "1", "۲", "2", "۳", "3", "۴", "4",
	"۵", "5", "۶", "6", "۷", "7", "۸", "8", "۹", "9",
	"٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4",
	"٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9",
)

func ToEnglishDigits(s string) string {
	return digitReplacer.Replace(s)
}

func GenerateSecureOTP(length int) (string, error) {
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

func GenerateSecureToken(bytesCount int) (string, error) {
	b := make([]byte, bytesCount)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func VerifyWebAppData(token string, initData string) (bool, error) {

	params, err := url.ParseQuery(initData)
	if err != nil {
		return false, err
	}

	hash := params.Get("hash")
	if hash == "" {
		return false, nil
	}

	var keys []string
	for k := range params {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var dataCheckArr []string
	for _, k := range keys {
		dataCheckArr = append(dataCheckArr, fmt.Sprintf("%s=%s", k, params.Get(k)))
	}
	dataCheckStr := strings.Join(dataCheckArr, "\n")

	macKey := hmac.New(sha256.New, []byte("WebAppData"))
	macKey.Write([]byte(token))
	secretKey := macKey.Sum(nil)

	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(dataCheckStr))
	expectedHash := hex.EncodeToString(mac.Sum(nil))

	isValid := subtle.ConstantTimeCompare([]byte(hash), []byte(expectedHash)) == 1
	return isValid, nil
}

// GoBale/utils/utils.go

// ParseDurationWithDays parses standard duration strings including custom 'd' (days) and 'w' (weeks) suffixes
func ParseDurationWithDays(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	if strings.HasSuffix(s, "w") {
		weeksStr := strings.TrimSuffix(s, "w")
		weeks, err := strconv.Atoi(weeksStr)
		if err != nil {
			return 0, err
		}
		return time.Duration(weeks) * 7 * 24 * time.Hour, nil
	}

	return time.ParseDuration(s)
}

func GetEnv[T any](key string) T {
	val := os.Getenv(key)
	var result T

	switch ptr := any(&result).(type) {
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
		if v, err := ParseDurationWithDays(val); err == nil { // Changed to capital P
			*ptr = v
		}
	}
	return result
}
