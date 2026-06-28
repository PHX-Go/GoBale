package gobale

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// SafirChain handles transactional messaging through Bale's Safir REST API
type SafirChain struct {
	bot    *Bot
	ctx    context.Context
	apiKey string
	botID  int64
	phone  string
	reqID  string
	text   string
	fileID string
	otp    string
	secure bool
	markup any
}

// Safir opens fluent Safir enterprise messaging dot chain from Bot context
func (b *Bot) Safir() *SafirChain {
	return &SafirChain{
		bot:    b,
		ctx:    context.Background(),
		apiKey: b.safirKey,
		botID:  b.safirBotID,
	}
}

// Safir opens fluent Safir enterprise messaging dot chain from Handler context
func (c *Ctx) Safir() *SafirChain {
	return &SafirChain{
		bot:    c.Bot,
		ctx:    c.ctx,
		apiKey: c.Bot.safirKey,
		botID:  c.Bot.safirBotID,
	}
}

// Phone registers the destination mobile number and normalizes it to Safir format
func (s *SafirChain) Phone(phoneNumber string) *SafirChain {
	phone, ok := NormalizeSafirPhone(phoneNumber)
	if ok {
		s.phone = phone
	} else {
		s.phone = phoneNumber
	}
	return s
}

// RequestID registers custom idempotency key to prevent duplicate sends
func (s *SafirChain) RequestID(id string) *SafirChain {
	s.reqID = id
	return s
}

// Text appends text body to the Safir sending pipeline
func (s *SendChain) SafirText(t string) *SendChain {
	s.text = t
	return s
}

// Text appends text body to the Safir sending pipeline
func (s *SafirChain) Text(t string) *SafirChain {
	s.text = t
	return s
}

// FileID attaches pre uploaded file ID to the Safir pipeline
func (s *SafirChain) FileID(id string) *SafirChain {
	s.fileID = id
	return s
}

// Secure enables password-locked secure encryption for this message
func (s *SafirChain) Secure(val bool) *SafirChain {
	s.secure = val
	return s
}

// OTP appends a numeric One Time Passcode to the Safir OTP template
func (s *SafirChain) OTP(code string) *SafirChain {
	s.otp = code
	return s
}

// Markup appends a glass inline keyboard to the Safir message
func (s *SafirChain) Markup(m any) *SafirChain {
	s.markup = m
	return s
}

// Go executes the transactional message sending to Safir REST servers
func (s *SafirChain) Go() (*SafirResponse, error) {
	if s.apiKey == "" || s.botID <= 0 {
		return nil, errors.New("missing Safir API credentials configuration")
	}
	if s.phone == "" {
		return nil, errors.New("missing recipient phone number")
	}

	// Auto-generate secure random idempotency request token if not provided
	if s.reqID == "" {
		s.reqID, _ = Token(12)
	}

	payload := map[string]any{
		"request_id":   s.reqID,
		"bot_id":       s.botID,
		"phone_number": s.phone,
	}

	msgData := map[string]any{
		"is_secure": s.secure,
	}

	if s.otp != "" {
		msgData["otp_message"] = map[string]string{
			"otp": s.otp,
		}
	} else {
		msgObj := map[string]any{
			"text":         s.text,
			"file_id":      s.fileID,
			"reply_markup": s.markup,
		}
		msgData["message"] = msgObj
	}

	payload["message_data"] = msgData

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(payload); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(s.ctx, http.MethodPost, "https://safir.bale.ai/api/v3/send_message", buf)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-access-key", s.apiKey)

	resp, err := s.bot.Client.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("safir API returned bad status: %d", resp.StatusCode)
	}

	var out SafirResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	return &out, nil
}

// SafirUploadChain handles fluent file uploads onto Safir REST servers
type SafirUploadChain struct {
	sc   *SafirChain
	file InputFile
}

// Upload initiates a Safir file uploader chain
func (s *SafirChain) Upload(file InputFile) *SafirUploadChain {
	return &SafirUploadChain{sc: s, file: file}
}

// Go executes the multipart file upload onto Safir servers and returns unique File ID
func (su *SafirUploadChain) Go() (string, error) {
	if su.sc.apiKey == "" {
		return "", errors.New("missing Safir API credentials configuration")
	}
	su.file.Field = "file"

	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)
	part, err := writer.CreateFormFile(su.file.Field, su.file.FileName)
	if err != nil {
		return "", err
	}
	_, _ = io.Copy(part, su.file.Reader)
	_ = writer.Close()

	req, err := http.NewRequestWithContext(su.sc.ctx, http.MethodPost, "https://safir.bale.ai/api/v3/upload_file", buf)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("api-access-key", su.sc.apiKey)

	resp, err := su.sc.bot.Client.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var out struct {
		FileID string `json:"file_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}

	return out.FileID, nil
}
