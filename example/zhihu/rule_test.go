package zhihu

import (
	"testing"

	"github.com/SolarDomo/Cobweb/internal/cobweb"
)

func TestZhihuRule(t *testing.T) {
	r := &zhihuRule{}
	suit := cobweb.NewTestSuits(t)
	suit.WithFile(r.InitParse, "hot.html", cobweb.H{})
}
