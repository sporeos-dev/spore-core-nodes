// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	spore "github.com/sporeos-dev/spore-client-libs/spore_go"
	"github.com/sporeos-dev/spore-client-libs/spore_go/witness"
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

	client := spore.New(appId).
		WithDefaultErrorHandler()

	client.OnWitness(func(w *witness.Witness) {
		sporeTimeStr := w.ArgIf("spore_time", "0")
		sporeTimeMs, _ := strconv.ParseInt(sporeTimeStr, 10, 64)
		t := time.UnixMilli(sporeTimeMs).UTC().Format("2006-01-02T15:04:05.000Z")
		kind := kindLabel(w)
		body := w.ArgIf("body", "")
		rawBody := w.Body()
		var line string
		if body != "" {
			line = fmt.Sprintf("%s  %s  %s  {%s}", t, kind, body, rawBody)
		} else {
			line = fmt.Sprintf("%s  %s  %s", t, kind, rawBody)
		}
		if err := logger.writeLine(line); err != nil {
			log.Println("spore-log: write error:", err)
		}
	})

	if err := client.Connect(); err != nil {
		log.Fatal("spore-log: connect:", err)
	}
	defer client.Disconnect()

	log.Printf("spore-log: connected, writing to %s", logPath)

	if err := client.Listen(); err != nil {
		if !strings.Contains(err.Error(), "use of closed network connection") {
			log.Println("spore-log: disconnected:", err)
		}
	}
}

// kindLabel returns a fixed-width label for a witness kind.
func kindLabel(w *witness.Witness) string {
	switch {
	case w.Flag("spore_incoming"):
		return "IN "
	case w.Flag("spore_outgoing"):
		return "OUT"
	case w.Flag("spore_expanded"):
		return "EXP"
	case w.Flag("spore_event"):
		return "EVT"
	case w.Flag("spore_node"):
		return "NOD"
	default:
		return "???"
	}
}


