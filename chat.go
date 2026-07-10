package gobale

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ChatChain wraps chat administration and management actions
type ChatChain struct {
	bot  *Bot
	ctx  context.Context
	chat any
	c    *Ctx
}

// Chat opens the chat management dot chain from the Bot context
func (b *Bot) Chat(chat any) *ChatChain {
	return &ChatChain{
		bot:  b,
		ctx:  context.Background(),
		chat: chat,
	}
}

// Chat opens the chat management dot chain from the Handler context
func (c *Ctx) Chat() *ChatChain {
	id, _ := c.ChatID()
	return &ChatChain{
		bot:  c.Bot,
		ctx:  c.ctx,
		chat: id,
		c:    c,
	}
}

// Title modifies the title of the designated chat
func (c *ChatChain) Title(t string) *TitleChain {
	return &TitleChain{cc: c, t: t}
}

// TitleChain handles fluent title edits
type TitleChain struct {
	cc *ChatChain
	t  string
}

// Go executes the chat title change with auto error logging
func (tc *TitleChain) Go() error {
	resolved := tc.cc.bot.ResolveChatID(tc.cc.chat)
	err := tc.cc.bot.BaseRequest(tc.cc.ctx, "setChatTitle", map[string]any{
		"chat_id": resolved,
		"title":   tc.t,
	}, nil)
	if err != nil {
		logErr(tc.cc.bot, "[Chat Title Error] ", err)
	}
	return err
}

// Desc modifies the description text of the designated chat
func (c *ChatChain) Desc(d string) *DescChain {
	return &DescChain{cc: c, d: d}
}

// DescChain handles fluent description edits
type DescChain struct {
	cc *ChatChain
	d  string
}

// Go executes the chat description change with auto error logging
func (dc *DescChain) Go() error {
	resolved := dc.cc.bot.ResolveChatID(dc.cc.chat)
	err := dc.cc.bot.BaseRequest(dc.cc.ctx, "setChatDescription", map[string]any{
		"chat_id":     resolved,
		"description": dc.d,
	}, nil)
	if err != nil {
		logErr(dc.cc.bot, "[Chat Description Error] ", err)
	}
	return err
}

// SetPhoto configures the chat chain to change the chat's avatar photo (supports local paths, file IDs, or URLs)
func (c *ChatChain) SetPhoto(photo any) *SetPhotoChain {
	return &SetPhotoChain{
		cc:    c,
		photo: photo,
	}
}

// SetPhotoChain handles fluent photo uploads safely using polymorphic any types
type SetPhotoChain struct {
	cc    *ChatChain
	photo any
}

// Go executes the photo upload on Bale servers supporting both local files and file IDs
func (sp *SetPhotoChain) Go() error {
	resolved := sp.cc.bot.ResolveChatID(sp.cc.chat)
	var err error

	switch p := sp.photo.(type) {
	case string:
		if isLocalFile(p) {
			file, errOpen := os.Open(p)
			if errOpen != nil {
				return errOpen
			}
			defer file.Close()
			inputFile := InputFile{
				FileName: filepath.Base(p),
				Reader:   file,
				Field:    "photo",
			}
			err = sp.cc.bot.BaseRequestMultipart(sp.cc.ctx, "setChatPhoto", map[string]any{
				"chat_id": resolved,
			}, []InputFile{inputFile}, nil)
		} else {
			err = sp.cc.bot.BaseRequest(sp.cc.ctx, "setChatPhoto", map[string]any{
				"chat_id": resolved,
				"photo":   p,
			}, nil)
		}
	case InputFile:
		p.Field = "photo"
		err = sp.cc.bot.BaseRequestMultipart(sp.cc.ctx, "setChatPhoto", map[string]any{
			"chat_id": resolved,
		}, []InputFile{p}, nil)
	}

	if err != nil {
		logErr(sp.cc.bot, "[Chat Photo Upload Error] ", err)
	}
	return err
}

// DelPhoto initiates a photo deletion chain
func (c *ChatChain) DelPhoto() *DelPhotoChain {
	return &DelPhotoChain{cc: c}
}

// DelPhotoChain handles fluent photo deletion
type DelPhotoChain struct {
	cc *ChatChain
}

// Go executes display photo deletion with auto error logging
func (dp *DelPhotoChain) Go() error {
	resolved := dp.cc.bot.ResolveChatID(dp.cc.chat)
	err := dp.cc.bot.BaseRequest(dp.cc.ctx, "deleteChatPhoto", map[string]any{
		"chat_id": resolved,
	}, nil)
	if err != nil {
		logErr(dp.cc.bot, "[Chat Photo Delete Error] ", err)
	}
	return err
}

// Ban initiates a user ban restriction chain
func (c *ChatChain) Ban(userID int64) *BanChain {
	return &BanChain{cc: c, user: userID}
}

// BanChain handles fluent user bans
type BanChain struct {
	cc   *ChatChain
	user int64
}

// Go executes ban with auto error logging
func (b *BanChain) Go() error {
	resolved := b.cc.bot.ResolveChatID(b.cc.chat)
	err := b.cc.bot.BaseRequest(b.cc.ctx, "banChatMember", map[string]any{
		"chat_id": resolved,
		"user_id": b.user,
	}, nil)
	if err != nil {
		logErr(b.cc.bot, "[Chat Ban Error] ", err)
	}
	return err
}

// Unban removes ban restriction from a chat member
func (c *ChatChain) Unban(userID int64) *UnbanChain {
	return &UnbanChain{cc: c, user: userID}
}

// UnbanChain holds parameters for unbanning fluent sequence
type UnbanChain struct {
	cc   *ChatChain
	user int64
	oib  bool
}

// OnlyIfBanned configures unban actions to trigger only for banned members
func (u *UnbanChain) OnlyIfBanned(val bool) *UnbanChain {
	u.oib = val
	return u
}

