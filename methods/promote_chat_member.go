package methods

type PromoteChatMember struct {
	ChatID              any   `json:"chat_id"`
	UserID              int64 `json:"user_id"`
	CanChangeInfo       bool  `json:"can_change_info,omitempty"`
	CanPostMessages     bool  `json:"can_post_messages,omitempty"`
	CanEditMessages     bool  `json:"can_edit_messages,omitempty"`
	CanDeleteMessages   bool  `json:"can_delete_messages,omitempty"`
	CanManageVideoChats bool  `json:"can_manage_video_chats,omitempty"`
	CanInviteUsers      bool  `json:"can_invite_users,omitempty"`
	CanRestrictMembers  bool  `json:"can_restrict_members,omitempty"`
}

func (p PromoteChatMember) Method() string {
	return "promoteChatMember"
}

func (p PromoteChatMember) Params() any {
	return p
}