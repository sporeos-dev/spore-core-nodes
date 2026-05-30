// Copyright 2026 mharr
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	spore "github.com/sporeos-dev/spore-client-libs/go"
)

const appId = "dev.sporeos.spore"

// defaultTimeoutMs is the maximum time to wait for a response.
const defaultTimeoutMs = 30_000

func main() {
	args := os.Args[1:]

	// No args or "shell" subcommand → exec into spore-shell via hub.
	if len(args) == 0 || args[0] == "shell" {
		runShell()
		return
	}

	// "spawn <id>" subcommand → rewrite as SPORE.spawn passthrough.
	if args[0] == "spawn" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: spore spawn <node-id>")
			os.Exit(1)
		}
		args = append([]string{"SPORE.spawn", "id=" + args[1]}, args[2:]...)
	}

	cmd := strings.Join(args, " ")

	// Auto-generate a handle if none was supplied.
	hasHandle := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "~") {
			hasHandle = true
			break
		}
	}
	if !hasHandle {
		cmd = cmd + fmt.Sprintf(" ~s%04x", rand.Intn(0x10000))
	}

	client := spore.NewClient(appId)

	if err := client.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, "connection failed:", err.Error())
		os.Exit(1)
	}
	defer client.Close()

	resp, err := client.SendAndWait(cmd, defaultTimeoutMs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err.Error())
		os.Exit(1)
	}

	printResponse(resp)

	if !resp.OK && !resp.Cancelled {
		os.Exit(1)
	}
}

// runShell queries the hub for the spore-shell app path and execs into it.
func runShell() {
	client := spore.NewClient(appId)

	if err := client.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, "connection failed:", err.Error())
		os.Exit(1)
	}

	resp, err := client.SendAndWait(
		fmt.Sprintf("SPORE.node.help node=dev.sporeos.shell ~s%04x", rand.Intn(0x10000)),
		defaultTimeoutMs,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err.Error())
		os.Exit(1)
	}
	if !resp.OK {
		fmt.Fprintln(os.Stderr, "could not resolve spore-shell:", resp.ErrWhat)
		os.Exit(1)
	}

	// Extract the app path from the nodeinfo JSON.
	raw, ok := resp.Args["nodeinfo"]
	if !ok {
		fmt.Fprintln(os.Stderr, "hub returned no nodeinfo for dev.sporeos.shell")
		os.Exit(1)
	}

	var info struct {
		App  string `json:"app"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		fmt.Fprintln(os.Stderr, "failed to parse nodeinfo:", err.Error())
		os.Exit(1)
	}
	if info.App == "" {
		fmt.Fprintln(os.Stderr, "spore-shell has no app path in its manifest")
		os.Exit(1)
	}

	// Resolve the app path: expand ~/, resolve relative against manifest dir.
	exe := info.App
	if strings.HasPrefix(exe, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot resolve home directory:", err.Error())
			os.Exit(1)
		}
		exe = filepath.Join(home, exe[2:])
	} else if !filepath.IsAbs(exe) && info.Path != "" {
		exe = filepath.Join(filepath.Dir(info.Path), exe)
	}

	// Close the client before exec replaces the process.
	client.Close()

	// Replace this process with spore-shell.
	if err := syscall.Exec(exe, []string{"spore-shell"}, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, "exec failed:", err.Error())
		os.Exit(1)
	}
}

// printResponse writes a formatted response to stdout.
func printResponse(resp *spore.Response) {
	handle := ""
	if resp.Handle != "" {
		handle = " ~" + resp.Handle
	}

	fmt.Println()
	switch {
	case resp.OK:
		fmt.Printf("[ok]%s\n", handle)
		fmt.Printf("%s // %s\n", resp.Subject, resp.Capture)
		if len(resp.Args) > 0 {
			fmt.Println("----------")
			keys := make([]string, 0, len(resp.Args))
			for k := range resp.Args {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Println(k)
				for _, line := range parseValueLines(resp.Args[k]) {
					fmt.Println(line)
				}
			}
		}

	case resp.Cancelled:
		fmt.Printf("[cancelled]%s\n", handle)
		fmt.Printf("%s // %s\n", resp.Subject, resp.Capture)

	default:
		errKind := "error"
		if resp.CustomError {
			errKind = "custom_error"
		}
		fmt.Fprintf(os.Stderr, "[%s]%s\n", errKind, handle)
		fmt.Fprintf(os.Stderr, "%s // %s\n", resp.Subject, resp.Capture)
		fmt.Fprintln(os.Stderr, "----------")
		fmt.Fprintln(os.Stderr, "code:", resp.ErrCode)
		fmt.Fprintln(os.Stderr, "what:", resp.ErrWhat)
		if resp.ErrorOrigin != "" {
			fmt.Fprintln(os.Stderr, "origin:", string(resp.ErrorOrigin))
		}
	}
	fmt.Println()
}

// parseValueLines returns indented display lines for a response arg value.
func parseValueLines(v string) []string {
	const indent = "    "

	// Try JSON object or array first.
	var jsonVal interface{}
	if err := json.Unmarshal([]byte(v), &jsonVal); err == nil {
		switch jsonVal.(type) {
		case map[string]interface{}, []interface{}:
			return formatJSONLines(jsonVal, indent)
		}
	}

	// Spore array syntax.
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		inner := strings.TrimSpace(v[1 : len(v)-1])
		if inner == "" {
			return []string{indent + "(empty)"}
		}
		var lines []string
		for _, item := range splitArgs(inner) {
			lines = append(lines, indent+"- "+item)
		}
		return lines
	}

	// Spore object syntax.
	if strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}") {
		inner := strings.TrimSpace(v[1 : len(v)-1])
		if inner == "" {
			return []string{indent + "(empty)"}
		}
		var lines []string
		for _, pair := range splitArgs(inner) {
			lines = append(lines, indent+strings.TrimSpace(pair))
		}
		return lines
	}

	// Quoted strings — strip balanced outer quotes.
	if len(v) >= 2 && ((strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"")) ||
		(strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'"))) {
		return []string{indent + v[1:len(v)-1]}
	}

	return []string{indent + v}
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

// splitArgs splits a comma-separated argument list, respecting nested brackets
// and quoted strings.
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