// Go executes the unban request with auto error logging
func (u *UnbanChain) Go() error {
	resolved := u.cc.bot.ResolveChatID(u.cc.chat)
	err := u.cc.bot.BaseRequest(u.cc.ctx, "unbanChatMember", map[string]any{
		"chat_id":        resolved,
		"user_id":        u.user,
		"only_if_banned": u.oib,
	}, nil)
	if err != nil {
		logErr(u.cc.bot, "[Chat Unban Error] ", err)
	}
	return err
}

// PromoteChain holds admin privileges flags for chat promotion based on Bale schema
type PromoteChain struct {
	cc    *ChatChain
	user  int64
	info  *bool // can_change_info
	post  *bool // can_post_messages
	edit  *bool // can_edit_messages
	del   *bool // can_delete_messages
	vChat *bool // can_manage_video_chats
	inv   *bool // can_invite_users
	rest  *bool // can_restrict_members
}

// Promote initializes a promotion configuration chain
func (c *ChatChain) Promote(userID int64) *PromoteChain {
	return &PromoteChain{cc: c, user: userID}
}

// ChangeInfo sets can_change_info permission
func (p *PromoteChain) ChangeInfo(v bool) *PromoteChain { p.info = &v; return p }

// PostMessages sets can_post_messages permission
func (p *PromoteChain) PostMessages(v bool) *PromoteChain { p.post = &v; return p }

// EditMessages sets can_edit_messages permission
func (p *PromoteChain) EditMessages(v bool) *PromoteChain { p.edit = &v; return p }

// DeleteMessages sets can_delete_messages permission
func (p *PromoteChain) DeleteMessages(v bool) *PromoteChain { p.del = &v; return p }

// ManageVideoChats sets can_manage_video_chats permission
func (p *PromoteChain) ManageVideoChats(v bool) *PromoteChain { p.vChat = &v; return p }

// InviteUsers sets can_invite_users permission
func (p *PromoteChain) InviteUsers(v bool) *PromoteChain { p.inv = &v; return p }

// RestrictMembers sets can_restrict_members permission
func (p *PromoteChain) RestrictMembers(v bool) *PromoteChain { p.rest = &v; return p }

// Go executes the promotion request on Bale servers with auto error logging
func (p *PromoteChain) Go() error {
	resolved := p.cc.bot.ResolveChatID(p.cc.chat)

	// Fetch current active member permissions natively to prevent overriding unconfigured fields
	current, errQuery := p.cc.bot.Chat(resolved).Member(p.user).Go()
	if errQuery != nil {
		return fmt.Errorf("failed to retrieve current member rights for merging: %w", errQuery)
	}

	// Resolve fallback values (new administrators implicitly get false for omitted fields)
	fallback := func(field bool) bool {
		if current.Status != "administrator" && current.Status != "creator" {
			return false
		}
		return field
	}

	// Dynamic mapper: prioritized explicit setter, fallback to current active permission
	resolve := func(explicit *bool, currentVal bool) bool {
		if explicit != nil {
			return *explicit
		}
		return fallback(currentVal)
	}

	payload := map[string]any{
		"chat_id":                resolved,
		"user_id":                p.user,
		"can_change_info":        resolve(p.info, current.CanChangeInfo),
		"can_post_messages":      resolve(p.post, current.CanPostMessages),
		"can_edit_messages":      resolve(p.edit, current.CanEditMessages),
		"can_delete_messages":    resolve(p.del, current.CanDeleteMessages),
		"can_manage_video_chats": resolve(p.vChat, current.CanManageVideoChats),
		"can_invite_users":       resolve(p.inv, current.CanInviteUsers),
		"can_restrict_members":   resolve(p.rest, current.CanRestrictMembers),
	}

	p.cc.bot.Log().Info("Outgoing promotion payload compiled").
		Any("resolved_chat_id", resolved).
		Int64("target_user_id", p.user).
		Any("compiled_permissions", payload).
		Go()

	err := p.cc.bot.BaseRequest(p.cc.ctx, "promoteChatMember", payload, nil)
	if err != nil {
		logErr(p.cc.bot, "[Chat Promote Error] ", err)
	}
	return err
}

// Leave requests the bot to leave the current chat group
func (c *ChatChain) Leave() *LeaveChain {
	return &LeaveChain{cc: c}
}

// LeaveChain handles fluent leave actions
type LeaveChain struct {
	cc *ChatChain
}

// Go executes the chat leaving action with auto error logging
func (l *LeaveChain) Go() error {
	resolved := l.cc.bot.ResolveChatID(l.cc.chat)

	// Execute the API request to Bale servers
	err := l.cc.bot.BaseRequest(l.cc.ctx, "leaveChat", map[string]any{
		"chat_id": resolved,
	}, nil)

	if err != nil {
		logErr(l.cc.bot, "[Chat Leave Error] ", err)
	} else {
		// Optional: Log successful departure in the ladder
		l.cc.bot.Log().Warn("🚪 ربات با موفقیت از چت خارج شد").
			Any("chat_target", resolved).
			Go()
	}

	return err
}

// MembersCount initiates a members count retrieval chain
func (c *ChatChain) MembersCount() *MembersCountChain {
	return &MembersCountChain{cc: c}
}

// MembersCountChain handles fluent members count retrieval
type MembersCountChain struct {
	cc *ChatChain
}

// Go executes count request and returns result with auto error logging
func (m *MembersCountChain) Go() (int, error) {
	resolved := m.cc.bot.ResolveChatID(m.cc.chat)
	var count int
	err := m.cc.bot.BaseRequest(m.cc.ctx, "getChatMembersCount", map[string]any{
		"chat_id": resolved,
	}, &count)
	if err != nil {
		logErr(m.cc.bot, "[Chat Members Count Error] ", err)
	}
	return count, err
}

// PinChain handles message pinning configuration fluidly
type PinChain struct {
	cc  *ChatChain
	id  int64
	dur time.Duration
}

// Pin initiates a message pinning chain
func (c *ChatChain) Pin(messageID int64) *PinChain {
	return &PinChain{cc: c, id: messageID}
}

// Temp configures the pinned message to automatically unpin itself after duration expires
func (p *PinChain) Temp(d time.Duration) *PinChain {
	p.dur = d
	return p
}

