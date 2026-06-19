package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PHX-Go/GoBale"
	"github.com/PHX-Go/GoBale/methods"
	"github.com/PHX-Go/GoBale/models"
	"github.com/PHX-Go/GoBale/utils"
)

func TestUpdateUnmarshaling(t *testing.T) {
	rawUpdateJSON := `{
		"update_id": 11111,
		"message": {
			"message_id": 22222,
			"text": "/start",
			"chat": {
				"id": 999999999,
				"type": "group"
			}
		}
	}`

	var update models.Update
	err := json.Unmarshal([]byte(rawUpdateJSON), &update)
	if err != nil {
		t.Fatalf("failed to unmarshal raw update: %v", err)
	}

	if update.UpdateID != 11111 {
		t.Errorf("expected UpdateID 11111, got %d", update.UpdateID)
	}

	if update.Message == nil || update.Message.Text != "/start" {
		t.Error("failed to parse nested Message or Text correctly")
	}

	if update.Message.Chat.ID != 999999999 {
		t.Errorf("expected ChatID 999999999, got %d", update.Message.Chat.ID)
	}
}

func TestSessionStoreMemoryConstrained(t *testing.T) {
	bot := gobale.NewBot("123456:ABCdefGhIJKlmNoPQRsTUVwxyZ", 0)
	bot.SetMemoryLimit(1)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				chatID := int64(workerID*1000 + j)
				session := bot.Sessions.Get(chatID)
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
	wg.Wait()
}

func TestGracefulWorkerDraining(t *testing.T) {
	bot := gobale.NewBot("123456:ABCdefGhIJKlmNoPQRsTUVwxyZ", 4)
	ctx := context.Background()
	bot.StartWorkers(ctx)

	processedCount := 0
	bot.OnMessage(func(c *gobale.Context) {
		processedCount++
	})

	for i := 0; i < 50; i++ {
		bot.SendUpdateToWorkerChan(&models.Update{
			UpdateID: i,
			Message: &models.Message{
				Text: "/start",
			},
		})
	}

	close(bot.GetWorkerChan())

	done := make(chan struct{})
	go func() {
		bot.GetWorkersWg().Wait()
		done <- struct{}{}
	}()

	select {
	case <-time.After(2 * time.Second):
		t.Fatal("worker pool did not drain and shut down gracefully within 2 seconds")
	case <-done:
	}
}

func TestUserAndChatFullInfoParsing(t *testing.T) {
	userJSON := `{
		"id": 111111111,
		"is_bot": false,
		"first_name": "PHX"
	}`

	var user models.User
	err := json.Unmarshal([]byte(userJSON), &user)
	if err != nil {
		t.Fatalf("failed to unmarshal User: %v", err)
	}

	if user.ID != 111111111 || user.FirstName != "PHX" {
		t.Error("User fields unmarshaled incorrectly")
	}

	chatFullJSON := `{
		"id": -1001111111111,
		"type": "supergroup",
		"title": "گروه توسعه بله",
		"description": "گروه تست ربات‌های برنامه‌نویسی بله",
		"invite_link": "https://ble.ir/join/ABC"
	}`

	var chatFull models.ChatFullInfo
	err = json.Unmarshal([]byte(chatFullJSON), &chatFull)
	if err != nil {
		t.Fatalf("failed to unmarshal ChatFullInfo: %v", err)
	}

	if chatFull.ID != -1001111111111 || chatFull.Type != "supergroup" || chatFull.InviteLink != "https://ble.ir/join/ABC" {
		t.Error("ChatFullInfo fields unmarshaled incorrectly")
	}
}

