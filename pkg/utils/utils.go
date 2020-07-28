package utils

import "github.com/sirupsen/logrus"

func LogrusFiledsUnion(f1, f2 logrus.Fields) logrus.Fields {
	f := make(logrus.Fields)
	for k, v := range f1 {
		f[k] = v
	}
	for k, v := range f2 {
		f[k] = v
	}
	return f
}
