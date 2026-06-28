package gobale

import (
	"errors"
)

// SettingsChain manages dynamic global variables and dynamic config UI toggles
type SettingsChain struct {
	bot *Bot
	ctx *Ctx
	key string
	op  string
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

// Settings opens fluent dynamic configuration settings dot system from Ctx context
func (c *Ctx) Settings() *SettingsChain {
	return &SettingsChain{bot: c.Bot, ctx: c}
}

// Register maps a boolean config key directly with its pointer and local DB
func (s *SettingsChain) Register(key, label string, ptr *bool) *SettingsChain {
	s.bot.mu.Lock()
	if s.bot.settingsDB == nil {
		s.bot.settingsDB = NewDatabase("gobale_settings.gob")
	}
	s.bot.mu.Unlock()

	s.bot.mu.Lock()
	s.bot.settings = append(s.bot.settings, SettingEntry{
		Key:   key,
		Label: label,
		Ptr:   ptr,
	})
	s.bot.mu.Unlock()

	db := s.bot.settingsDB
	if val, ok := db.Get(key); ok {
		if bVal, ok := val.(bool); ok {
			*ptr = bVal
		}
	}
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

// Go executes the settings registration or toggle operation with auto error logging
func (s *SettingsChain) Go() error {
	var err error
	if s.op == "toggle" {
		s.bot.mu.Lock()
		if s.bot.settingsDB == nil {
			s.bot.settingsDB = NewDatabase("gobale_settings.gob")
		}
		s.bot.mu.Unlock()

		db := s.bot.settingsDB
		s.bot.mu.Lock()
		defer s.bot.mu.Unlock()
		found := false
		for i := range s.bot.settings {
			if s.bot.settings[i].Key == s.key {
				*s.bot.settings[i].Ptr = !*s.bot.settings[i].Ptr

				// Standardized atomic write using the native Storage interface method Set
				err = db.Set(s.key, *s.bot.settings[i].Ptr)

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
