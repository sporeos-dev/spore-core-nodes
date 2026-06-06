// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"spore-shell/internal/utilities"
	"strings"
	"sync"

	spore "github.com/sporeos-dev/spore-client-libs/go"
	"golang.org/x/term"
)

var errInterrupt = errors.New("interrupt")

const appId = "dev.sporeos.shell"

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
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// No raw-mode support (e.g. piped input) — fall back to buffered read.
		outputMutex.Lock()
		currentPrompt = prompt
		waitingForInput = true
		fmt.Print(prompt)
		outputMutex.Unlock()

		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')

		outputMutex.Lock()
		waitingForInput = false
		outputMutex.Unlock()

		return strings.TrimRight(line, "\r\n"), err
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

// parseValueLines returns indented display lines for a response arg value and
// an optional warning string when the value appears malformed.
func parseValueLines(v string) (lines []string, warning string) {
	const indent = "    "

	// Try JSON first — if the value is a valid JSON object or array, format it recursively.
	var jsonVal interface{}
	if err := json.Unmarshal([]byte(v), &jsonVal); err == nil {
		switch jsonVal.(type) {
		case map[string]interface{}, []interface{}:
			return formatJSONLines(jsonVal, indent), ""
		}
	}

	// Single-quoted string: strip quotes if balanced.
	if strings.HasPrefix(v, "'") {
		if len(v) >= 2 && strings.HasSuffix(v, "'") {
			return []string{indent + v[1:len(v)-1]}, ""
		}
		return []string{indent + v}, "unmatched '"
	}
	if strings.HasSuffix(v, "'") {
		return []string{indent + v}, "unmatched '"
	}

	// Remaining double-quote means malformed — client strips valid \"…\" pairs.
	if strings.HasPrefix(v, "\"") || strings.HasSuffix(v, "\"") {
		return []string{indent + v}, "unmatched \""
	}

	// Non-JSON array (Spore [...] syntax).
	if strings.HasPrefix(v, "[") {
		if strings.HasSuffix(v, "]") {
			inner := strings.TrimSpace(v[1 : len(v)-1])
			if inner == "" {
				return []string{indent + "(empty)"}, ""
			}
			for _, item := range splitArgs(inner) {
				lines = append(lines, indent+"- "+item)
			}
			return lines, ""
		}
		return []string{indent + v}, "unmatched ["
	}
	if strings.HasSuffix(v, "]") {
		return []string{indent + v}, "unmatched ]"
	}

	// Non-JSON object (Spore {...} syntax).
	if strings.HasPrefix(v, "{") {
		if strings.HasSuffix(v, "}") {
			inner := strings.TrimSpace(v[1 : len(v)-1])
			if inner == "" {
				return []string{indent + "(empty)"}, ""
			}
			for _, pair := range splitArgs(inner) {
				lines = append(lines, indent+strings.TrimSpace(pair))
			}
			return lines, ""
		}
		return []string{indent + v}, "unmatched {"
	}
	if strings.HasSuffix(v, "}") {
		return []string{indent + v}, "unmatched }"
	}

	// Plain string.
	return []string{indent + v}, ""
}

// printResponse formats and prints a spore Response above the current prompt.
// Lines are joined with \r\n so they render correctly in raw terminal mode.
func printResponse(resp *spore.Response) {
	lines := []string{""}

	handle := ""
	if resp.Handle != "" {
		handle = " ~" + resp.Handle
	}

	// Second line: subject // capture
	subjectLine := "  " + resp.Subject + " // " + resp.Capture

	switch {
	case resp.OK:
		lines = append(lines, "  [ok]"+handle)
		lines = append(lines, subjectLine)
		if len(resp.Args) > 0 {
			lines = append(lines, "  ----------")
			var warnings []string
			for k, v := range resp.Args {
				lines = append(lines, "  "+k)
				valLines, warn := parseValueLines(v)
				lines = append(lines, valLines...)
				if warn != "" {
					warnings = append(warnings, "  "+k+": "+warn)
				}
			}
			if len(warnings) > 0 {
				lines = append(lines, "  ----------")
				lines = append(lines, "  warnings")
				lines = append(lines, warnings...)
			}
		}

	case resp.Cancelled:
		lines = append(lines, "  [cancelled]"+handle)
		lines = append(lines, subjectLine)

	default: // error or custom_error
		errKind := "error"
		if resp.CustomError {
			errKind = "custom_error"
		}
		lines = append(lines, "  ["+errKind+"]"+handle)
		lines = append(lines, subjectLine)
		lines = append(lines, "  ----------")
		lines = append(lines, "  code: "+resp.ErrCode)
		lines = append(lines, "  what: "+resp.ErrWhat)
		if resp.ErrorOrigin != "" {
			lines = append(lines, "  origin: "+string(resp.ErrorOrigin))
		}
	}

	lines = append(lines, "")

	printAbovePrompt(strings.Join(lines, "\r\n"))
}

func main() {

	fmt.Println("Starting Spore CLI")
	fmt.Println("Type (h)elp for list of commands.")

	client := spore.NewClient(appId)

	// // When the hub routes cli.echo back to us, print the received expression
	// // and send the reply.
	// client.HandleRequest("echo", func(call *spore.Call) {
	// 	expression := call.ArgIf("expression", "")
	// 	printAbovePrompt("[echo received: " + expression + "]")
	// 	call.Reply(map[string]string{"echo": expression})
	// })

	// Print all responses that don't match a specific HandleResponse subject.
	client.HandleResponseFallback(func(resp *spore.Response) {
		printResponse(resp)
	})

	status := "disconnected"

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
		input, err := readLine(fmt.Sprintf("[%s]>: ", status))
		if err == errInterrupt {
			break MainLoop
		}
		if err != nil {
			fmt.Println("Error reading input:", err.Error())
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
			client.Close()
			status = "disconnected"
			continue

		case "s":
			fmt.Println("SPORE help...")
			input = "SPORE.help"
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

		if err := client.Send(input); err != nil {
			fmt.Println("Send error:", err.Error())
		}
	}

	//
	// closing
	// application
	//
	client.Close()
	fmt.Println("Exit complete")
}
