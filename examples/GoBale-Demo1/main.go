package main

import (
	"fmt"
	"strconv"

	gobale "github.com/PHX-Go/GoBale"
	"github.com/PHX-Go/GoBale/models"
)

type Product struct {
	ID    int
	Name  string
	Price int64
}

var mockDB = []Product{
	{1, "📱 آیفون ۱۵ پرو مکس", 750000000},
	{2, "💻 مک‌بوک پرو M3", 950000000},
	{3, "🎧 هدفون ایرپاد مکس", 34000000},
	{4, "⌚ اپل واچ سری ۹", 22000000},
	{5, "🔌 پاوربانک شیائومی", 1800000},
	{6, "⌨️ کیبورد مکانیکی", 4500000},
	{7, "🖱️ موس گیمینگ ریزر", 3200000},
	{8, "📁 هارد اکسترنال ۲ ترابایت", 4800000},
}

func main() {
	gobale.LoadEnv(".env")

	botToken := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("BALE_ADMIN_ID")

	bot := gobale.NewBot(botToken, 0)

	// تنظیمات رانتایم
	bot.SetMemoryLimit(10)
	bot.SetGCPercent(50)
	bot.SetMaxBackgroundTasks(50)

	bot.MaintenanceAdminID = adminID
	bot.ReportErrorToAdmin("سیستم مرکزی فروشگاه بله", adminID)

	bot.RegisterSetting("logger", "📊 لاگر خطایابی ربات", &bot.Logger).
		RegisterSetting("maintenance", "🛠️ وضعیت تعمیرات سراسری", &bot.Maintenance)

	// روت دستور شروع با استفاده از پترن دکمه تکی هوشمند
	bot.OnCommand("/start", func(c *gobale.Context) {
		markup := models.ReplyMarkup().
			Row(
				models.ReplyBtn("📱 اشتراک شماره موبایل").Contact(),
				models.ReplyBtn("📍 اشتراک موقعیت مکانی").Location(),
			).
			Row("🛍️ کاتالوگ محصولات", "📞 پشتیبانی زنده").
			Row("📋 ثبت‌نام ویژه").
			Build()

		c.Send("👋 به بات فروشگاهی بله خوش آمدید! لطفا انتخاب کنید:", gobale.WithKeyboard(markup))
	})

	bot.OnText("📋 ثبت‌نام ویژه", func(c *gobale.Context) {
		isNumeric := func(text string) bool {
			_, err := strconv.Atoi(text)
			return err == nil
		}

		c.NewWizard().
			Step("👤 گام اول: لطفاً نام و نام خانوادگی خود را بفرستید:", func(c *gobale.Context) {
				c.SetData("name", c.MessageText())
			}).
			StepWithValidation("⏳ گام دوم: سن خود را وارد کنید (فقط عدد):", isNumeric, "❌ سن باید فقط عدد باشد! مجدداً ارسال کنید:", func(c *gobale.Context) {
				c.SetData("age", c.MessageText())
			}).
			OnComplete(func(c *gobale.Context) {
				name, _ := c.GetData("name")
				age, _ := c.GetData("age")
				c.SetData("role", "VIP")
				c.Reply(fmt.Sprintf("🎉 ثبت‌نام شما با موفقیت تکمیل شد!\n👤 نام: %s\n⏳ سن: %s سال", name, age))
			}).
			Run()
	})

	// صفحه‌بندی داینامیک دیتابیس
	bot.OnText("🛍️ کاتالوگ محصولات", func(c *gobale.Context) {
		_, _ = c.SendPaginatedDynamic("🛍️ کاتالوگ محصولات زنده فروشگاه (دریافت پویا از دیتابیس):", 3, func(page, limit int) ([]models.InlineKeyboardButton, int, error) {
			start := (page - 1) * limit
			end := start + limit
			if start > len(mockDB) {
				return nil, len(mockDB), nil
			}
			if end > len(mockDB) {
				end = len(mockDB)
			}

			dbPage := mockDB[start:end]

			// تعریف به صورت []any برای عبور بدون خطای سیستم تایپ گو به متد Row
			var buttons []any
			for _, prod := range dbPage {
				btn := models.Btn(fmt.Sprintf("%s - %s ریال", prod.Name, gobale.FormatMoney(prod.Price))).
					Callback(fmt.Sprintf("buy:%d", prod.ID))

				buttons = append(buttons, btn)
			}

			// ساخت کیبورد با مپ کردن دکمه‌های هوشمند
			markup := models.InlineMarkup().
				Row(buttons...).
				Build()

			return markup.InlineKeyboard[0], len(mockDB), nil
		})
	})

	bot.OnCallbackData("buy", func(c *gobale.Context) {
		var productID int
		_ = c.ScanCallbackArgs(&productID)

		_ = c.Answer("در حال تولید فاکتور...", false)

		var selectedProd Product
		for _, p := range mockDB {
			if p.ID == productID {
				selectedProd = p
				break
			}
		}

		invoice := models.NewInvoice().
			AddPrice(selectedProd.Name, selectedProd.Price).
			AddPrice("مالیات بر ارزش افزوده ۹٪", selectedProd.Price*9/100).
			Build()
		c.SendInvoice(selectedProd.Name, "خرید مستقیم از درگاه بانکی بله", fmt.Sprintf("tx_prod_%d", productID), "WALLET-TEST-1111111111111111", invoice)
	})

	bot.OnPreCheckout(func(c *gobale.Context) {
		c.AnswerPreCheckout(true, "")
	})

	bot.OnCommand("/menu", func(c *gobale.Context) {
		markup := models.InlineMarkup().
			Row(
				models.Btn("🛍️ خرید محصول").Callback("buy_product"),
				models.Btn("📞 پشتیبانی").Callback("support"),
			).
			Row(
				models.Btn("💳 کپی شماره کارت").Copy("6037991122223333"), // دکمه هوشمند کپی تکی
			).
			Row(
				models.Btn("🎮 ورود به بازی").WebApp("https://game.bale.ai"), // دکمه هوشمند وب‌آپ تکی
				models.Btn("🌐 وبسایت ما").URL("https://website.com"),        // دکمه هوشمند لینک تکی
			).
			Build()

		c.Send("منوی شیشه‌ای خدمات:", gobale.WithKeyboard(markup))
	})

	bot.OnCallbackData("buy_product", func(c *gobale.Context) {
		_ = c.Answer("در حال بارگذاری...", false)
		c.Reply("🛍️ سبد خرید لود شد.")
	})

	bot.OnCallbackData("support", func(c *gobale.Context) {
		_ = c.Answer("", false)
		c.Reply("📞 اپراتور آماده پاسخگویی است.")
	})

	adminGroup := bot.Group(gobale.AdminsOnly())
	adminGroup.OnCommand("/admin_config", func(c *gobale.Context) {
		_, _ = c.SendSettingsMenu("⚙️ منوی تنظیمات سیستمی:", bot.MaintenanceAdminID)
	})

	adminGroup.OnCommand("/panic_test", func(c *gobale.Context) {
		c.Reply("🔥 تحریک پنیک رانتایم گو...")
		var nilPointer *gobale.Context
		_ = nilPointer.Message.Text
	})

	bot.RunPolling()
}