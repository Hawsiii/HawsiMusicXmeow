package bot

import (
	"testing"

	td "github.com/AshokShau/gotdbot"
)

func TestIsAnonymousAdminSender(t *testing.T) {
	const chatID = int64(-1001234567890)
	anonymousAdmin := &td.ChatMember{
		MemberId: &td.MessageSenderUser{UserId: 123},
		Status: &td.ChatMemberStatusAdministrator{
			Rights: &td.ChatAdministratorRights{IsAnonymous: true},
		},
	}
	regularAdmin := &td.ChatMember{
		MemberId: &td.MessageSenderUser{UserId: 456},
		Status: &td.ChatMemberStatusAdministrator{
			Rights: &td.ChatAdministratorRights{IsAnonymous: false},
		},
	}

	tests := []struct {
		name   string
		chatID int64
		sender td.MessageSender
		admins []*td.ChatMember
		want   bool
	}{
		{
			name:   "anonymous admin sends as group",
			chatID: chatID,
			sender: &td.MessageSenderChat{ChatId: chatID},
			admins: []*td.ChatMember{anonymousAdmin},
			want:   true,
		},
		{
			name:   "group sender without anonymous admin",
			chatID: chatID,
			sender: &td.MessageSenderChat{ChatId: chatID},
			admins: []*td.ChatMember{regularAdmin},
		},
		{
			name:   "different chat sender",
			chatID: chatID,
			sender: &td.MessageSenderChat{ChatId: -1009876543210},
			admins: []*td.ChatMember{anonymousAdmin},
		},
		{
			name:   "ordinary user sender",
			chatID: chatID,
			sender: &td.MessageSenderUser{UserId: 123},
			admins: []*td.ChatMember{anonymousAdmin},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isAnonymousAdminMemberSender(test.chatID, test.sender, test.admins); got != test.want {
				t.Errorf("isAnonymousAdminMemberSender() = %t, want %t", got, test.want)
			}
		})
	}
}
