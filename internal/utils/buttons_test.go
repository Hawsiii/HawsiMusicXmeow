package utils

import (
	"testing"

	"github.com/AshokShau/gotdbot"
)

func TestStartMarkupOnlyShowsAddToGroup(t *testing.T) {
	markup := StartMarkup("hawsi_bot")
	if len(markup.Rows) != 1 || len(markup.Rows[0]) != 1 {
		t.Fatalf("expected only the add-to-group button, got %#v", markup.Rows)
	}

	button := markup.Rows[0][0]
	if button.Text != "Aᴅᴅ ᴍᴇ ᴛᴏ ʏᴏᴜʀ ɢʀᴏᴜᴘ" {
		t.Fatalf("unexpected start button text: %q", button.Text)
	}

	urlButton, ok := button.Type.(*gotdbot.InlineKeyboardButtonTypeUrl)
	if !ok || urlButton.Url != "https://t.me/hawsi_bot?startgroup=true" {
		t.Fatalf("expected add-to-group URL button, got %#v", button.Type)
	}
}