func TestMessageAndEntityParsing(t *testing.T) {
	messageJSON := `{
		"message_id": 22222,
		"date": 1781463901,
		"chat": {
			"id": 999999999,
			"type": "group",
			"title": "گروه برنامه‌نویسی"
		},
		"text": "/start ربات",
		"entities": [
			{
				"type": "bot_command",
				"offset": 0,
				"length": 6
			}
		]
	}`

	var msg models.Message
	err := json.Unmarshal([]byte(messageJSON), &msg)
	if err != nil {
		t.Fatalf("failed to unmarshal Message: %v", err)
	}

	if msg.MessageID != 22222 || msg.Text != "/start ربات" {
		t.Error("Message basic fields unmarshaled incorrectly")
	}

	if len(msg.Entities) == 0 {
		t.Fatal("expected at least one MessageEntity")
	}

	entity := msg.Entities[0]
	if entity.Type != "bot_command" || entity.Offset != 0 || entity.Length != 6 {
		t.Error("MessageEntity fields unmarshaled incorrectly")
	}

	cmd := msg.Text[entity.Offset : entity.Offset+entity.Length]
	if cmd != "/start" {
		t.Errorf("expected command '/start', got %q", cmd)
	}
}

func TestMediaAndLocationModelsParsing(t *testing.T) {
	animationJSON := `{
		"file_id": "anim_12345",
		"file_unique_id": "uniq_anim_99",
		"width": 320,
		"height": 240,
		"duration": 5,
		"file_name": "sticker.gif",
		"mime_type": "image/gif",
		"file_size": 204800,
		"thumbnail": {
			"file_id": "thumb_123",
			"file_unique_id": "uniq_thumb_1",
			"width": 100,
			"height": 100,
			"file_size": 15400
		}
	}`

	var anim models.Animation
	err := json.Unmarshal([]byte(animationJSON), &anim)
	if err != nil {
		t.Fatalf("failed to unmarshal Animation: %v", err)
	}

	if anim.FileID != "anim_12345" || anim.FileName != "sticker.gif" || anim.Thumbnail == nil {
		t.Error("Animation fields or nested Thumbnail unmarshaled incorrectly")
	}

	if anim.Thumbnail.FileID != "thumb_123" || anim.Thumbnail.Width != 100 {
		t.Error("Nested Thumbnail fields unmarshaled incorrectly")
	}

	contactJSON := `{
		"phone_number": "09120001111",
		"first_name": "پشتیبانی",
		"last_name": "بله",
		"user_id": 111111111
	}`

	var contact models.Contact
	err = json.Unmarshal([]byte(contactJSON), &contact)
	if err != nil {
		t.Fatalf("failed to unmarshal Contact: %v", err)
	}

	if contact.PhoneNumber != "09120001111" || contact.FirstName != "پشتیبانی" || contact.UserID != 111111111 {
		t.Error("Contact fields unmarshaled incorrectly")
	}

	locationJSON := `{
		"latitude": 35.6997,
		"longitude": 51.3380
	}`

	var location models.Location
	err = json.Unmarshal([]byte(locationJSON), &location)
	if err != nil {
		t.Fatalf("failed to unmarshal Location: %v", err)
	}

	if location.Latitude != 35.6997 || location.Longitude != 51.3380 {
		t.Error("Location fields unmarshaled incorrectly")
	}

	fileJSON := `{
		"file_id": "doc_file_9012",
		"file_unique_id": "uniq_doc_4",
		"file_size": 1048576,
		"file_path": "documents/file_1.pdf"
	}`

	var file models.File
	err = json.Unmarshal([]byte(fileJSON), &file)
	if err != nil {
		t.Fatalf("failed to unmarshal File: %v", err)
	}

	if file.FileID != "doc_file_9012" || file.FileSize != 1048576 || file.FilePath != "documents/file_1.pdf" {
		t.Error("File fields unmarshaled incorrectly")
	}
}

