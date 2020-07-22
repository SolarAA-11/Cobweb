package models

import (
	"fmt"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpproxy"
)

type AnonymityLevel int

const (
	Transparent AnonymityLevel = iota
	Anonymous
	Elite
)

// 代理结构体
type Proxy struct {
	ID int `gorm:"PRIMARY_KEY;AUTO_INCREMENT;"`

	Url       string
	Port      string
	HTTPS     bool
	Anonymity AnonymityLevel

	Score int
}

func (this *Proxy) GetProxyURL() string {
	schema := "http"
	if this.HTTPS {
		schema = "https"
	}
	return fmt.Sprintf("%s://%s:%s", schema, this.Url, this.Port)
}

func (this *Proxy) FastHTTPDialHTTPProxy() fasthttp.DialFunc {
	return fasthttpproxy.FasthttpHTTPDialer(fmt.Sprintf("%s:%s", this.Url, this.Port))
}

func (this *Proxy) Equal(proxy *Proxy) bool {
	return this.Url == proxy.Url && this.Port == proxy.Port
}