// Go executes message pinning and schedules unpin with auto error logging
func (p *PinChain) Go() error {
	resolved := p.cc.bot.ResolveChatID(p.cc.chat)
	err := p.cc.bot.BaseRequest(p.cc.ctx, "pinChatMessage", map[string]any{
		"chat_id":    resolved,
		"message_id": p.id,
	}, nil)

	if err == nil && p.dur > 0 {
		p.cc.bot.Task().In(p.dur, func() {
			_ = p.cc.bot.BaseRequest(context.Background(), "unPinChatMessage", map[string]any{
				"chat_id":    resolved,
				"message_id": p.id,
			}, nil)
		})
	}
	if err != nil {
		logErr(p.cc.bot, "[Chat Pin Error] ", err)
	}
	return err
}

// Unpin initiates a specific message unpinning chain
func (c *ChatChain) Unpin(messageID int64) *UnpinChain {
	return &UnpinChain{cc: c, id: messageID}
}

// UnpinChain handles fluent message unpinning
type UnpinChain struct {
	cc *ChatChain
	id int64
}

// Go executes unpinning action with auto error logging
func (u *UnpinChain) Go() error {
	resolved := u.cc.bot.ResolveChatID(u.cc.chat)
	err := u.cc.bot.BaseRequest(u.cc.ctx, "unPinChatMessage", map[string]any{
		"chat_id":    resolved,
		"message_id": u.id,
	}, nil)
	if err != nil {
		logErr(u.cc.bot, "[Chat Unpin Error] ", err)
	}
	return err
}

// UnpinAll unpins all pinned messages in the chat
func (c *ChatChain) UnpinAll() *UnpinAllChain {
	return &UnpinAllChain{cc: c}
}

// UnpinAllChain handles fluent unpinning of all messages
type UnpinAllChain struct {
	cc *ChatChain
}

// Go executes unpin all action with auto error logging
func (ua *UnpinAllChain) Go() error {
	resolved := ua.cc.bot.ResolveChatID(ua.cc.chat)
	err := ua.cc.bot.BaseRequest(ua.cc.ctx, "unpinAllChatMessages", map[string]any{
		"chat_id": resolved,
	}, nil)
	if err != nil {
		logErr(ua.cc.bot, "[Chat Unpin All Error] ", err)
	}
	return err // Fixed: return the single executed request error
}

// Info initiates a chat info retrieval chain
func (c *ChatChain) Info() *InfoChain {
	return &InfoChain{cc: c}
}

// InfoChain handles fluent chat info retrieval
type InfoChain struct {
	cc *ChatChain
}

// Go executes the info retrieval and returns ChatFullInfo with auto error logging
func (ic *InfoChain) Go() (*ChatFullInfo, error) {
	resolved := ic.cc.bot.ResolveChatID(ic.cc.chat)
	var info ChatFullInfo
	err := ic.cc.bot.BaseRequest(ic.cc.ctx, "getChat", map[string]any{
		"chat_id": resolved,
	}, &info)
	if err != nil {
		logErr(ic.cc.bot, "[Chat Info Query Error] ", err)
	}
	return &info, err
}

// Member initiates a member retrieval chain
func (c *ChatChain) Member(userID int64) *MemberChain {
	return &MemberChain{cc: c, user: userID}
}

// MemberChain handles fluent member retrieval
type MemberChain struct {
	cc   *ChatChain
	user int64
}

// Go executes member info retrieval and returns ChatMember with auto error logging
func (m *MemberChain) Go() (*ChatMember, error) {
	resolved := m.cc.bot.ResolveChatID(m.cc.chat)
	var member ChatMember
	err := m.cc.bot.BaseRequest(m.cc.ctx, "getChatMember", map[string]any{
		"chat_id": resolved,
		"user_id": m.user,
	}, &member)
	if err != nil {
		logErr(m.cc.bot, "[Chat Member Query Error] ", err)
	}
	return &member, err
}

// Admins initiates an admins list retrieval chain
func (c *ChatChain) Admins() *AdminsChain {
	return &AdminsChain{cc: c}
}

// AdminsChain handles fluent admins list retrieval
type AdminsChain struct {
	cc *ChatChain
}

// Go executes the admins list retrieval and returns list with auto error logging
func (ac *AdminsChain) Go() ([]ChatMember, error) {
	resolved := ac.cc.bot.ResolveChatID(ac.cc.chat)
	var admins []ChatMember
	err := ac.cc.bot.BaseRequest(ac.cc.ctx, "getChatAdministrators", map[string]any{
		"chat_id": resolved,
	}, &admins)
	if err != nil {
		logErr(ac.cc.bot, "[Chat Admins Query Error] ", err)
	}
	return admins, err
}

// IsAdmin initiates a fluent check if current member is admin
func (c *ChatChain) IsAdmin(userID ...int64) *IsAdminChain {
	return &IsAdminChain{cc: c, user: userID}
}

// IsAdminChain handles fluent verification of admin privileges
type IsAdminChain struct {
	cc   *ChatChain
	user []int64
}

// Go executes check permissions on Bale servers and returns true/false with auto error logging
func (ia *IsAdminChain) Go() (bool, error) {
	// Bypasses the API request if the chat is a private direct message
	if ia.cc.c != nil && ia.cc.c.IsPrivate() {
		return true, nil
	}

	resolved := ia.cc.bot.ResolveChatID(ia.cc.chat)
	var targetUserID int64
	if len(ia.user) > 0 {
		targetUserID = ia.user[0]
	} else if ia.cc.c != nil {
		targetUserID = ia.cc.c.SenderID()
	} else {
		return false, errors.New("cannot determine target user ID")
	}

	// Try loading admin status from cache
	cacheKey := fmt.Sprintf("is_admin:%v:%d", resolved, targetUserID)
	if cachedVal, ok := ia.cc.bot.Cache().Get(cacheKey).Go(); ok {
		if isAdmin, okBool := cachedVal.(bool); okBool {
			return isAdmin, nil
		}
	}

	var member ChatMember
	err := ia.cc.bot.BaseRequest(ia.cc.ctx, "getChatMember", map[string]any{
		"chat_id": resolved,
		"user_id": targetUserID,
	}, &member)
	if err != nil {
		logErr(ia.cc.bot, "[Chat IsAdmin Check Error] ", err)
		return false, err
	}
	isAdmin := member.Status == "administrator" || member.Status == "creator"

	// Cache admin status for 5 minutes
	ia.cc.bot.Cache().Set(cacheKey, isAdmin, 5*time.Minute).Go()

	return isAdmin, nil
}

