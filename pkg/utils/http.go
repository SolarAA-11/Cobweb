package utils

import (
	"fmt"
	"net/url"
	"strings"
)

func GetPathLastPart(str string) string {
	u, e := url.Parse(str)
	if e != nil {
		return ""
	}
	parts := strings.Split(u.Path, "/")
	return parts[len(parts)-1]
}

func GetQueryPart(str string) string {
	u, e := url.Parse(str)
	if e != nil {
		return ""
	}
	return u.RawQuery
}

func GetURLSaveFileName(str string) string {
	u, e := url.Parse(str)
	if e != nil {
		return ""
	}

	parts := strings.Split(u.Path, "/")
	fileName := parts[len(parts)-1]

	if len(u.RawQuery) != 0 {
		fileName += "#"
		for key, vals := range u.Query() {
			for _, val := range vals {
				fileName += fmt.Sprint(key, "=", val, ",")
			}
		}
	}

	return fileName
}
