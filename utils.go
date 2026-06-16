package gobale

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
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
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var res []string
	for len(s) > 3 {
		res = append([]string{s[len(s)-3:]}, res...)
		s = s[:len(s)-3]
	}
	res = append([]string{s}, res...)
	return strings.Join(res, ",")
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