// InviteLink initiates an invite link creation chain
func (c *ChatChain) InviteLink() *InviteLinkChain {
	return &InviteLinkChain{cc: c}
}

// InviteLinkChain handles fluent invite link creation
type InviteLinkChain struct {
	cc *ChatChain
}

// Go executes the creation of invite link with auto error logging and caching
func (il *InviteLinkChain) Go() (*ChatInviteLink, error) {
	resolved := il.cc.bot.ResolveChatID(il.cc.chat)
	var link ChatInviteLink
	err := il.cc.bot.BaseRequest(il.cc.ctx, "createChatInviteLink", map[string]any{
		"chat_id": resolved,
	}, &link)
	if err == nil && link.InviteLink != "" {
		if cid, ok := resolved.(int64); ok {
			cleanLink := strings.TrimPrefix(link.InviteLink, "http://")
			cleanLink = strings.TrimPrefix(cleanLink, "https://")
			il.cc.bot.inviteCache.Store(cleanLink, cid)
		}
	}
	if err != nil {
		logErr(il.cc.bot, "[Chat Invite Link Create Error] ", err)
	}
	return &link, err
}

// RevokeLink initiates an invite link revocation chain
func (c *ChatChain) RevokeLink(link string) *RevokeLinkChain {
	return &RevokeLinkChain{cc: c, link: link}
}

// RevokeLinkChain handles fluent invite link revocation
type RevokeLinkChain struct {
	cc   *ChatChain
	link string
}

// Go executes revocation with auto error logging and caching
func (rl *RevokeLinkChain) Go() (*ChatInviteLink, error) {
	resolved := rl.cc.bot.ResolveChatID(rl.cc.chat)
	var out ChatInviteLink
	err := rl.cc.bot.BaseRequest(rl.cc.ctx, "revokeChatInviteLink", map[string]any{
		"chat_id":     resolved,
		"invite_link": rl.link,
	}, &out)
	if err == nil && out.InviteLink != "" {
		if cid, ok := resolved.(int64); ok {
			cleanLink := strings.TrimPrefix(out.InviteLink, "http://")
			cleanLink = strings.TrimPrefix(cleanLink, "https://")
			rl.cc.bot.inviteCache.Store(cleanLink, cid)
		}
	}
	if err != nil {
		logErr(rl.cc.bot, "[Chat Invite Link Revoke Error] ", err)
	}
	return &out, err
}

// ExportLink initiates an invite link export chain
func (c *ChatChain) ExportLink() *ExportLinkChain {
	return &ExportLinkChain{cc: c}
}

// ExportLinkChain handles fluent invite link exporting
type ExportLinkChain struct {
	cc *ChatChain
}

// Go executes export with auto error logging and caching
func (el *ExportLinkChain) Go() (string, error) {
	resolved := el.cc.bot.ResolveChatID(el.cc.chat)
	var link string
	err := el.cc.bot.BaseRequest(el.cc.ctx, "exportChatInviteLink", map[string]any{
		"chat_id": resolved,
	}, &link)
	if err == nil && link != "" {
		if cid, ok := resolved.(int64); ok {
			cleanLink := strings.TrimPrefix(link, "http://")
			cleanLink = strings.TrimPrefix(cleanLink, "https://")
			el.cc.bot.inviteCache.Store(cleanLink, cid)
		}
	}
	if err != nil {
		logErr(el.cc.bot, "[Chat Invite Link Export Error] ", err)
	}
	return link, err
}

// CacheLink stores a custom invite link mapped to the chat ID in the bot's memory cache
func (c *ChatChain) CacheLink(link string) *CacheLinkChain {
	return &CacheLinkChain{cc: c, link: link}
}

// CacheLinkChain handles fluent manual cache registrations
type CacheLinkChain struct {
	cc   *ChatChain
	link string
}

// Go executes the manual invite link caching process with auto error logging
func (cl *CacheLinkChain) Go() error {
	if cl.link == "" {
		return errors.New("empty invite link")
	}
	resolved := cl.cc.bot.ResolveChatID(cl.cc.chat)
	if cid, ok := resolved.(int64); ok {
		cleanLink := strings.TrimPrefix(cl.link, "http://")
		cleanLink = strings.TrimPrefix(cleanLink, "https://")
		cl.cc.bot.inviteCache.Store(cleanLink, cid)
	}
	return nil
}

// Context registers a custom parent context to control deadlines or cancellation propagation
func (c *ChatChain) Context(ctx context.Context) *ChatChain {
	if ctx != nil {
		c.ctx = ctx
	}
	return c
}

// InviteChain handles fluent structures of bale system reviews with concurrent multi-user invites
type InviteChain struct {
	cc      *ChatChain
	userIDs []int64
}

// Invite configures the chat chain to invite one or multiple users directly to the group/channel
func (c *ChatChain) Invite(userIDs ...int64) *InviteChain {
	return &InviteChain{
		cc:      c,
		userIDs: userIDs,
	}
}

