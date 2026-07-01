package gobale

import (
	"errors"
	"fmt"
)

// SettingsChain manages dynamic global variables and dynamic config UI toggles
type SettingsChain struct {
	bot  *Bot
	ctx  *Ctx
	key  string
	op   string
	chat any // Optional target chat override for remote management
}

// SettingsGetChain handles safe concurrent read transactions on settings variables
type SettingsGetChain struct {
	sc  *SettingsChain
	key string
}

// Settings opens fluent dynamic configuration settings dot system from Bot context
func (b *Bot) Settings() *SettingsChain {
	return &SettingsChain{bot: b}
}

// Settings opens fluent dynamic configuration settings dot system from Ctx context supporting optional target chats
func (c *Ctx) Settings(chatID ...any) *SettingsChain {
	var target any
	if len(chatID) > 0 {
		target = chatID[0]
	}
	return &SettingsChain{bot: c.Bot, ctx: c, chat: target}
}

// Register maps a boolean config key directly with its pointer and consolidated dbInstance
func (s *SettingsChain) Register(key, label string, ptr *bool) *SettingsChain {
	s.bot.mu.Lock()
	s.bot.settings = append(s.bot.settings, SettingEntry{
		Key:   key,
		Label: label,
		Ptr:   ptr,
	})
	s.bot.mu.Unlock()

	db := s.bot.dbInstance // Fixed: Read from consolidated dbInstance instead of uninitialized settingsDB
	if val, ok := db.Get(key); ok {
		if bVal, ok := val.(bool); ok {
			*ptr = bVal
		}
	}
	return s
}

// RegisterLocal registers a chat-isolated boolean configuration template
func (s *SettingsChain) RegisterLocal(key, label string, defaultVal bool) *SettingsChain {
	s.bot.mu.Lock()
	s.bot.settings = append(s.bot.settings, SettingEntry{
		Key:     key,
		Label:   label,
		Default: defaultVal,
		IsLocal: true,
	})
	s.bot.mu.Unlock()
	return s
}

// Get prepares a safe concurrent read transaction on settings registry
func (s *SettingsChain) Get(key string) *SettingsGetChain {
	return &SettingsGetChain{
		sc:  s,
		key: key,
	}
}

// Go executes the safe read transaction and returns current setting value thread-safely
func (sg *SettingsGetChain) Go() (bool, error) {
	sg.sc.bot.mu.RLock()
	defer sg.sc.bot.mu.RUnlock()
	for i := range sg.sc.bot.settings {
		if sg.sc.bot.settings[i].Key == sg.key {
			if sg.sc.bot.settings[i].Ptr == nil {
				return false, errors.New("nil settings pointer registry")
			}
			return *sg.sc.bot.settings[i].Ptr, nil
		}
	}
	return false, errors.New("setting not found")
}

// Toggle registers a toggle operation on a setting key
func (s *SettingsChain) Toggle(key string) *SettingsChain {
	s.op = "toggle"
	s.key = key
	return s
}

// Go executes the settings registration or toggle operation with local-state fallback
func (s *SettingsChain) Go() error {
	var err error
	if s.op == "toggle" {
		db := s.bot.dbInstance
		s.bot.mu.Lock()
		defer s.bot.mu.Unlock()

		found := false
		for i := range s.bot.settings {
			if s.bot.settings[i].Key == s.key {
				if s.bot.settings[i].IsLocal {
					// Toggle chat-isolated GOB record dynamically (supports target chat override)
					var targetChat any
					if s.chat != nil {
						targetChat = s.chat
					} else {
						targetChat, _ = s.ctx.ChatID()
					}
					resolved := s.bot.ResolveChatID(targetChat)
					dbKey := fmt.Sprintf("group_config_%v_%s", resolved, s.key)

					current := s.bot.settings[i].Default
					val, ok := db.Get(dbKey)
					if ok {
						if bVal, okBool := val.(bool); okBool {
							current = bVal
						}
					}
					err = db.Set(dbKey, !current)
				} else {
					// Toggle global pointer variable
					*s.bot.settings[i].Ptr = !*s.bot.settings[i].Ptr
					err = db.Set(s.key, *s.bot.settings[i].Ptr)
				}
				found = true
				break
			}
		}
		if !found {
			err = errors.New("setting key not found")
		}
	}
	if err != nil {
		logErr(s.bot, "[Settings Error] ", err)
	}
	return err
}
