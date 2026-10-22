package prefilter

import (
	"testing"
	"time"

	"github.com/aalsi-app/aalsi/pkg/model"
)

func TestShouldKeep(t *testing.T) {
	tests := []struct {
		name     string
		msg      model.Message
		expected bool
	}{
		{
			name: "Keep assignment notice",
			msg: model.Message{
				Text: "Operating Systems assignment 2 is due on 18th Oct.",
			},
			expected: true,
		},
		{
			name: "Keep Hindi / Hinglish relative date",
			msg: model.Message{
				Text: "kal subah 10 baje tak assignment submit karna hai",
			},
			expected: true,
		},
		{
			name: "Keep short date message (never drop date)",
			msg: model.Message{
				Text: "28th Oct",
			},
			expected: true,
		},
		{
			name: "Keep short Hinglish date message",
			msg: model.Message{
				Text: "kal parso tak",
			},
			expected: true,
		},
		{
			name: "Keep cancellation",
			msg: model.Message{
				Text: "Today's lab cancelled",
			},
			expected: true,
		},
		{
			name: "Keep Devanagari Hindi notice",
			msg: model.Message{
				Text: "कल परीक्षा है",
			},
			expected: true,
		},
		{
			name: "Drop ok sir",
			msg: model.Message{
				Text: "ok sir",
			},
			expected: false,
		},
		{
			name: "Drop single word acknowledgement",
			msg: model.Message{
				Text: "noted!",
			},
			expected: false,
		},
		{
			name: "Drop laughter",
			msg: model.Message{
				Text: "hahahaha lol",
			},
			expected: false,
		},
		{
			name: "Drop emojis only",
			msg: model.Message{
				Text: "😂😂😂👍",
			},
			expected: false,
		},
		{
			name: "Drop system message",
			msg: model.Message{
				Text:     "Messages are end-to-end encrypted",
				IsSystem: true,
			},
			expected: false,
		},
		{
			name: "Drop media omitted",
			msg: model.Message{
				Text:    "<Media omitted>",
				IsMedia: true,
			},
			expected: false,
		},
		{
			name: "Drop deleted message",
			msg: model.Message{
				Text:      "This message was deleted",
				IsDeleted: true,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.msg.Timestamp = time.Now()
			actual := ShouldKeep(tt.msg)
			if actual != tt.expected {
				t.Errorf("ShouldKeep(%q) = %v, expected %v", tt.msg.Text, actual, tt.expected)
			}
		})
	}
}