func TestKeyboardAndCallbackModelsParsing(t *testing.T) {
	replyMarkupJSON := `{
		"keyboard": [
			[
				{"text": "📱 ارسال تلفن", "request_contact": true},
				{"text": "📍 ارسال موقعیت", "request_location": true}
			]
		],
		"resize_keyboard": true,
		"one_time_keyboard": true
	}`

	var replyMarkup models.ReplyKeyboardMarkup
	err := json.Unmarshal([]byte(replyMarkupJSON), &replyMarkup)
	if err != nil {
		t.Fatalf("failed to unmarshal ReplyKeyboardMarkup: %v", err)
	}

	if len(replyMarkup.Keyboard) == 0 || len(replyMarkup.Keyboard[0]) != 2 {
		t.Error("ReplyKeyboardMarkup nested Keyboard array unmarshaled incorrectly")
	}

	btn := replyMarkup.Keyboard[0][0]
	if btn.Text != "📱 ارسال تلفن" || !btn.RequestContact {
		t.Error("Nested KeyboardButton fields unmarshaled incorrectly")
	}

	inlineMarkupJSON := `{
		"inline_keyboard": [
			[
				{
					"text": "💳 کپی شماره کارت",
					"copy_text": {
						"text": "6037991122223333"
					}
				},
				{
					"text": "🎮 اجرای بازی",
					"web_app": {
						"url": "https://game.bale.ai"
					}
				}
			]
		]
	}`

	var inlineMarkup models.InlineKeyboardMarkup
	err = json.Unmarshal([]byte(inlineMarkupJSON), &inlineMarkup)
	if err != nil {
		t.Fatalf("failed to unmarshal InlineKeyboardMarkup: %v", err)
	}

	if len(inlineMarkup.InlineKeyboard) == 0 || len(inlineMarkup.InlineKeyboard[0]) != 2 {
		t.Error("InlineKeyboardMarkup nested InlineKeyboard array unmarshaled incorrectly")
	}

	copyBtn := inlineMarkup.InlineKeyboard[0][0]
	if copyBtn.Text != "💳 کپی شماره کارت" || copyBtn.CopyText == nil || copyBtn.CopyText.Text != "6037991122223333" {
		t.Error("Nested CopyTextButton in InlineKeyboardButton unmarshaled incorrectly")
	}

	webAppBtn := inlineMarkup.InlineKeyboard[0][1]
	if webAppBtn.Text != "🎮 اجرای بازی" || webAppBtn.WebApp == nil || webAppBtn.WebApp.URL != "https://game.bale.ai" {
		t.Error("Nested WebAppInfo in InlineKeyboardButton unmarshaled incorrectly")
	}

	callbackJSON := `{
		"id": "cb_query_9012",
		"data": "buy_item_123",
		"from": {
			"id": 111111111,
			"is_bot": false,
			"first_name": "PHX"
		}
	}`

	var callback models.CallbackQuery
	err = json.Unmarshal([]byte(callbackJSON), &callback)
	if err != nil {
		t.Fatalf("failed to unmarshal CallbackQuery: %v", err)
	}

	if callback.ID != "cb_query_9012" || callback.Data != "buy_item_123" || callback.From.ID != 111111111 {
		t.Error("CallbackQuery fields unmarshaled incorrectly")
	}
}

func TestChatMemberAndParamsParsing(t *testing.T) {
	memberJSON := `{
		"status": "administrator",
		"can_delete_messages": true,
		"can_restrict_members": true,
		"can_promote_members": false,
		"user": {
			"id": 111111111,
			"is_bot": false,
			"first_name": "PHX"
		}
	}`

	var member models.ChatMember
	err := json.Unmarshal([]byte(memberJSON), &member)
	if err != nil {
		t.Fatalf("failed to unmarshal ChatMember: %v", err)
	}

	if member.Status != "administrator" || !member.CanDeleteMessages || member.CanPromoteMembers {
		t.Error("ChatMember role or permissions unmarshaled incorrectly")
	}

	if member.User.ID != 111111111 || member.User.FirstName != "PHX" {
		t.Error("Nested User in ChatMember unmarshaled incorrectly")
	}

	photoJSON := `{
		"small_file_id": "small_img_1",
		"small_file_unique_id": "uniq_small_1",
		"big_file_id": "big_img_1",
		"big_file_unique_id": "uniq_big_1"
	}`

	var chatPhoto models.ChatPhoto
	err = json.Unmarshal([]byte(photoJSON), &chatPhoto)
	if err != nil {
		t.Fatalf("failed to unmarshal ChatPhoto: %v", err)
	}

	if chatPhoto.SmallFileID != "small_img_1" || chatPhoto.BigFileID != "big_img_1" {
		t.Error("ChatPhoto fields unmarshaled incorrectly")
	}

	paramsJSON := `{
		"retry_after": 15
	}`

	var params models.ResponseParameters
	err = json.Unmarshal([]byte(paramsJSON), &params)
	if err != nil {
		t.Fatalf("failed to unmarshal ResponseParameters: %v", err)
	}

	if params.RetryAfter != 15 {
		t.Errorf("expected RetryAfter 15, got %d", params.RetryAfter)
	}
}

