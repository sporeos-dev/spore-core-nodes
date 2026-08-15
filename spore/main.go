// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	spore "github.com/sporeos-dev/spore-client-libs/spore_go"
	"github.com/sporeos-dev/spore-client-libs/spore_go/response"
)

const appId = "dev.sporeos.spore"

// defaultTimeoutMs is the maximum time to wait for a response.
const defaultTimeoutMs = 30_000

func main() {
	args := os.Args[1:]

	// "help" subcommand → print usage.
	if args[0] == "help" {
		printHelp()
		return
	}

	// "open <node-id>" subcommand → run node in the foreground of this terminal.
	if args[0] == "open" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: spore open <node-id>")
			os.Exit(1)
		}
		runNode(args[1])
		return
	}

	cmd := strings.Join(args, " ")

	// subscribe/unsubscribe require a persistent connection — block them here.
	if args[0] == "SPORE.topic.subscribe" || args[0] == "SPORE.topic.unsubscribe" {
		fmt.Fprintln(os.Stderr, "error: subscribe and unsubscribe require a persistent connection; use spore-shell instead")
		os.Exit(1)
	}

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

	client, err := spore.New(appId)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create client:", err.Error())
		os.Exit(1)
	}

	if err := client.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, "connection failed:", err.Error())
		os.Exit(1)
	}
	defer client.Disconnect()

	err = client.SendRaw(cmd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err.Error())
		os.Exit(1)
	}

	isErr := false
	client.OnResponse(func(resp *response.Response, rerr *response.ResponseError) {
		if rerr != nil {
			isErr = true
		}
		printResponse(resp, rerr)
		client.Disconnect()
	})

	client.Listen()

	if isErr {
		os.Exit(1)
	}
}

// printHelp prints usage information for the spore tool.
func printHelp() {
	fmt.Println()
	fmt.Println("Usage: spore <subcommand> [args]")
	fmt.Println()
	fmt.Println("  open <node-id>   Run a node in the foreground of this terminal")
	fmt.Println("  help             Show this help")
	fmt.Println()
}

// runNode queries the hub for a node's binary path via SPORE.node.help and
// replaces this process with that binary, running it in the foreground.
func runNode(nodeID string) {
	client, err := spore.New(appId)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create client:", err.Error())
		os.Exit(1)
	}

	if err := client.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, "connection failed:", err.Error())
		os.Exit(1)
	}
	exeChan := make(chan string, 1)

	client.OnResponse(func(resp *response.Response, rerr *response.ResponseError) {
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "could not resolve node %q: %s\n", nodeID, rerr.What())
			client.Disconnect()
			os.Exit(1)
		}

		exe, ok := resp.Arg("binary")
		if !ok || strings.TrimSpace(exe) == "" {
			fmt.Fprintf(os.Stderr, "hub returned no binary path for %s\n", nodeID)
			client.Disconnect()
			os.Exit(1)
		}
		exeChan <- strings.TrimSpace(exe)
	})

	go func() {
		err := client.Listen()
		if err != nil {
			fmt.Fprintln(os.Stderr, "listen failed:", err.Error())
			client.Disconnect()
			os.Exit(1)
		}
	}()

	err = client.SendRaw(fmt.Sprintf("SPORE.node.help node=%s ~s%04x", nodeID, rand.Intn(0x10000)))
	if err != nil {
		fmt.Fprintln(os.Stderr, "send failed:", err.Error())
		client.Disconnect()
		os.Exit(1)
	}

	select {
	case exe := <-exeChan:
		client.Disconnect()
		if unquoted, err := strconv.Unquote(exe); err == nil {
			exe = unquoted
		}
		if err := syscall.Exec(exe, []string{nodeID}, os.Environ()); err != nil {
			fmt.Fprintln(os.Stderr, "exec failed:", err.Error())
			client.Disconnect()
			os.Exit(1)
		}
	case <-time.After(5 * time.Second):
		fmt.Fprintf(os.Stderr, "timed out waiting for response\n")
		client.Disconnect()
		os.Exit(1)
	}
}

// printResponse writes a formatted response to stdout.
func printResponse(resp *response.Response, rerr *response.ResponseError) {
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

	fmt.Println()
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
		fmt.Fprintf(os.Stderr, "[%s]%s\n", errKind, handleStr)
		fmt.Fprintf(os.Stderr, "%s // %s\n", subject, capture)
		fmt.Fprintln(os.Stderr, "----------")
		fmt.Fprintln(os.Stderr, "code:", rerr.Code())
		fmt.Fprintln(os.Stderr, "what:", rerr.What())
		if origin != "" {
			fmt.Fprintln(os.Stderr, "origin:", origin)
		}

	case resp != nil && resp.Flag("ok"):
		fmt.Printf("[ok]%s\n", handleStr)
		fmt.Printf("%s // %s\n", subject, capture)
		args := parseRespArgs(resp)
		if len(args) > 0 {
			fmt.Println("----------")
			keys := make([]string, 0, len(args))
			for k := range args {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Println(k)
				for _, line := range parseValueLines(args[k]) {
					fmt.Println(line)
				}
			}
		}

	case resp != nil && resp.Flag("cancelled"):
		fmt.Printf("[cancelled]%s\n", handleStr)
		fmt.Printf("%s // %s\n", subject, capture)
	}
	fmt.Println()
}

// parseRespArgs extracts all non-reserved key=value pairs from a serialized response.
func parseRespArgs(resp *response.Response) map[string]string {
	raw := resp.Serialize()
	fields := splitFields(raw)
	args := make(map[string]string)
	skipArgs := map[string]bool{"capture": true, "code": true, "what": true}
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
		} else if skipFlags[f] || strings.HasPrefix(f, "~") {
			continue
		}
	}
	return args
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
