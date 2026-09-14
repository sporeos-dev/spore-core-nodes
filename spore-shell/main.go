// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"spore-shell/internal/utilities"
	"strings"
	"sync"
	"sync/atomic"

	spore "github.com/sporeos-dev/spore-client-libs/spore_go"
	"github.com/sporeos-dev/spore-client-libs/spore_go/publish"
	"github.com/sporeos-dev/spore-client-libs/spore_go/request"
	"github.com/sporeos-dev/spore-client-libs/spore_go/response"
	"golang.org/x/term"
)

var errInterrupt = errors.New("interrupt")

const appId = "dev.sporeos.shell"

// defaultTimeoutMs is used for subscribe/unsubscribe hub handshakes.
const defaultTimeoutMs = 30_000

var outputMutex sync.Mutex

// currentPrompt holds the prompt string so receiveMessages can redraw it
// after printing an incoming message that interrupts the input line.
var currentPrompt string

// waitingForInput is true when the main loop is blocked in readLine.
// printAbovePrompt only redraws the prompt+buffer in that case.
var waitingForInput bool

// inputBuf and inputCursor are the live input state during readLine.
// printAbovePrompt reads these to redraw the user's partially-typed text.
var inputBuf []rune
var inputCursor int

// history stores previously entered non-empty commands.
var history []string

// handleCounter generates unique handle tokens for subscribe/unsubscribe requests.
var handleCounter atomic.Int64

// client is the active Spore client connection.
var client *spore.Client

// status tracks the connection state ("connected" or "disconnected").
var status = "disconnected"

// pendingConfirm holds an incoming "confirm" request while we wait for the
// user to type y/n at the prompt. Guarded by pendingConfirmMu since it is
// set from the listen goroutine (OnRequest) and read/cleared from main().
var pendingConfirmMu sync.Mutex
var pendingConfirm *request.Request

// sendHint sends a hint command with the full typed text, the cursor
// position (in runes), and an eol flag when the cursor is at the end of the line.
func sendHint(fullText string, cursor int) {
	if status != "connected" || client == nil {
		printAbovePrompt("[not connected: cannot fetch hints]")
		return
	}

	escaped := strings.ReplaceAll(fullText, `"`, `\"`)
	cmd := fmt.Sprintf(`hint body="%s" cursor=%d`, escaped, cursor)
	if cursor == len([]rune(fullText)) {
		cmd += " eol"
	}
	cmd = utilities.AppendHandle(cmd)

	if err := client.SendRaw(cmd); err != nil {
		printAbovePrompt("[hint error: " + err.Error() + "]")
	}
}

// printAbovePrompt clears the current input line, prints a message on its own
// line, then redraws the prompt and any partially-typed input so the user can
// keep typing uninterrupted.
//
// Uses \r\n explicitly because readLine puts the terminal in raw mode,
// where a bare \n is only a line-feed (no carriage return). \r\n is safe
// in cooked mode too, so this function works regardless of terminal state.
func printAbovePrompt(msg string) {
	outputMutex.Lock()
	defer outputMutex.Unlock()
	// \r     — move to start of current line
	// \033[K — erase to end of line
	fmt.Print("\r\033[K")
	fmt.Print(msg + "\r\n")
	if waitingForInput {
		fmt.Print(currentPrompt)
		fmt.Print(string(inputBuf))
		// Reposition cursor if it isn't at the end of the buffer.
		back := len(inputBuf) - inputCursor
		if back > 0 {
			fmt.Printf("\033[%dD", back)
		}
	}
}

// redrawInputLine redraws the text portion of the current input line and
// positions the cursor correctly. Must be called with outputMutex held.
func redrawInputLine() {
	promptLen := len([]rune(currentPrompt))
	// Move to start of line, then skip past the prompt.
	fmt.Printf("\r\033[%dC", promptLen)
	// Clear from here to end of line.
	fmt.Print("\033[K")
	// Write the buffer.
	fmt.Print(string(inputBuf))
	// Move cursor back if it is not at the end.
	back := len(inputBuf) - inputCursor
	if back > 0 {
		fmt.Printf("\033[%dD", back)
	}
}

// pathCompletion holds the result of a tab-completion analysis.
type pathCompletion struct {
	matches   []string // filesystem entries that match (full paths, dirs end with /)
	insertAt  int      // rune index in inputBuf where the path value starts
	pathSoFar string   // the path fragment the user has typed so far
}

