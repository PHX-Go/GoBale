package tests

import (
	"fmt"
	"sync"
	"testing"

	"github.com/PHX-Go/GoBale"
)

// تستینگ سیستم فوق‌سنگین هم‌روندی و مانیتورینگ رم اتمیک تحت بار غیرمتعارف
func TestSessionStoreBrutalStress(t *testing.T) {
	// نمونه‌سازی ربات شما
	bot := gobale.NewBot("123:test_token", 0)

	// تنظیم تهاجمی‌ترین و سخت‌گیرانه‌ترین لیمیت ممکن (فقط ۱ مگابایت رم فیزیکی!) برای محک زدن سیستم
	bot.SetMemoryLimit(1)
	bot.SetGCPercent(10) // بیدار شدن زباله‌روب در کسری از ثانیه‌ها جهت تخلیه مداوم رم تحت همروندی شدید

	var wg sync.WaitGroup
	workersCount := 5000     // ۵,۰۰۰ گوروتین موازی و همزمان در حافظه رم
	requestsPerWorker := 100 // ۱۰۰ درخواست به ازای هر ورکر (مجموعاً ۵۰۰,۰۰۰ تراکنش موازی روی شاردها)

	// آزاد کردن همزمان ۵,۰۰۰ نخ در حافظه
	for i := 0; i < workersCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < requestsPerWorker; j++ {
				// تولید چت‌آیدی اختصاصی برای هر ورکر جهت شبیه‌سازی ۵۰۰,۰۰۰ نشست موازی کاربران
				chatID := int64(workerID*1000 + j)
				session := bot.Sessions.Get(chatID)

				// تغییر و خواندن همزمان وضعیت‌ها (States) و داده‌های پویا (Data)
				session.SetState(fmt.Sprintf("state_%d", j))
				_ = session.GetState()

				session.SetData("key", j)
				_, exists := session.GetData("key")
				if !exists {
					t.Errorf("expected data to exist for worker %d, request %d", workerID, j)
				}
			}
		}(i)
	}

	// منتظر ماندن برای پایان کار تمام ۵,۰۰۰ ورکر پس‌زمینه
	wg.Wait()
}