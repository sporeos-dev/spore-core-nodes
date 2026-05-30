// Copyright 2026 mharr
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	spore "github.com/sporeos-dev/spore-client-libs/go"
)

const appId = "dev.sporeos.log"

const maxLogSize = 10 * 1024 * 1024 // 10 MB per file
const maxRotated = 5                 // keep spore.log.1 … spore.log.5

// rollingLogger writes lines to a file and rotates when it exceeds maxLogSize.
type rollingLogger struct {
	mu   sync.Mutex
	file *os.File
	path string
	size int64
}

func newRollingLogger(path string) (*rollingLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat log file: %w", err)
	}
	return &rollingLogger{file: f, path: path, size: info.Size()}, nil
}

func (l *rollingLogger) writeLine(line string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	n, err := fmt.Fprintln(l.file, line)
	if err != nil {
		return err
	}
	l.size += int64(n)

	if l.size >= maxLogSize {
		return l.rotate()
	}
	return nil
}

// rotate closes the current file, shifts existing numbered files up by one,
// renames the current log to .1, and opens a fresh log file.
func (l *rollingLogger) rotate() error {
	l.file.Close()

	for i := maxRotated - 1; i >= 1; i-- {
		old := fmt.Sprintf("%s.%d", l.path, i)
		newName := fmt.Sprintf("%s.%d", l.path, i+1)
		os.Rename(old, newName) // best-effort; ignore error if file absent
	}
	os.Rename(l.path, l.path+".1")

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open new log after rotation: %w", err)
	}
	l.file = f
	l.size = 0
	log.Printf("spore-log: rotated log, new file at %s", l.path)
	return nil
}

func (l *rollingLogger) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Close()
	}
}

// defaultLogPath returns the platform-specific path for the spore-log output file.
func defaultLogPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Logs/spore-os/spore.log"
	case "linux":
		return "/var/log/spore-os/spore.log"
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "spore.log")
		}
		return filepath.Join(home, ".spore", "logs", "spore.log")
	}
}

func main() {
	logPath := defaultLogPath()

	logger, err := newRollingLogger(logPath)
	if err != nil {
		log.Fatalf("spore-log: %v", err)
	}
	defer logger.close()

	client := spore.NewClient(appId)

	client.HandleWitness(func(msg *spore.WitnessMessage) {
		t := time.UnixMilli(msg.SporeTime).UTC().Format("2006-01-02T15:04:05.000Z")
		kind := kindLabel(msg.Kind)
		body := formatBody(msg)
		line := fmt.Sprintf("%s  %s  %s", t, kind, body)
		if err := logger.writeLine(line); err != nil {
			log.Println("spore-log: write error:", err)
		}
	})

	if err := client.Connect(); err != nil {
		log.Fatal("spore-log: connect:", err)
	}
	defer client.Close()

	log.Printf("spore-log: connected, writing to %s", logPath)

	if err := client.Listen(); err != nil {
		if !strings.Contains(err.Error(), "use of closed network connection") {
			log.Println("spore-log: disconnected:", err)
		}
	}
}

// kindLabel returns a fixed-width label for the witness kind.
func kindLabel(kind spore.WitnessKind) string {
	switch kind {
	case spore.WitnessKindIncoming:
		return "IN "
	case spore.WitnessKindOutgoing:
		return "OUT"
	case spore.WitnessKindExpanded:
		return "EXP"
	case spore.WitnessKindEvent:
		return "EVT"
	case spore.WitnessKindNode:
		return "NOD"
	default:
		return "???"
	}
}

// formatBody reconstructs a wire-like string from a WitnessMessage.
func formatBody(msg *spore.WitnessMessage) string {
	if msg.IsResponse {
		return formatResponseBody(msg)
	}
	return formatCallBody(msg)
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
