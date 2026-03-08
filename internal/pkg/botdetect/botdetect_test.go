package botdetect

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBot(t *testing.T) {
	tests := []struct {
		desc      string
		userAgent string
		expected  bool
	}{
		// Known social media bots
		{
			desc:      "Facebook external hit",
			userAgent: "facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)",
			expected:  true,
		},
		{
			desc:      "Twitterbot",
			userAgent: "Twitterbot/1.0",
			expected:  true,
		},
		{
			desc:      "LinkedInBot",
			userAgent: "LinkedInBot/1.0 (compatible; Mozilla/5.0; Apache-HttpClient +http://www.linkedin.com)",
			expected:  true,
		},
		{
			desc:      "WhatsApp",
			userAgent: "WhatsApp/2.21.11.17 A",
			expected:  true,
		},
		{
			desc:      "Slackbot",
			userAgent: "Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)",
			expected:  true,
		},
		{
			desc:      "Discordbot",
			userAgent: "Mozilla/5.0 (compatible; Discordbot/2.0; +https://discordapp.com)",
			expected:  true,
		},
		{
			desc:      "TelegramBot",
			userAgent: "TelegramBot (like TwitterBot)",
			expected:  true,
		},

		// Search engine bots
		{
			desc:      "Googlebot",
			userAgent: "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			expected:  true,
		},
		{
			desc:      "Bingbot",
			userAgent: "Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
			expected:  true,
		},
		{
			desc:      "Baiduspider",
			userAgent: "Mozilla/5.0 (compatible; Baiduspider/2.0; +http://www.baidu.com/search/spider.html)",
			expected:  true,
		},

		// Generic bot signals
		{
			desc:      "HeadlessChrome",
			userAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/91.0.4472.114 Safari/537.36",
			expected:  true,
		},

		// Real browsers — must NOT be detected as bots
		{
			desc:      "Chrome browser",
			userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			expected:  false,
		},
		{
			desc:      "Firefox browser",
			userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
			expected:  false,
		},
		{
			desc:      "Safari browser",
			userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			expected:  false,
		},
		{
			desc:      "Edge browser",
			userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
			expected:  false,
		},

		// Edge cases
		{
			desc:      "empty user agent",
			userAgent: "",
			expected:  false,
		},
		{
			desc:      "case-insensitive FACEBOOKEXTERNALHIT",
			userAgent: "FACEBOOKEXTERNALHIT/1.1",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := IsBot(tt.userAgent)
			assert.Equal(t, tt.expected, got)
		})
	}
}