func TestInputMediaAndFileModelsParsing(t *testing.T) {
	mediaPhotoJSON := `{
		"type": "photo",
		"media": "attach://file_0",
		"caption": "تصویر تستی"
	}`

	var inputPhoto models.InputMediaPhoto
	err := json.Unmarshal([]byte(mediaPhotoJSON), &inputPhoto)
	if err != nil {
		t.Fatalf("failed to unmarshal InputMediaPhoto: %v", err)
	}

	if inputPhoto.Type != "photo" || inputPhoto.Media != "attach://file_0" || inputPhoto.Caption != "تصویر تستی" {
		t.Error("InputMediaPhoto fields unmarshaled incorrectly")
	}

	var _ models.InputMedia = inputPhoto

	mediaVideoJSON := `{
		"type": "video",
		"media": "https://website.com/clip.mp4",
		"caption": "ویدیوی تستی"
	}`

	var inputVideo models.InputMediaVideo
	err = json.Unmarshal([]byte(mediaVideoJSON), &inputVideo)
	if err != nil {
		t.Fatalf("failed to unmarshal InputMediaVideo: %v", err)
	}

	if inputVideo.Type != "video" || inputVideo.Media != "https://website.com/clip.mp4" || inputVideo.Caption != "ویدیوی تستی" {
		t.Error("InputMediaVideo fields unmarshaled incorrectly")
	}

	var _ models.InputMedia = inputVideo

	fileBytes := []byte("mock binary data of the file")
	inputFile := models.InputFile{
		Field:    "document",
		FileName: "test_doc.pdf",
		Reader:   bytes.NewReader(fileBytes),
	}

	readBytes, err := io.ReadAll(inputFile.Reader)
	if err != nil {
		t.Fatalf("failed to read from InputFile reader: %v", err)
	}

	if string(readBytes) != "mock binary data of the file" {
		t.Errorf("expected file content 'mock binary data of the file', got %q", string(readBytes))
	}
}

func TestMessagingMethodsSerialization(t *testing.T) {
	sendMsg := methods.SendMessage{
		ChatID:           999999999,
		Text:             "سلام",
		ParseMode:        "Markdown",
		ReplyToMessageID: 22222,
	}

	rawBytes, err := json.Marshal(sendMsg)
	if err != nil {
		t.Fatalf("failed to marshal SendMessage: %v", err)
	}

	var parsed map[string]any
	_ = json.Unmarshal(rawBytes, &parsed)

	if parsed["chat_id"].(float64) != 999999999 || parsed["text"].(string) != "سلام" || parsed["parse_mode"].(string) != "Markdown" {
		t.Error("SendMessage serialized incorrectly")
	}

	forwardMsg := methods.ForwardMessage{
		ChatID:     999999999,
		FromChatID: "@source_channel",
		MessageID:  22222,
	}

	rawBytes, err = json.Marshal(forwardMsg)
	if err != nil {
		t.Fatalf("failed to marshal ForwardMessage: %v", err)
	}

	var parsedForward map[string]any
	_ = json.Unmarshal(rawBytes, &parsedForward)

	if parsedForward["chat_id"].(float64) != 999999999 || parsedForward["from_chat_id"].(string) != "@source_channel" || parsedForward["message_id"].(float64) != 22222 {
		t.Error("ForwardMessage serialized incorrectly")
	}

	mediaList := []any{
		models.InputMediaPhoto{Type: "photo", Media: "attach://file_0", Caption: "تصویر تستی"},
	}
	sendAlbum := methods.SendMediaGroup{
		ChatID: 999999999,
		Media:  mediaList,
	}

	rawBytes, err = json.Marshal(sendAlbum)
	if err != nil {
		t.Fatalf("failed to marshal SendMediaGroup: %v", err)
	}

	var parsedAlbum map[string]any
	_ = json.Unmarshal(rawBytes, &parsedAlbum)

	if parsedAlbum["chat_id"].(float64) != 999999999 || parsedAlbum["media"] == nil {
		t.Error("SendMediaGroup serialized incorrectly")
	}
}