// Go executes the user invitations safely and concurrently with zero-overhead fallback for single users
func (i *InviteChain) Go() error {
	if len(i.userIDs) == 0 {
		return errors.New("empty user IDs list")
	}
	resolved := i.cc.bot.ResolveChatID(i.cc.chat)

	// Zero-overhead synchronous execution if there is only one user
	if len(i.userIDs) == 1 {
		err := i.cc.bot.BaseRequest(i.cc.ctx, "inviteUser", map[string]any{
			"chat_id": resolved,
			"user_id": i.userIDs[0],
		}, nil)
		if err != nil {
			logErr(i.cc.bot, "[Chat Invite User Error] ", err)
		}
		return err
	}

	// Concurrent multi-user invitations using parallel goroutines and sync.WaitGroup
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error

	for _, id := range i.userIDs {
		wg.Add(1)
		go func(targetID int64) {
			defer wg.Done()
			err := i.cc.bot.BaseRequest(i.cc.ctx, "inviteUser", map[string]any{
				"chat_id": resolved,
				"user_id": targetID,
			}, nil)
			if err != nil {
				logErr(i.cc.bot, "[Chat Invite User Error] ", err)
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
			}
		}(id)
	}

	wg.Wait()
	return firstErr
}

// Restrict initiates a fluent chat member restriction chain
func (c *ChatChain) Restrict(userID int64) *RestrictChain {
	return &RestrictChain{cc: c, user: userID}
}

// RestrictChain handles fluent restrictions for chat members based on Bale schema
type RestrictChain struct {
	cc          *ChatChain
	user        int64
	sendMsg     *bool // can_send_messages
	inviteUsers *bool // can_invite_users
	pinMessages *bool // can_pin_messages (Private groups only)
	changeInfo  *bool // can_change_info (Private groups only)
	duration    time.Duration
}

// SendMessages configures if the user is allowed to send standard messages
func (r *RestrictChain) SendMessages(v bool) *RestrictChain { r.sendMsg = &v; return r }

// InviteUsers configures if the user is allowed to invite other users
func (r *RestrictChain) InviteUsers(v bool) *RestrictChain { r.inviteUsers = &v; return r }

// PinMessages configures if the user is allowed to pin messages (Private groups only)
func (r *RestrictChain) PinMessages(v bool) *RestrictChain { r.pinMessages = &v; return r }

// ChangeInfo configures if the user is allowed to change group info (Private groups only)
func (r *RestrictChain) ChangeInfo(v bool) *RestrictChain { r.changeInfo = &v; return r }

// For sets a native duration from now for how long the restriction will last before auto-lift
func (r *RestrictChain) For(d time.Duration) *RestrictChain { r.duration = d; return r }

// Go executes the chat member restriction on Bale servers with auto error logging
func (r *RestrictChain) Go() error {
	resolved := r.cc.bot.ResolveChatID(r.cc.chat)
	chatID, okChat := resolved.(int64)

	// Fetch current active member permissions natively to prevent overriding unconfigured fields
	current, errQuery := r.cc.bot.Chat(resolved).Member(r.user).Go()
	if errQuery != nil {
		return fmt.Errorf("failed to retrieve current member rights for merging: %w", errQuery)
	}

	// Resolve fallback value based on user's current restriction status (non-restricted users implicitly get true)
	fallback := func(field bool) bool {
		if current.Status != "restricted" && current.Status != "kicked" && current.Status != "left" {
			return true
		}
		return field
	}

	// Dynamic mapper: prioritized explicit setter, fallback to current active permission
	resolve := func(explicit *bool, currentVal bool) bool {
		if explicit != nil {
			return *explicit
		}
		return fallback(currentVal)
	}

	// Prevent redundant getChat requests by using cached chat status
	cacheKey := fmt.Sprintf("chat_is_public:%v", resolved)
	isPublic := false
	if cachedVal, ok := r.cc.bot.Cache().Get(cacheKey).Go(); ok {
		if val, okBool := cachedVal.(bool); okBool {
			isPublic = val
		}
	} else {
		if info, err := r.cc.bot.Chat(resolved).Info().Go(); err == nil && info != nil {
			isPublic = info.Username != ""
			r.cc.bot.Cache().Set(cacheKey, isPublic, 12*time.Hour).Go()
		}
	}

	permissions := map[string]any{
		"can_send_messages": resolve(r.sendMsg, current.CanSendMessages),
		"can_invite_users":  resolve(r.inviteUsers, current.CanInviteUsers),
	}

	// Pin and Info are only configurable and editable in Private groups natively on Bale
	if !isPublic {
		permissions["can_pin_messages"] = resolve(r.pinMessages, current.CanPinMessages)
		permissions["can_change_info"] = resolve(r.changeInfo, current.CanChangeInfo)
	}

	// Log compiled permissions payload before sending to network
	r.cc.bot.Log().Info("Outgoing restriction payload compiled").
		Any("resolved_chat_id", resolved).
		Int64("target_user_id", r.user).
		Any("compiled_permissions", permissions).
		Go()

	err := r.cc.bot.BaseRequest(r.cc.ctx, "restrictChatMember", map[string]any{
		"chat_id":     resolved,
		"user_id":     r.user,
		"permissions": permissions,
	}, nil)

	if err != nil {
		logErr(r.cc.bot, "[Chat Restrict Error] ", err)
		return err
	}

	// Schedule automatic lifting of restrictions using framework's internal Task scheduler
	if r.duration > 0 && okChat {
		botInstance := r.cc.bot
		targetUser := r.user

		botInstance.Task().In(r.duration, func() {
			_ = botInstance.Chat(chatID).Restrict(targetUser).
				SendMessages(true).
				InviteUsers(true).
				PinMessages(true).
				ChangeInfo(true).
				Go()
		})
	}

	return nil
}

// DelMsg initiates a specific message deletion chain inside the chat using its ID
func (c *ChatChain) DelMsg(messageID int64) *DelMsgChain {
	return &DelMsgChain{cc: c, id: messageID}
}

// DelMsgChain handles fluent deletion of a specific message in the chat
type DelMsgChain struct {
	cc *ChatChain
	id int64
}

// Go executes the specific message deletion on Bale servers with auto error logging
func (d *DelMsgChain) Go() error {
	resolved := d.cc.bot.ResolveChatID(d.cc.chat)
	err := d.cc.bot.BaseRequest(d.cc.ctx, "deleteMessage", map[string]any{
		"chat_id":    resolved,
		"message_id": d.id,
	}, nil)
	if err != nil {
		logErr(d.cc.bot, "[Chat Message Delete Error] ", err)
	}
	return err
}

