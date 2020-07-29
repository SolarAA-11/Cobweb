package meizitu

import (
	"testing"

	"github.com/SolarDomo/Cobweb/internal/cobweb"
)

func TestParseRule(t *testing.T) {
	rule := &MeizituRule{}
	suit := cobweb.NewTestSuits(t)
	suit.WithFile(rule.InitParse, "listpage.html", cobweb.H{})
	suit.WithFile(rule.parseDetailPage, "detail.html", cobweb.H{})
}
