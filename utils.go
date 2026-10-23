//go:build windows

package main

import (
	"fmt"
	"path"
	"runtime"
	"strconv"
	"strings"
)

func NewError(err error) error {
	_, file, line, ok := runtime.Caller(1)
	if ok {
		return fmt.Errorf("%s:%d: %s", path.Base(file), line, err)
	}
	return err
}

func SplitLines(s string) (r []string) {
	for s := range strings.FieldsFuncSeq(s, func(r rune) bool { return r == '\n' }) {
		if s != "" {
			r = append(r, s)
		}
	}
	return
}

func Join(ss ...string) string {
	slice := make([]string, len(ss))
	for i, s := range ss {
		if strings.ContainsRune(s, ' ') {
			slice[i] = strconv.Quote(s)
		} else {
			slice[i] = s
		}
	}
	return strings.Join(slice, " ")
}
