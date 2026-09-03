//go:build windows

package main

import (
	"fmt"
	"path"
	"runtime"
	"strings"
)

func NewError(err error) error {
	_, file, line, ok := runtime.Caller(1)
	if ok {
		return fmt.Errorf("%s:%d: %s", path.Base(file), line, err)
	}
	return err
}

func SplitNewline(s string) (r []string) {
	for s := range strings.FieldsFuncSeq(s, func(r rune) bool { return r == '\r' || r == '\n' }) {
		if s != "" {
			r = append(r, s)
		}
	}
	return
}