// MuteChain handles temporary chat restriction for a user
type MuteChain struct {
	cc       *ChatChain
	userID   int64
	duration time.Duration
}

// Mute initiates a temporary mute chain for a user
func (c *ChatChain) Mute(userID int64) *MuteChain {
	return &MuteChain{cc: c, userID: userID}
}

// For sets how long the mute should last
func (m *MuteChain) For(d time.Duration) *MuteChain {
	m.duration = d
	return m
}

// Go executes the mute natively on Bale servers and schedules automatic unmute
func (m *MuteChain) Go() error {
	resolved := m.cc.bot.ResolveChatID(m.cc.chat)
	chatID, okChat := resolved.(int64)

	// Compile standard Bale permission keys for restricted member
	perms := map[string]any{
		"can_send_messages": false,
		"can_invite_users":  false,
	}

	err := m.cc.bot.BaseRequest(m.cc.ctx, "restrictChatMember", map[string]any{
		"chat_id":     resolved,
		"user_id":     m.userID,
		"permissions": perms,
	}, nil)

	if err != nil {
		logErr(m.cc.bot, "[Chat Mute Error] ", err)
		return err
	}

	// Schedule automatic unmute natively using framework's internal Task scheduler
	if m.duration > 0 && okChat {
		botInstance := m.cc.bot
		targetUser := m.userID

		botInstance.Task().In(m.duration, func() {
			_ = botInstance.Chat(chatID).Restrict(targetUser).
				SendMessages(true).
				InviteUsers(true).
				PinMessages(true).
				ChangeInfo(true).
				Go()
		})
	}

	return nil
}

// TempBanChain handles temporary ban with automatic unban
type TempBanChain struct {
	cc       *ChatChain
	userID   int64
	duration time.Duration
}

// TempBan initiates a temporary ban chain for a user
func (c *ChatChain) TempBan(userID int64) *TempBanChain {
	return &TempBanChain{cc: c, userID: userID}
}

// For sets how long the ban should last
func (t *TempBanChain) For(d time.Duration) *TempBanChain {
	t.duration = d
	return t
}

// Go executes ban and schedules automatic unban after duration
func (t *TempBanChain) Go() error {
	if err := t.cc.Ban(t.userID).Go(); err != nil {
		return err
	}

	// Schedule automatic unban using framework's internal Task scheduler
	if t.duration > 0 {
		botInstance := t.cc.bot
		targetChat := t.cc.chat
		targetUser := t.userID

		botInstance.Task().In(t.duration, func() {
			_ = botInstance.Chat(targetChat).Unban(targetUser).OnlyIfBanned(true).Go()
		})
	}

	return nil
}

// ActionChain handles fluent chat action states (like typing) using the unified dot system
type ActionChain struct {
	bot    *Bot
	ctx    context.Context
	chat   any
	action string
}

// Action opens the chat action dot chain from the Bot context
func (b *Bot) Action(chat any) *ActionChain {
	return &ActionChain{
		bot:  b,
		ctx:  context.Background(),
		chat: chat,
	}
}

// Action opens the chat action dot chain from the Handler context
func (c *Ctx) Action() *ActionChain {
	id, _ := c.ChatID()
	return &ActionChain{
		bot:  c.Bot,
		ctx:  c.ctx,
		chat: id,
	}
}

// Typing configures the action as typing
func (a *ActionChain) Typing() *ActionChain {
	a.action = "typing"
	return a
}

// UploadPhoto configures the action as upload_photo
func (a *ActionChain) UploadPhoto() *ActionChain {
	a.action = "upload_photo"
	return a
}

// UploadDoc configures the action as upload_document
func (a *ActionChain) UploadDoc() *ActionChain {
	a.action = "upload_document"
	return a
}

// Custom configures any custom or raw action string
func (a *ActionChain) Custom(act string) *ActionChain {
	a.action = act
	return a
}

// Go executes the chat action on Bale servers with auto error logging
func (a *ActionChain) Go() (bool, error) {
	if a.chat == nil {
		return false, errors.New("missing chat destination")
	}
	if a.action == "" {
		return false, errors.New("missing action type")
	}

	resolved := a.bot.ResolveChatID(a.chat)

	// Convert resolved ID to a clean string format to ensure compatibility with Bale's JSON parser
	resolvedStr := fmt.Sprintf("%v", resolved)

	var ok bool
	err := a.bot.BaseRequest(a.ctx, "sendChatAction", map[string]any{
		"chat_id": resolvedStr, // Send as a safe string
		"action":  a.action,
	}, &ok)
	if err != nil {
		logErr(a.bot, "[Chat Action Error] ", err)
	}
	return ok, err
}

// JoinChain handles user join events using the unified fluent dot system
type JoinChain struct {
	bot      *Bot
	msg      string
	dbKey    string
	doSave   bool
	handlers []Handler
}

// Join opens the fluent join event registration chain
func (o *OnChain) Join() *JoinChain {
	return &JoinChain{bot: o.bot}
}

// Msg sets a custom welcome message for joining members
func (j *JoinChain) Msg(text string) *JoinChain {
	j.msg = text
	return j
}

// Save enables saving of the joining member's ID in GOB database
func (j *JoinChain) Save(key ...string) *JoinChain {
	j.doSave = true
	if len(key) > 0 {
		j.dbKey = key[0]
	} else {
		j.dbKey = "group_members"
	}
	return j
}

// Do appends a custom closure handler to execute on member join
func (j *JoinChain) Do(h ...Handler) *JoinChain {
	j.handlers = h
	return j
}