func TestModerationMethodsSerialization(t *testing.T) {
	banReq := methods.BanChatMember{
		ChatID: 999999999,
		UserID: 111111111,
	}

	rawBytes, err := json.Marshal(banReq)
	if err != nil {
		t.Fatalf("failed to marshal BanChatMember: %v", err)
	}

	var parsedBan map[string]any
	_ = json.Unmarshal(rawBytes, &parsedBan)

	if parsedBan["chat_id"].(float64) != 999999999 || parsedBan["user_id"].(float64) != 111111111 {
		t.Error("BanChatMember serialized incorrectly")
	}

	promoteReq := methods.PromoteChatMember{
		ChatID:            999999999,
		UserID:            111111111,
		CanDeleteMessages: true,
		CanInviteUsers:    true,
	}

	rawBytes, err = json.Marshal(promoteReq)
	if err != nil {
		t.Fatalf("failed to marshal PromoteChatMember: %v", err)
	}

	var parsedPromote map[string]any
	_ = json.Unmarshal(rawBytes, &parsedPromote)

	if parsedPromote["user_id"].(float64) != 111111111 || !parsedPromote["can_delete_messages"].(bool) || !parsedPromote["can_invite_users"].(bool) {
		t.Error("PromoteChatMember serialized incorrectly")
	}

	pinReq := methods.PinChatMessage{
		ChatID:    999999999,
		MessageID: 22222,
	}

	rawBytes, err = json.Marshal(pinReq)
	if err != nil {
		t.Fatalf("failed to marshal PinChatMessage: %v", err)
	}

	var parsedPin map[string]any
	_ = json.Unmarshal(rawBytes, &parsedPin)

	if parsedPin["chat_id"].(float64) != 999999999 || parsedPin["message_id"].(float64) != 22222 {
		t.Error("PinChatMessage serialized incorrectly")
	}
}

func TestRateLimiterTickerFree(t *testing.T) {
	rl := gobale.NewRateLimiter(5, time.Second)
	ctx := context.Background()
	startTime := time.Now()

	for i := 0; i < 5; i++ {
		err := rl.Wait(ctx)
		if err != nil {
			t.Fatalf("rate limiter failed: %v", err)
		}
	}

	elapsed := time.Since(startTime)
	if elapsed > 100*time.Millisecond {
		t.Errorf("expected first 5 tokens to be consumed instantly, took %v", elapsed)
	}
}

func TestCircuitBreaker(t *testing.T) {
	cb := gobale.NewCircuitBreaker(3, 50*time.Millisecond)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.CanExecute() {
		t.Error("circuit breaker should be open and block execution")
	}

	time.Sleep(60 * time.Millisecond)

	if !cb.CanExecute() {
		t.Error("circuit breaker should allow the first probe (transition to half-open)")
	}

	if !cb.CanExecute() {
		t.Error("circuit breaker should allow the second probe in half-open state")
	}

	if cb.CanExecute() {
		t.Error("circuit breaker should block subsequent probes in half-open state")
	}

	cb.RecordSuccess()

	if !cb.CanExecute() {
		t.Error("circuit breaker should be closed after recording success")
	}
}