// findPathCompletion inspects the input buffer up to the cursor. If the cursor
// is inside a token whose value portion starts with /, ~/, or ./ it performs a
// filesystem lookup and returns the matches. Returns nil otherwise.
//
// It handles both plain values (path=/Users/mh/...) and quoted values
// (path="/Users/mh/...) and preserves the ~ prefix in results when the user
// typed a tilde path.
func findPathCompletion(buf []rune, cursor int) *pathCompletion {
	left := buf[:cursor]

	// Find the rune index where the current token starts (scan back to the
	// last unquoted space, or the beginning of the buffer).
	tokenStart := 0
	for i := cursor - 1; i >= 0; i-- {
		if left[i] == ' ' {
			tokenStart = i + 1
			break
		}
	}
	token := left[tokenStart:]

	// Skip past '=' if present (key=value syntax).
	valueStart := 0
	for i, ch := range token {
		if ch == '=' {
			valueStart = i + 1
			break
		}
	}
	// Skip a leading quote character.
	if valueStart < len(token) && (token[valueStart] == '"' || token[valueStart] == '\'') {
		valueStart++
	}

	pathValue := string(token[valueStart:])

	// Only complete when the value looks like a filesystem path.
	if !strings.HasPrefix(pathValue, "/") &&
		!strings.HasPrefix(pathValue, "~/") &&
		!strings.HasPrefix(pathValue, "./") &&
		pathValue != "~" {
		return nil
	}

	// Expand ~ for the filesystem lookup.
	lookupPath := pathValue
	usesTilde := strings.HasPrefix(pathValue, "~")
	if usesTilde {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		if pathValue == "~" {
			// Bare ~ — treat as ~/  so we list the home directory's contents.
			lookupPath = home + "/"
		} else {
			// ~/... — replace the ~ with the real home path.
			lookupPath = home + pathValue[1:]
		}
	}

	// Split the lookup path into directory + name prefix.
	var dir, namePrefix string
	if strings.HasSuffix(lookupPath, "/") {
		dir = lookupPath
		namePrefix = ""
	} else {
		dir = filepath.Dir(lookupPath)
		namePrefix = filepath.Base(lookupPath)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	home, _ := os.UserHomeDir()
	var matches []string
	for _, e := range entries {
		name := e.Name()
		if namePrefix != "" && !strings.HasPrefix(name, namePrefix) {
			continue
		}
		fullPath := filepath.Join(dir, name)
		if e.IsDir() {
			fullPath += "/"
		}
		// Restore the ~ prefix if the user typed a tilde path.
		if usesTilde && home != "" {
			stripped := strings.TrimSuffix(fullPath, "/")
			if stripped == home {
				fullPath = "~/"
			} else if strings.HasPrefix(fullPath, home+"/") {
				rel := fullPath[len(home)+1:]
				fullPath = "~/" + rel
			}
		}
		matches = append(matches, fullPath)
	}

	return &pathCompletion{
		matches:   matches,
		insertAt:  tokenStart + valueStart,
		pathSoFar: pathValue,
	}
}

// longestCommonPrefix returns the longest string that is a prefix of every
// element in strs. Returns "" for an empty slice.
func longestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	prefix := strs[0]
	for _, s := range strs[1:] {
		for !strings.HasPrefix(s, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
}

// readLine puts the terminal into raw mode and reads a single line of input,
// handling arrow keys for history navigation and cursor movement.
//
// Arrow keys:  ↑ / ↓  scroll through history; ← / → move the cursor.
// Editing:     Backspace deletes the character before the cursor.
//              Ctrl+A / Ctrl+E jump to the beginning / end of the line.
//              Ctrl+C cancels the current line (returns "").
//
// Raw mode is automatically restored when the function returns, so normal
// fmt.Println calls in main() between prompts work correctly.
func readLine(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	// stdin is verified to be a tty in main() before the input loop starts,
	// so MakeRaw should never fail here in normal operation.
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", fmt.Errorf("stdin is not a terminal: %w", err)
	}
	defer term.Restore(fd, oldState)

	// Initialise shared state before printing the prompt.
	outputMutex.Lock()
	currentPrompt = prompt
	inputBuf = nil
	inputCursor = 0
	waitingForInput = true
	fmt.Print(prompt)
	outputMutex.Unlock()

	// histIdx points one past the end of history when the user hasn't
	// navigated yet ("current" position).
	histIdx := len(history)
	var savedInput []rune // in-progress text saved before navigating history

	b := make([]byte, 1)
	for {
		_, err := os.Stdin.Read(b)
		if err != nil {
			outputMutex.Lock()
			waitingForInput = false
			outputMutex.Unlock()
			return "", err
		}

		// Escape sequences (arrow keys etc.) are handled outside the main
		// mutex block so we can do additional reads without risk of deadlock.
		if b[0] == 0x1b {
			seq := make([]byte, 2)
			os.Stdin.Read(seq[:1])
			if seq[0] != '[' {
				// Not a CSI sequence we recognise — ignore it.
				continue
			}
			os.Stdin.Read(seq[1:])

			outputMutex.Lock()
			switch seq[1] {
			case 'A': // Up arrow — previous history entry.
				if histIdx > 0 {
					if histIdx == len(history) {
						// Save the in-progress text before we start scrolling.
						savedInput = make([]rune, len(inputBuf))
						copy(savedInput, inputBuf)
					}
					histIdx--
					inputBuf = []rune(history[histIdx])
					inputCursor = len(inputBuf)
					redrawInputLine()
				}
			case 'B': // Down arrow — next history entry.
				if histIdx < len(history) {
					histIdx++
					if histIdx == len(history) {
						// Restore the in-progress text.
						inputBuf = make([]rune, len(savedInput))
						copy(inputBuf, savedInput)
					} else {
						inputBuf = []rune(history[histIdx])
					}
					inputCursor = len(inputBuf)
					redrawInputLine()
				}
			case 'C': // Right arrow — move cursor right.
				if inputCursor < len(inputBuf) {
					inputCursor++
					fmt.Print("\033[C")
				}
			case 'D': // Left arrow — move cursor left.
				if inputCursor > 0 {
					inputCursor--
					fmt.Print("\033[D")
				}
			}
			outputMutex.Unlock()
			continue
		}

		outputMutex.Lock()
		switch b[0] {
		case '\r', '\n': // Enter — return the line.
			waitingForInput = false
			result := string(inputBuf)
			fmt.Print("\r\n")
			outputMutex.Unlock()
			return result, nil

		case 0x7f, 0x08: // Backspace — delete character before cursor.
			if inputCursor > 0 {
				inputBuf = append(inputBuf[:inputCursor-1], inputBuf[inputCursor:]...)
				inputCursor--
				redrawInputLine()
			}

		case 0x01: // Ctrl+A — jump to beginning of line.
			inputCursor = 0
			redrawInputLine()

		case 0x05: // Ctrl+E — jump to end of line.
			inputCursor = len(inputBuf)
			redrawInputLine()

		case 0x03: // Ctrl+C — exit.
			inputBuf = nil
			inputCursor = 0
			waitingForInput = false
			fmt.Print("^C\r\n")
			outputMutex.Unlock()
			return "", errInterrupt

		case 0x09: // Tab — path completion or hint fallback.
			// Analyse the buffer while the mutex is still held (no mutex calls
			// inside findPathCompletion), then release it so printAbovePrompt
			// can acquire it safely when we need to display multiple matches or hints.
			comp := findPathCompletion(inputBuf, inputCursor)
			outputMutex.Unlock()

			var newPath string // the completion string to insert (empty = nothing to do)
			if comp != nil && len(comp.matches) > 0 {
				if len(comp.matches) == 1 {
					// Unambiguous match — insert it directly.
					newPath = comp.matches[0]
				} else {
					// Multiple matches — display them above the prompt and fill
					// the longest common prefix so the user can keep typing.
					matchLines := strings.Join(comp.matches, "\r\n  ")
					printAbovePrompt("  " + matchLines)
					lcp := longestCommonPrefix(comp.matches)
					if len(lcp) > len(comp.pathSoFar) {
						newPath = lcp
					}
				}
			} else {
				// Path completion found no matches — fall back to sending hint.
				sendHint(string(inputBuf), inputCursor)
			}

			// Re-acquire the mutex before touching shared input state.
			outputMutex.Lock()
			if newPath != "" {
				completion := []rune(newPath)
				// Replace the old path fragment (from insertAt to cursor) with
				// the completion, preserving any text that follows the cursor.
				before := append([]rune{}, inputBuf[:comp.insertAt]...)
				after := append([]rune{}, inputBuf[inputCursor:]...)
				inputBuf = append(before, append(completion, after...)...)
				inputCursor = comp.insertAt + len(completion)
				redrawInputLine()
			}
			// Fall through to outputMutex.Unlock() after the switch.

		default:
			if b[0] >= 0x20 { // Printable ASCII — insert at cursor position.
				inputBuf = append(inputBuf, 0)
				copy(inputBuf[inputCursor+1:], inputBuf[inputCursor:])
				inputBuf[inputCursor] = rune(b[0])
				inputCursor++
				redrawInputLine()
			}
		}
		outputMutex.Unlock()
	}
}

// addToHistory appends cmd to the history list, skipping empty strings and
// consecutive duplicates.
func addToHistory(cmd string) {
	if cmd == "" {
		return
	}
	if len(history) > 0 && history[len(history)-1] == cmd {
		return
	}
	history = append(history, cmd)
}

// splitArgs splits s by commas at the top level only, not inside nested
// brackets or quoted strings.
func splitArgs(s string) []string {
	var parts []string
	depth := 0
	inDouble := false
	inSingle := false
	start := 0
	for i, ch := range s {
		switch {
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case (ch == '[' || ch == '{') && !inDouble && !inSingle:
			depth++
		case (ch == ']' || ch == '}') && !inDouble && !inSingle:
			depth--
		case ch == ',' && depth == 0 && !inDouble && !inSingle:
			parts = append(parts, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	if start < len(s) {
		parts = append(parts, strings.TrimSpace(s[start:]))
	}
	return parts
}

// formatJSONLines recursively formats a JSON value into indented display lines.
func formatJSONLines(v interface{}, indent string) []string {
	switch val := v.(type) {
	case map[string]interface{}:
		if len(val) == 0 {
			return []string{indent + "(empty)"}
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var lines []string
		for _, k := range keys {
			subLines := formatJSONLines(val[k], indent+"  ")
			if len(subLines) == 1 {
				lines = append(lines, indent+k+": "+strings.TrimSpace(subLines[0]))
			} else {
				lines = append(lines, indent+k+":")
				lines = append(lines, subLines...)
			}
		}
		return lines
	case []interface{}:
		if len(val) == 0 {
			return []string{indent + "(empty)"}
		}
		// A single-item list is displayed as a plain pair rather than a list.
		if len(val) == 1 {
			return formatJSONLines(val[0], indent)
		}
		var lines []string
		for _, item := range val {
			subLines := formatJSONLines(item, indent+"  ")
			if len(subLines) == 1 {
				lines = append(lines, indent+"- "+strings.TrimSpace(subLines[0]))
			} else {
				lines = append(lines, indent+"-")
				lines = append(lines, subLines...)
			}
		}
		return lines
	case string:
		return []string{indent + val}
	case bool:
		if val {
			return []string{indent + "true"}
		}
		return []string{indent + "false"}
	case nil:
		return []string{indent + "(null)"}
	default:
		return []string{indent + fmt.Sprintf("%v", val)}
	}
}

// parseValue parses a raw response argument value string into display lines or a single value string.
// It returns:
// - isComplex: true if the value is a list, object, or multiline string.
// - singleVal: the scalar value string if !isComplex.
// - lines: indented display lines for list/object/multiline items if isComplex.
// - warning: an optional warning string if quotes or brackets were unmatched.
func parseValue(v string, indent string) (isComplex bool, singleVal string, lines []string, warning string) {
	raw := strings.TrimSpace(v)

	// Check for unmatched outer quotes or brackets.
	if strings.HasPrefix(raw, "'") && !strings.HasSuffix(raw, "'") {
		warning = "unmatched '"
	} else if !strings.HasPrefix(raw, "'") && strings.HasSuffix(raw, "'") {
		warning = "unmatched '"
	} else if strings.HasPrefix(raw, "\"") && !strings.HasSuffix(raw, "\"") {
		warning = "unmatched \""
	} else if !strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		warning = "unmatched \""
	} else if strings.HasPrefix(raw, "[") && !strings.HasSuffix(raw, "]") {
		warning = "unmatched ["
	} else if !strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		warning = "unmatched ]"
	} else if strings.HasPrefix(raw, "{") && !strings.HasSuffix(raw, "}") {
		warning = "unmatched {"
	} else if !strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		warning = "unmatched }"
	}

	// Clean/unwrap surrounding quotes if present.
	clean := raw
	if len(clean) >= 2 && ((strings.HasPrefix(clean, "'") && strings.HasSuffix(clean, "'")) ||
		(strings.HasPrefix(clean, "\"") && strings.HasSuffix(clean, "\""))) {
		clean = clean[1 : len(clean)-1]
	}

	// 1. Try JSON unmarshaling first.
	var jsonVal interface{}
	errClean := json.Unmarshal([]byte(clean), &jsonVal)
	if errClean != nil && clean != raw {
		if json.Unmarshal([]byte(raw), &jsonVal) == nil {
			errClean = nil
		}
	}

	if errClean == nil {
		// If jsonVal is a string, it might be a nested JSON or Spore encoded array/object
		if str, ok := jsonVal.(string); ok {
			strTrim := strings.TrimSpace(str)
			var nestedVal interface{}
			if (strings.HasPrefix(strTrim, "[") && strings.HasSuffix(strTrim, "]")) ||
				(strings.HasPrefix(strTrim, "{") && strings.HasSuffix(strTrim, "}")) {
				if json.Unmarshal([]byte(strTrim), &nestedVal) == nil {
					jsonVal = nestedVal
				}
			}
		}

		switch val := jsonVal.(type) {
		case map[string]interface{}, []interface{}:
			valLines := formatJSONLines(val, indent)
			if len(valLines) == 1 {
				return false, strings.TrimSpace(valLines[0]), nil, warning
			}
			return true, "", valLines, warning
		case string:
			if strings.Contains(val, "\n") {
				var multiline []string
				for _, l := range strings.Split(val, "\n") {
					multiline = append(multiline, indent+l)
				}
				return true, "", multiline, warning
			}
			return false, val, nil, warning
		case bool:
			if val {
				return false, "true", nil, warning
			}
			return false, "false", nil, warning
		case nil:
			return false, "(null)", nil, warning
		default:
			return false, fmt.Sprintf("%v", val), nil, warning
		}
	}

	// 2. Try Spore array syntax [...]
	if strings.HasPrefix(clean, "[") && strings.HasSuffix(clean, "]") {
		inner := strings.TrimSpace(clean[1 : len(clean)-1])
		if inner == "" {
			return true, "", []string{indent + "(empty)"}, warning
		}
		items := splitArgs(inner)
		unwrap := func(item string) string {
			item = strings.TrimSpace(item)
			if len(item) >= 2 && ((strings.HasPrefix(item, "\"") && strings.HasSuffix(item, "\"")) ||
				(strings.HasPrefix(item, "'") && strings.HasSuffix(item, "'"))) {
				item = item[1 : len(item)-1]
			}
			return item
		}
		// A single-item list is displayed as a plain pair rather than a list.
		if len(items) == 1 {
			return false, unwrap(items[0]), nil, warning
		}
		var itemLines []string
		for _, item := range items {
			itemLines = append(itemLines, indent+"- "+unwrap(item))
		}
		return true, "", itemLines, warning
	}

	// 3. Try Spore object syntax {...}
	if strings.HasPrefix(clean, "{") && strings.HasSuffix(clean, "}") {
		inner := strings.TrimSpace(clean[1 : len(clean)-1])
		if inner == "" {
			return true, "", []string{indent + "(empty)"}, warning
		}
		var pairLines []string
		for _, pair := range splitArgs(inner) {
			pair = strings.TrimSpace(pair)
			eachIdx := strings.IndexAny(pair, "=:")
			if eachIdx < 0 {
				pairLines = append(pairLines, indent+pair)
				continue
			}
			key := strings.TrimSpace(pair[:eachIdx])
			val := strings.TrimSpace(pair[eachIdx+1:])
			if len(val) >= 2 && ((strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
				(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'"))) {
				val = val[1 : len(val)-1]
			}
			pairLines = append(pairLines, indent+key+": "+val)
		}
		return true, "", pairLines, warning
	}

	// 4. Plain scalar string value
	if strings.Contains(clean, "\n") {
		var multiline []string
		for _, l := range strings.Split(clean, "\n") {
			multiline = append(multiline, indent+l)
		}
		return true, "", multiline, warning
	}

	return false, clean, nil, warning
}

// maxLineWidth is the target wrap width for long single-line values.
const maxLineWidth = 100

// wrapKeyValue formats "key: value" as one or more lines, word-wrapping value
// at maxLineWidth and indenting continuation lines under the value's column.
func wrapKeyValue(baseIndent, key, value string) []string {
	prefix := baseIndent + key + ": "
	words := strings.Fields(value)
	if len(words) == 0 {
		return []string{strings.TrimRight(prefix, " ")}
	}
	hang := strings.Repeat(" ", len(prefix))
	var lines []string
	cur := prefix
	first := true
	for _, w := range words {
		candidate := cur
		if !first {
			candidate += " "
		}
		candidate += w
		if !first && len(candidate) > maxLineWidth {
			lines = append(lines, cur)
			cur = hang + w
			continue
		}
		cur = candidate
		first = false
	}
	lines = append(lines, cur)
	return lines
}

// printResponse formats and prints a spore Response above the current prompt.
// Lines are joined with \r\n so they render correctly in raw terminal mode.
func printResponse(resp *response.Response, rerr *response.ResponseError) {
	if rerr == nil && resp != nil && resp.Flag("ok") && resp.Command() == "hint" {
		printHintResponse(resp)
		return
	}

	lines := []string{""}

	var handle, subject, capture string
	if rerr != nil {
		handle = rerr.Handle()
		subject = rerr.Command()
		capture = rerr.ArgIf("capture", "")
	} else if resp != nil {
		handle = resp.Handle()
		subject = resp.Command()
		capture = resp.ArgIf("capture", "")
	}

	handleStr := ""
	if handle != "" {
		handleStr = " ~" + handle
	}
	subjectLine := "  " + subject + " // " + capture

	switch {
	case rerr != nil:
		errKind := "error"
		if rerr.Flag("custom_error") {
			errKind = "custom_error"
		}
		origin := ""
		switch {
		case rerr.Flag("node_error"):
			origin = "node_error"
		case rerr.Flag("spore_error"):
			origin = "spore_error"
		case rerr.Flag("cast_error"):
			origin = "cast_error"
		case rerr.Flag("capture_error"):
			origin = "capture_error"
		}
		lines = append(lines, "  ["+errKind+"]"+handleStr)
		lines = append(lines, subjectLine)
		lines = append(lines, "  ----------")
		lines = append(lines, "  code: "+rerr.Code())
		if module := rerr.ArgIf("module", ""); module != "" {
			lines = append(lines, "  module: "+module)
		}
		lines = append(lines, "  what: "+rerr.What())
		if extra := rerr.ArgIf("extra", ""); extra != "" && extra != "[]" {
			lines = append(lines, "  extra: "+extra)
		}
		if origin != "" {
			lines = append(lines, "  origin: "+origin)
		}

	case resp != nil && resp.Flag("ok"):
		lines = append(lines, "  [ok]"+handleStr)
		lines = append(lines, subjectLine)
		args, flags := parseRespArgs(resp)
		if len(args) > 0 || len(flags) > 0 {
			lines = append(lines, "  ----------")
			var warnings []string
			keys := make([]string, 0, len(args))
			for k := range args {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				isComplex, singleVal, valLines, warn := parseValue(args[k], "    ")
				if isComplex {
					lines = append(lines, "  "+k+":")
					lines = append(lines, valLines...)
				} else {
					lines = append(lines, wrapKeyValue("  ", k, singleVal)...)
				}
				if warn != "" {
					warnings = append(warnings, "  "+k+": "+warn)
				}
			}
			for _, f := range flags {
				lines = append(lines, "  "+f)
			}
			if len(warnings) > 0 {
				lines = append(lines, "  ----------")
				lines = append(lines, "  warnings")
				lines = append(lines, warnings...)
			}
		}

	case resp != nil && resp.Flag("cancelled"):
		lines = append(lines, "  [cancelled]"+handleStr)
		lines = append(lines, subjectLine)
	}

	lines = append(lines, "")
	printAbovePrompt(strings.Join(lines, "\r\n"))
}

// printHintResponse formats a hint response as a compact block: the
// typed query as a title line, a divider, then the hint fields — with no
// surrounding blank lines.
func printHintResponse(resp *response.Response) {
	lines := []string{"[connected]>: " + resp.ArgIf("body", ""), "  hint // " + resp.ArgIf("body", ""), "  ----------"}

	args, flags := parseRespArgs(resp)
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k == "body" {
			continue
		}
		isComplex, singleVal, valLines, _ := parseValue(args[k], "    ")
		if isComplex {
			lines = append(lines, "  "+k+":")
			lines = append(lines, valLines...)
		} else {
			lines = append(lines, wrapKeyValue("  ", k, singleVal)...)
		}
	}
	for _, f := range flags {
		lines = append(lines, "  "+f)
	}

	lines = append(lines, "")
	printAbovePrompt(strings.Join(lines, "\r\n"))
}

// parseRespArgs extracts all non-reserved key=value pairs and flags from a
// serialized response.
func parseRespArgs(resp *response.Response) (map[string]string, []string) {
	raw := resp.Serialize()
	fields := splitFields(raw)
	args := make(map[string]string)
	var flags []string
	skipArgs := map[string]bool{"capture": true, "code": true, "what": true, "cast": true}
	skipFlags := map[string]bool{
		"ok": true, "cancelled": true, "error": true, "custom_error": true,
		"node_error": true, "spore_error": true, "cast_error": true, "capture_error": true,
	}
	for _, f := range fields[1:] { // skip the ~handle:command token
		if strings.Contains(f, "=") {
			kv := strings.SplitN(f, "=", 2)
			if skipArgs[kv[0]] {
				continue
			}
			v := kv[1]
			if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
				v = v[1 : len(v)-1]
			}
			args[kv[0]] = v
		} else if !skipFlags[f] && !strings.HasPrefix(f, "~") {
			flags = append(flags, f)
		}
	}
	return args, flags
}

// splitFields splits a Spore wire string by spaces, respecting quoted strings
// and nested [array] / {object} tokens.
func splitFields(s string) []string {
	var fields []string
	var current strings.Builder
	inDouble := false
	depth := 0

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '"' && !inDouble:
			inDouble = true
			current.WriteByte(ch)
		case ch == '\\' && inDouble && i+1 < len(s):
			current.WriteByte(ch)
			current.WriteByte(s[i+1])
			i++
		case ch == '"' && inDouble:
			inDouble = false
			current.WriteByte(ch)
		case (ch == '[' || ch == '{') && !inDouble:
			depth++
			current.WriteByte(ch)
		case (ch == ']' || ch == '}') && !inDouble:
			depth--
			current.WriteByte(ch)
		case ch == ' ' && !inDouble && depth == 0:
			if current.Len() > 0 {
				fields = append(fields, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		fields = append(fields, current.String())
	}
	return fields
}

// parsePublishData extracts args (excluding cast) and flags from a publish message.
func parsePublishData(p *publish.Publish) (map[string]string, []string) {
	raw := p.Serialize()
	fields := splitFields(raw)
	args := make(map[string]string)
	var flags []string
	// fields[0]="publish", fields[1]=topic, rest is content
	for _, f := range fields[2:] {
		if strings.Contains(f, "=") {
			kv := strings.SplitN(f, "=", 2)
			if kv[0] == "cast" {
				continue
			}
			v := kv[1]
			if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
				v = v[1 : len(v)-1]
			}
			args[kv[0]] = v
		} else {
			flags = append(flags, f)
		}
	}
	return args, flags
}

// printPublishMessage formats an incoming pub/sub message and prints it above
// the current prompt, just like a regular response.
func printPublishMessage(p *publish.Publish) {
	lines := []string{""}
	topic := p.Topic()
	lines = append(lines, "  [publish] "+topic)
	args, flags := parsePublishData(p)
	if len(args) > 0 || len(flags) > 0 {
		lines = append(lines, "  ----------")
		keys := make([]string, 0, len(args))
		for k := range args {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			isComplex, singleVal, valLines, _ := parseValue(args[k], "    ")
			if isComplex {
				lines = append(lines, "  "+k+":")
				lines = append(lines, valLines...)
			} else {
				lines = append(lines, wrapKeyValue("  ", k, singleVal)...)
			}
		}
		for _, f := range flags {
			lines = append(lines, "  "+f)
		}
	}
	lines = append(lines, "")
	printAbovePrompt(strings.Join(lines, "\r\n"))
}

// extractTopicArg extracts the value of topic=<value> from a command string.
func extractTopicArg(cmd string) string {
	for _, field := range strings.Fields(cmd) {
		if strings.HasPrefix(field, "topic=") {
			return strings.TrimPrefix(field, "topic=")
		}
	}
	return ""
}

// confirmRespondPrompt is the fixed prompt shown while a confirm request is
// pending; the question itself is printed as a block above it.
const confirmRespondPrompt = "  [respond][y/N]>: "

// handleIncomingRequest processes commands the hub routes to this node's own
// API. Currently only "confirm" is handled: the request is stashed in
// pendingConfirm, the body/lines are printed as an indented block, and the
// confirmRespondPrompt is shown in place of the normal shell prompt until
// the user answers.
func handleIncomingRequest(r *request.Request) {
	if !strings.HasSuffix(r.Command(), "confirm") {
		return
	}

	lines := []string{"", "  " + r.ArgIf("body", "")}
	if linesRaw, ok := r.Arg("lines"); ok {
		for _, l := range parseConfirmLines(linesRaw) {
			lines = append(lines, "    "+l)
		}
	}

	pendingConfirmMu.Lock()
	pendingConfirm = r
	pendingConfirmMu.Unlock()

	// Switch to the respond prompt so it redraws in place of whatever the
	// terminal was previously displaying.
	outputMutex.Lock()
	currentPrompt = confirmRespondPrompt
	inputBuf = nil
	inputCursor = 0
	outputMutex.Unlock()

	printAbovePrompt(strings.Join(lines, "\r\n"))
}

// parseConfirmLines splits a Spore array value (e.g. ["what", "who"]) into
// its plain unquoted string entries, with no added bullet formatting.
func parseConfirmLines(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")

	var out []string
	for _, item := range splitArgs(raw) {
		item = strings.TrimSpace(item)
		if len(item) >= 2 && ((strings.HasPrefix(item, "\"") && strings.HasSuffix(item, "\"")) ||
			(strings.HasPrefix(item, "'") && strings.HasSuffix(item, "'"))) {
			item = item[1 : len(item)-1]
		}
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func main() {

	fmt.Println("Starting Spore CLI")
	fmt.Println("Type (h)elp for list of commands.")

	client = spore.New(appId)

	// Don't use WithDefaultErrorHandler(): it writes straight to stdout with
	// plain fmt.Printf, bypassing outputMutex and never redrawing the prompt.
	// Since parse errors arrive on the listen goroutine while readLine is
	// blocked in raw mode, that corrupts the display and makes the shell
	// look hung. Route through printAbovePrompt instead, like every other
	// inbound message.
	client.OnParseError(func(code, what, raw string) {
		printAbovePrompt("[parse error] " + code + ": " + what + "\r\n  raw: " + raw)
	})

	// The hub routes our own "confirm" API command back to us when another
	// node asks the user to confirm an action.
	client.OnRequest(func(r *request.Request) {
		handleIncomingRequest(r)
	})

	client.OnResponse(func(resp *response.Response, rerr *response.ResponseError) {
		printResponse(resp, rerr)
	})

	client.OnPublish(func(p *publish.Publish) {
		printPublishMessage(p)
	})

	status = "disconnected"

	fmt.Println("Connecting to socket.")
	if err := client.Connect(); err != nil {
		fmt.Println("Connection failed:", err.Error())
	} else {
		status = "connected"
		go client.Listen()
	}

	MainLoop:
	for {

		//
		// get next line
		//
		prompt := fmt.Sprintf("[%s]>: ", status)
		pendingConfirmMu.Lock()
		hasPendingConfirm := pendingConfirm != nil
		if hasPendingConfirm {
			prompt = confirmRespondPrompt
		}
		pendingConfirmMu.Unlock()

		input, err := readLine(prompt)
		if err == errInterrupt {
			break MainLoop
		}
		if err != nil {
			fmt.Println("Error reading input:", err.Error())
			continue
		}

		//
		// answer a pending confirm request, if any, instead of treating
		// this line as a normal shell/hub command
		//
		pendingConfirmMu.Lock()
		pc := pendingConfirm
		pendingConfirm = nil
		pendingConfirmMu.Unlock()

		if pc != nil {
			answer := strings.ToLower(strings.TrimSpace(input))
			resp := response.New(pc.Command(), pc.Handle())
			if answer == "y" || answer == "yes" {
				resp = resp.WithFlag("yes")
			} else {
				resp = resp.WithFlag("no")
			}
			if err := client.SendResponse(resp); err != nil {
				fmt.Println("Send error:", err.Error())
			}
			continue
		}

		addToHistory(input)

		//
		// handle cli commands
		//

		switch input {

		// help
		case "h":
			fmt.Println("Commands:")
			fmt.Println(" - (h)elp")
			fmt.Println(" - (q)uit")
			fmt.Println(" - (c)onnect")
			fmt.Println(" - (d)isconnect")
			fmt.Println(" - (s)pore help")
			continue

		// quit
		case "q":
			fmt.Println("Quitting...")
			break MainLoop

		// connect
		case "c":
			fmt.Println("Connecting...")
			if status == "connected" {
				fmt.Println("Already connected")
				continue
			}
			if err := client.Connect(); err != nil {
				fmt.Println("Failed to connect:", err.Error())
				continue
			}
			status = "connected"
			go client.Listen()
			continue

		// disconnect
		case "d":
			fmt.Println("Disconnecting...")
			if status == "disconnected" {
				fmt.Println("Not connected")
				continue
			}
			client.Disconnect()
			status = "disconnected"
			continue

		case "s":
			fmt.Println("SPORE help...")
			input = "help"
		}

		//
		// send command to the hub
		//
		if status == "disconnected" {
			fmt.Println("Not connected")
			continue
		}

		if !utilities.HasHandle(input) {
			input = utilities.AppendHandle(input)
		}

		if err := client.SendRaw(input); err != nil {
			fmt.Println("Send error:", err.Error())
		}
	}

	//
	// closing
	// application
	//
	client.Disconnect()
	fmt.Println("Exit complete")
}
