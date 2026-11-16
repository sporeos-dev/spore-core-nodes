// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

package utilities

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

//
//
// HasHandle
//

func HasHandle(s string) bool {
	_, hasHandle := requestParts(s)
	return hasHandle
}

//
//
// AppendHandle
//

func AppendHandle(s string) string {
	count++
	handle := " ~shell-" + fmt.Sprintf("%02d", (count%100))
	pipe, _ := requestParts(s)
	if pipe < 0 {
		return s + handle
	}
	return strings.TrimRightFunc(s[:pipe], unicode.IsSpace) + handle + " " + s[pipe:]
}

func requestParts(s string) (int, bool) {
	firstPipe := -1
	hasHandle := false
	var quote rune
	var delimiters []rune
	escaped := false
	inBlock := false
	lineStart := false
	tokenStart := true

	for i, r := range s {
		if inBlock {
			if lineStart && strings.HasPrefix(s[i:], ">>") {
				next, _ := utf8.DecodeRuneInString(s[i+2:])
				if i+2 == len(s) || unicode.IsSpace(next) {
					inBlock = false
				}
			}
			lineStart = r == '\n'
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if quote == '"' && r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		if len(delimiters) == 0 && r == '<' && i > 0 && s[i-1] == '=' && strings.HasPrefix(s[i:], "<<") {
			inBlock = true
			lineStart = false
			tokenStart = false
			continue
		}
		switch r {
		case '"', '\'':
			quote = r
		case '[':
			delimiters = append(delimiters, ']')
		case '{':
			delimiters = append(delimiters, '}')
		case ']', '}':
			if len(delimiters) > 0 && delimiters[len(delimiters)-1] == r {
				delimiters = delimiters[:len(delimiters)-1]
			}
		case '|':
			if len(delimiters) == 0 && firstPipe < 0 {
				firstPipe = i
			}
		case '~':
			next, _ := utf8.DecodeRuneInString(s[i+1:])
			if len(delimiters) == 0 && tokenStart && i+1 < len(s) && !unicode.IsSpace(next) {
				hasHandle = true
			}
		}
		tokenStart = len(delimiters) == 0 && unicode.IsSpace(r)
	}
	return firstPipe, hasHandle
}

var count int = 0

func setCount(c int) {
	count = c
}