// Go registers the completed join sequence into the bot's router
func (j *JoinChain) Go() {
	handler := func(c *Ctx) {
		chatID, err := c.ChatID()
		if err != nil {
			c.Next()
			return
		}
		nowNs := time.Now().UnixNano()

		for _, user := range c.Message.NewChatMembers {
			joinKey := fmt.Sprintf("join_time_%d_%d", chatID, user.ID)
			_ = c.DB().Set(joinKey, nowNs).Go()

			// Publish user join event to the central EventBus asynchronously
			c.Bot.Bus.Publish("user.join", map[string]any{
				"ChatID": chatID,
				"User":   user,
			})

			// Only credit human inviters
			if c.Message.From != nil && !c.Message.From.IsBot && c.Message.From.ID != user.ID {
				inviterID := c.Message.From.ID
				invitesKey := fmt.Sprintf("invites_%d_%d", chatID, inviterID)
				_ = c.DB().Tx(func(store map[string]any) {
					count := 0
					if val, ok := store[invitesKey]; ok {
						if cVal, ok := val.(int); ok {
							count = cVal
						}
					}
					store[invitesKey] = count + 1
				}).Go()
			}

			if j.doSave && j.dbKey != "" {
				_ = c.DB().Tx(func(store map[string]any) {
					var list []int64
					if val, ok := store[j.dbKey]; ok {
						if l, ok := val.([]int64); ok {
							list = l
						}
					}
					found := false
					for _, id := range list {
						if id == user.ID {
							found = true
							break
						}
					}
					if !found {
						store[j.dbKey] = append(list, user.ID)
					}
				}).Go()
			}

			if j.msg != "" {
				name := user.Mention()
				if user.LastName != "" {
					name += " " + user.LastName
				}
				chatTitle := c.Message.Chat.Title
				text := j.msg
				text = strings.ReplaceAll(text, "{name}", name)
				text = strings.ReplaceAll(text, "{id}", fmt.Sprintf("%d", user.ID))
				text = strings.ReplaceAll(text, "{title}", chatTitle)
				_, _ = c.Send().Text(text).Go()
			}
		}
		c.Next()
	}

	var finalHandlers []Handler
	finalHandlers = append(finalHandlers, handler)
	finalHandlers = append(finalHandlers, j.handlers...)

	o := &OnChain{bot: j.bot}
	o.Callback("_sys_join").Do(finalHandlers...)
}

// ExitChain handles user exit events using the unified fluent dot system
type ExitChain struct {
	bot      *Bot
	msg      string
	dbKey    string
	doRemove bool
	handlers []Handler
}

// Exit opens the fluent exit event registration chain
func (o *OnChain) Exit() *ExitChain {
	return &ExitChain{bot: o.bot}
}

// Msg sets a custom exit message for leaving members
func (e *ExitChain) Msg(text string) *ExitChain {
	e.msg = text
	return e
}

// Remove enables removing of the leaving member's ID from GOB database
func (e *ExitChain) Remove(key ...string) *ExitChain {
	e.doRemove = true
	if len(key) > 0 {
		e.dbKey = key[0]
	} else {
		e.dbKey = "group_members"
	}
	return e
}

// Do appends a custom closure handler to execute on member exit
func (e *ExitChain) Do(h ...Handler) *ExitChain {
	e.handlers = h
	return e
}

// Go registers the completed exit sequence into the bot's router
func (e *ExitChain) Go() {
	handler := func(c *Ctx) {
		user := c.Message.LeftChatMember
		if user != nil {
			// Extract chatID safely from context
			chatID, err := c.ChatID()
			if err != nil {
				c.Next()
				return
			}

			// Remove the user ID atomically from local GOB DB if option is configured
			if e.doRemove && e.dbKey != "" {
				_ = c.DB().Tx(func(store map[string]any) {
					var list []int64
					if val, ok := store[e.dbKey]; ok {
						if l, ok := val.([]int64); ok {
							list = l
						}
					}
					var newList []int64
					for _, id := range list {
						if id != user.ID {
							newList = append(newList, id)
						}
					}
					store[e.dbKey] = newList
				}).Go()
			}

			// Format and send exit message fluidly if option is configured
			if e.msg != "" {
				name := user.Mention() // Updated to use smart Mention() fallback
				if user.LastName != "" {
					name += " " + user.LastName
				}
				chatTitle := c.Message.Chat.Title
				text := e.msg
				text = strings.ReplaceAll(text, "{name}", name)
				text = strings.ReplaceAll(text, "{id}", fmt.Sprintf("%d", user.ID))
				text = strings.ReplaceAll(text, "{title}", chatTitle)
				_, _ = c.Send().Text(text).Go()
			}

			// Publish user exit event to the central EventBus asynchronously
			c.Bot.Bus.Publish("user.exit", map[string]any{
				"ChatID": chatID,
				"User":   *user,
			})
		}
		c.Next()
	}

	var finalHandlers []Handler
	finalHandlers = append(finalHandlers, handler)
	finalHandlers = append(finalHandlers, e.handlers...)

	o := &OnChain{bot: e.bot}
	o.Callback("_sys_exit").Do(finalHandlers...)
}

// Permissions handles global group-wide rights for all members
func (c *ChatChain) Permissions() *PermissionsChain {
	return &PermissionsChain{cc: c}
}

// PermissionsChain holds flags for global chat permissions
type PermissionsChain struct {
	cc      *ChatChain
	sendMsg bool
	sendMed bool
	sendStk bool
	inv     bool
	pin     bool
	info    bool
	call    bool
}

// SendMessages toggles text message permission for all members
func (p *PermissionsChain) SendMessages(v bool) *PermissionsChain { p.sendMsg = v; return p }

// SendMedia toggles photo/video/audio permissions
func (p *PermissionsChain) SendMedia(v bool) *PermissionsChain { p.sendMed = v; return p }

// SendOther toggles stickers and animations
func (p *PermissionsChain) SendOther(v bool) *PermissionsChain { p.sendStk = v; return p }

// InviteUsers toggles member addition permission
func (p *PermissionsChain) InviteUsers(v bool) *PermissionsChain { p.inv = v; return p }

// PinMessages toggles pinning (Only works in Private Groups)
func (p *PermissionsChain) PinMessages(v bool) *PermissionsChain { p.pin = v; return p }

// ChangeInfo toggles group info editing (Only works in Private Groups)
func (p *PermissionsChain) ChangeInfo(v bool) *PermissionsChain { p.info = v; return p }