func TestCircuitBreakerHalfOpenCAS(t *testing.T) {
	cb := gobale.NewCircuitBreaker(3, 50*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.CanExecute() {
		t.Fatal("expected circuit breaker to be open and block execution")
	}

	time.Sleep(60 * time.Millisecond)

	var wg sync.WaitGroup
	allowedCount := int32(0)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if cb.CanExecute() {
				atomic.AddInt32(&allowedCount, 1)
			}
		}()
	}
	wg.Wait()

	if allowedCount != 2 {
		t.Errorf("expected exactly 2 probe requests to be allowed in Half-Open transition, got %d", allowedCount)
	}
}

func TestScanValuesUnified(t *testing.T) {
	commandArgs := []string{"@user_id", "10d", "به", "علت", "اسپم"}

	var username string
	var duration time.Duration
	var reason string

	err := gobale.ScanValues(commandArgs, " ", &username, &duration, &reason)
	if err != nil {
		t.Fatalf("ScanValues failed for command args: %v", err)
	}

	if username != "@user_id" || duration != 240*time.Hour || reason != "به علت اسپم" {
		t.Error("Command arguments scanned incorrectly")
	}

	callbackArgs := []string{"1001", "5", "کارت", "بانکی"}

	var productID int64
	var quantity int
	var description string

	err = gobale.ScanValues(callbackArgs, ":", &productID, &quantity, &description)
	if err != nil {
		t.Fatalf("ScanValues failed for callback args: %v", err)
	}

	if productID != 1001 || quantity != 5 || description != "کارت:بانکی" {
		t.Error("Callback arguments scanned incorrectly")
	}
}

func TestPersistentFileCache(t *testing.T) {
	testFileCachePath := "test_file_cache.db"
	defer os.Remove(testFileCachePath)

	fc := gobale.NewFileCache(testFileCachePath)
	fc.Store("local_photo.jpg", "AgADBAADFjg...file_id_example")

	fcRecreated := gobale.NewFileCache(testFileCachePath)
	cachedID, ok := fcRecreated.Load("local_photo.jpg")

	if !ok || cachedID.(string) != "AgADBAADFjg...file_id_example" {
		t.Error("Persistent file ID cache failed to load/save atomically across sessions")
	}
}

func TestFormattedTextLineBuilder(t *testing.T) {
	text := utils.Text().
		Line(utils.Bold("🧾 رسید")).
		Line().
		Line("محصول: ", utils.Bold("اشتراک طلایی")).
		Build()

	expected := " *🧾 رسید* \n\nمحصول:  *اشتراک طلایی* \n"
	if text != expected {
		t.Errorf("expected formatted text %q, got %q", expected, text)
	}
}

func TestSendOptionsFunctionalPattern(t *testing.T) {
	config := &gobale.SendOptions{}

	opts := []gobale.Option{
		gobale.WithReply(),
		gobale.WithMarkdown(),
		gobale.WithCaption("توضیحات عکس تستی"),
	}

	for _, opt := range opts {
		opt(config)
	}

	if config.ReplyToMessageID != -1 {
		t.Errorf("expected ReplyToMessageID -1, got %d", config.ReplyToMessageID)
	}

	if config.ParseMode != "Markdown" {
		t.Errorf("expected ParseMode 'Markdown', got %q", config.ParseMode)
	}

	if config.Caption != "توضیحات عکس تستی" {
		t.Errorf("expected Caption 'توضیحات عکس تستی', got %q", config.Caption)
	}
}

func BenchmarkBotCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		gobale.NewBot("123456:ABCdefGhIJKlmNoPQRsTUVwxyZ", 10)
	}
}

func BenchmarkRateLimiter(b *testing.B) {
	limiter := gobale.NewRateLimiter(1000000, time.Second)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = limiter.Wait(ctx)
	}
}
