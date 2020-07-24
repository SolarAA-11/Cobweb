package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetPathLastPart(t *testing.T) {
	data := [][]string{
		{"https://godoc.org/github.com/valyala/fasthttp", "fasthttp"},
		{"https://www.bilibili.com/video/BV1T54y1B7Yj", "BV1T54y1B7Yj"},
		{"https://www.jb51.net/article/170045.htm", "170045.htm"},
		{"asdawww:aswe000", ""},
		{"https://www.jb51.net/article/分布式.htm", "分布式.htm"},
		{"https://movie.douban.com/top250?start=%d&filter=", "top250"},
	}

	for _, datum := range data {
		assert.Equal(t, GetPathLastPart(datum[0]), datum[1])
	}
}

func TestGetQueryPart(t *testing.T) {
	data := [][]string{
		{"https://godoc.org/github.com/valyala/fasthttp", ""},
		{"https://www.bilibili.com/video/BV1T54y1B7Yj", ""},
		{"https://www.jb51.net/article/170045.htm", ""},
		{"asdawww:aswe000", ""},
		{"https://www.jb51.net/article/分布式.htm", ""},
		{"https://movie.douban.com/top250?start=%d&filter=", "start=%d&filter="},
	}
	for _, datum := range data {
		assert.Equal(t, GetQueryPart(datum[0]), datum[1])
	}
}