// Go executes the permission update on Bale servers with local hybrid fallback
func (p *PermissionsChain) Go() error {
	resolved := p.cc.bot.ResolveChatID(p.cc.chat)
	chatID, okChat := resolved.(int64)

	// Prevent redundant getChat requests by using cached chat status
	cacheKey := fmt.Sprintf("chat_is_public:%v", resolved)
	isPublic := false
	if cachedVal, ok := p.cc.bot.Cache().Get(cacheKey).Go(); ok {
		if val, okBool := cachedVal.(bool); okBool {
			isPublic = val
		}
	} else {
		if info, err := p.cc.bot.Chat(resolved).Info().Go(); err == nil && info != nil {
			isPublic = info.Username != ""
			p.cc.bot.Cache().Set(cacheKey, isPublic, 12*time.Hour).Go()
		}
	}

	// Apply virtual group restrictions inside local database for local middleware enforcement
	if okChat && p.cc.bot.dbInstance != nil {
		db := p.cc.bot.dbInstance

		// 1. Text message restriction -> Virtual full group lock
		lockKey := fmt.Sprintf("group_lock_%d", chatID)
		if !p.sendMsg {
			_ = db.Set(lockKey, true)
		} else {
			_ = db.Del(lockKey)
		}

		// 2. Media & Sticker restrictions -> Virtual media guard list
		var localBlocked []string
		if !p.sendMed {
			localBlocked = append(localBlocked, "photo", "video", "voice", "audio", "document", "animation", "location", "contact")
		}
		if !p.sendStk {
			localBlocked = append(localBlocked, "sticker")
		}

		groupKey := fmt.Sprintf("blocked_media_group_%d", chatID)
		if len(localBlocked) > 0 {
			_ = db.Set(groupKey, localBlocked)
		} else {
			_ = db.Del(groupKey)
		}
	}

	perms := map[string]any{
		"can_send_messages":       p.sendMsg,
		"can_send_media_messages": p.sendMed,
		"can_send_other_messages": p.sendStk,
		"can_invite_users":        p.inv,
	}

	// Bale constraint: Pin and Info can only be toggled in Private groups
	if !isPublic {
		perms["can_pin_messages"] = p.pin
		perms["can_change_info"] = p.info
	}

	// Log compiled permissions payload before sending to network
	p.cc.bot.Log().Info("Outgoing global permissions payload compiled").
		Any("resolved_chat_id", resolved).
		Any("compiled_permissions", perms).
		Go()

	// Attempt API call but handle 501 gracefully as virtual fallback is active
	err := p.cc.bot.BaseRequest(p.cc.ctx, "setChatPermissions", map[string]any{
		"chat_id":     resolved,
		"permissions": perms,
	}, nil)

	if err != nil {
		// Gracefully bypass 501 Not Implemented since local database Virtual Shield is fully active
		if strings.Contains(err.Error(), "501") || strings.Contains(err.Error(), "Not Implemented") {
			p.cc.bot.Log().Warn("setChatPermissions API not supported by Bale, virtual database restriction applied instead").Go()
			return nil
		}
		logErr(p.cc.bot, "[Chat Permissions Error] ", err)
		return err
	}
	return nil
}

// RefreshRights triggers a getChatMember call on the bot itself
// to force Bale to update its internal permission cache for this bot.
func (c *ChatChain) RefreshRights() *ChatChain {
	me, err := c.bot.Me().Go()
	if err == nil {
		// Just calling getChatMember is often enough to wake up Bale's permission sync
		_, _ = c.bot.Chat(c.chat).Member(me.ID).Go()
	}
	return c
}

// ForceSync performs a harmless metadata update to force Bale to refresh its access cache.
// This is a workaround for the persistent 403 Forbidden error in some groups.
func (c *ChatChain) ForceSync() *ChatChain {
	// 1. Get current info
	info, err := c.bot.Chat(c.chat).Info().Go()
	if err != nil || info == nil {
		return c
	}

	currentDesc := info.Description
	tempDesc := currentDesc + " ." // Harmless change

	// 2. Flip the description to trigger a server-side cache invalidation
	_ = c.bot.Chat(c.chat).Desc(tempDesc).Go()
	time.Sleep(150 * time.Millisecond)
	_ = c.bot.Chat(c.chat).Desc(currentDesc).Go()

	return c
}

// IsOwnerChain handles fluent verification of native group creator/ownership
type IsOwnerChain struct {
	cc   *ChatChain
	user []int64
}

// IsOwner initiates a fluent check if current member is the native creator/owner of the group chat
func (c *ChatChain) IsOwner(userID ...int64) *IsOwnerChain {
	return &IsOwnerChain{cc: c, user: userID}
}

// Go executes the creator/ownership check on Bale servers and returns true/false with auto error logging
func (io *IsOwnerChain) Go() (bool, error) {
	// Bypasses the API request if the chat is a private direct message
	if io.cc.c != nil && io.cc.c.IsPrivate() {
		return true, nil
	}

	resolved := io.cc.bot.ResolveChatID(io.cc.chat)
	var targetUserID int64
	if len(io.user) > 0 {
		targetUserID = io.user[0]
	} else if io.cc.c != nil {
		targetUserID = io.cc.c.SenderID()
	} else {
		return false, fmt.Errorf("cannot determine target user ID")
	}

	// Try loading creator status from cache
	cacheKey := fmt.Sprintf("is_owner:%v:%d", resolved, targetUserID)
	if cachedVal, ok := io.cc.bot.Cache().Get(cacheKey).Go(); ok {
		if isOwner, okBool := cachedVal.(bool); okBool {
			return isOwner, nil
		}
	}

	var member ChatMember
	err := io.cc.bot.BaseRequest(io.cc.ctx, "getChatMember", map[string]any{
		"chat_id": resolved,
		"user_id": targetUserID,
	}, &member)
	if err != nil {
		logErr(io.cc.bot, "[Chat IsOwner Check Error] ", err)
		return false, err
	}
	isOwner := member.Status == "creator"

	// Cache creator status for 5 minutes
	io.cc.bot.Cache().Set(cacheKey, isOwner, 5*time.Minute).Go()

	return isOwner, nil
}
