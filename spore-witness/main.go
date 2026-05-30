// Copyright 2026 mharr
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	spore "github.com/sporeos-dev/spore-client-libs/go"
)

const appId = "dev.sporeos.witness"

// ANSI color codes for terminal output.
const (
	colorReset     = "\033[0m"
	colorGray      = "\033[90m"
	colorGreen     = "\033[92m" // bright green
	colorMagenta   = "\033[35m" // magenta
	colorBlue      = "\033[34m"
	colorCyan      = "\033[36m"
	colorRed       = "\033[91m" // bright red
)

func main() {
	client := spore.NewClient(appId)

	client.HandleWitness(func(msg *spore.WitnessMessage) {
		t := time.UnixMilli(msg.SporeTime).Local().Format("15:04:05.000")
		color, label := kindMeta(msg.Kind)
		body := formatBody(msg)
		fmt.Printf("%s%s  %s%s  %s\n", color, t, label, colorReset, body)
	})

	if err := client.Connect(); err != nil {
		log.Fatal("connect:", err)
	}
	defer client.Close()

	fmt.Println("spore-witness: connected, watching hub traffic...")

	for {
		if err := client.Listen(); err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				break
			}
			fmt.Println("spore-witness: disconnected, reconnecting...")
		}
		client.Close()

		for {
			time.Sleep(5 * time.Second)
			if err := client.Connect(); err != nil {
				fmt.Println("spore-witness: reconnect failed, retrying in 5s...")
				continue
			}
			fmt.Println("spore-witness: reconnected, watching hub traffic...")
			break
		}
	}
}

// kindMeta returns the ANSI color and short label for a witness kind.
func kindMeta(kind spore.WitnessKind) (string, string) {
	switch kind {
	case spore.WitnessKindIncoming:
		return colorCyan, "IN "
	case spore.WitnessKindOutgoing:
		return colorGreen, "OUT"
	case spore.WitnessKindExpanded:
		return colorBlue, "EXP"
	case spore.WitnessKindEvent:
		return colorRed, "EVT"
	case spore.WitnessKindNode:
		return colorMagenta, "NOD"
	default:
		return colorReset, "???"
	}
}

// formatBody reconstructs a wire-like string from a WitnessMessage.
func formatBody(msg *spore.WitnessMessage) string {
	// Hub events and node-emitted witness messages are structured with Subject + Args + Flags
	if msg.Kind == spore.WitnessKindEvent || msg.Kind == spore.WitnessKindNode {
		return formatDiagnosticBody(msg)
	}
	if msg.IsResponse {
		return formatResponseBody(msg)
	}
	return formatCallBody(msg)
}

// formatDiagnosticBody reconstructs event and node-emitted messages from Subject, Args, and Flags.
func formatDiagnosticBody(msg *spore.WitnessMessage) string {
	var parts []string
	parts = append(parts, msg.Subject)
	for _, k := range sortedKeys(msg.Args) {
		parts = append(parts, formatKV(k, msg.Args[k]))
	}
	parts = append(parts, sortedFlags(msg.Flags)...)
	return strings.Join(parts, " ")
}

// formatCallBody reconstructs a call-type witness body: subject [args] [flags] [~handle]
func formatCallBody(msg *spore.WitnessMessage) string {
	var parts []string
	parts = append(parts, msg.Subject)
	if msg.Cast != "" {
		parts = append(parts, "cast="+msg.Cast)
	}
	for _, k := range sortedKeys(msg.Args) {
		parts = append(parts, formatKV(k, msg.Args[k]))
	}
	parts = append(parts, sortedFlags(msg.Flags)...)
	if msg.Handle != "" {
		parts = append(parts, "~"+msg.Handle)
	}
	return strings.Join(parts, " ")
}

// formatResponseBody reconstructs a response-type witness body: ~handle:subject status [fields]
func formatResponseBody(msg *spore.WitnessMessage) string {
	var parts []string

	head := msg.Subject
	if msg.Handle != "" {
		head = "~" + msg.Handle + ":" + msg.Subject
	}
	parts = append(parts, head)

	switch {
	case msg.OK:
		parts = append(parts, "ok")
	case msg.Cancelled:
		parts = append(parts, "cancelled")
	case msg.CustomError:
		parts = append(parts, "custom_error")
	default:
		parts = append(parts, "error")
	}

	if msg.Capture != "" {
		parts = append(parts, "capture="+msg.Capture)
	}
	if msg.ErrCode != "" {
		parts = append(parts, "code="+msg.ErrCode)
	}
	if msg.ErrWhat != "" {
		parts = append(parts, formatKV("what", msg.ErrWhat))
	}
	for _, k := range sortedKeys(msg.Args) {
		parts = append(parts, formatKV(k, msg.Args[k]))
	}
	parts = append(parts, sortedFlags(msg.Flags)...)
	return strings.Join(parts, " ")
}

// formatKV formats a key=value pair, quoting the value if it contains spaces.
func formatKV(k, v string) string {
	if strings.ContainsAny(v, " \t") {
		return k + `="` + v + `"`
	}
	return k + "=" + v
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedFlags(m map[string]bool) []string {
	flags := make([]string, 0, len(m))
	for f := range m {
		flags = append(flags, f)
	}
	sort.Strings(flags)
	return flags
}
