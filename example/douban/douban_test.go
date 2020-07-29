package douban

import (
	"testing"

	"github.com/SolarDomo/Cobweb/internal/cobweb"
)

func TestDouban(t *testing.T) {
	rule := &DoubanRule{}
	suit := cobweb.NewTestSuits(t)
	suit.WithFile(rule.InitParse, "list.html", cobweb.H{})
	suit.WithFile(rule.scrapeDetailPage, "detail.html", cobweb.H{
		"Rank": "1",
	})
}
