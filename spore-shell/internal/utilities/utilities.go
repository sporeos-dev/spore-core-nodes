package utilities

import (
	"fmt"
	"strings"
	"unicode"
)

//
//
// HasHandle
//

func HasHandle(s string) bool {
	index := strings.Index(s, "~")

	if index == -1 || 
	   index + 1 == len(s) || 
	   unicode.IsSpace(rune(s[index + 1])) {
		return false
	}

	return true
}

//
//
// AppendHandle
//

func AppendHandle(s string) string {
	count++
	return s + " ~shell-" + fmt.Sprintf("%02d", (count % 100))
}

var count int = 0

func setCount(c int) {
	count = c
}

func getCount() int {
	return count
}
