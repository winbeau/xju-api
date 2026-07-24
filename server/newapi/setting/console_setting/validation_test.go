package console_setting

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestValidateAnnouncementsAllowsLongMarkdownAndHTML(t *testing.T) {
	content := strings.Repeat("这是一段 **Markdown** 与 <strong>HTML</strong> 公告内容。\n", 200)
	extra := strings.Repeat("补充说明\n", 200)
	payload, err := json.Marshal([]map[string]interface{}{
		{
			"id":          1,
			"content":     content,
			"publishDate": time.Now().UTC().Format(time.RFC3339),
			"type":        "success",
			"extra":       extra,
		},
	})
	if err != nil {
		t.Fatalf("marshal announcement: %v", err)
	}

	if err := ValidateConsoleSettings(string(payload), "Announcements"); err != nil {
		t.Fatalf("expected long announcement to be accepted, got %v", err)
	}
}
