package botdetect

import "strings"

// botSignatures is a list of substrings found in known crawler / bot User-Agents.
// All comparisons are case-insensitive.
var botSignatures = []string{
	// Social media crawlers
	"facebookexternalhit",
	"facebookcatalog",
	"twitterbot",
	"linkedinbot",
	"pinterestbot",
	"whatsapp",
	"telegrambot",
	"slackbot",
	"discordbot",
	"vkshare",
	"line-poker",

	// Search engine crawlers
	"googlebot",
	"bingbot",
	"baiduspider",
	"yandexbot",
	"duckduckbot",
	"sogou",
	"exabot",
	"ia_archiver",

	// SEO / auditing tools
	"semrushbot",
	"ahrefsbot",
	"mj12bot",
	"dotbot",
	"rogerbot",

	// Generic bot signals
	"crawler",
	"spider",
	"scraper",
	"headlesschrome",
	"phantomjs",
	"prerender",
}

// IsBot reports whether the given User-Agent string belongs to a known bot or crawler.
// The check is case-insensitive substring match against a curated list of signatures.
func IsBot(userAgent string) bool {
	if userAgent == "" {
		return false
	}
	ua := strings.ToLower(userAgent)
	for _, sig := range botSignatures {
		if strings.Contains(ua, sig) {
			return true
		}
	}
	return false
}
