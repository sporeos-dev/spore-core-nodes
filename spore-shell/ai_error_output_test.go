// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/sporeos-dev/spore-client-libs/spore_go/response"
)

func TestPrintResponseErrorDetails(t *testing.T) {
	err := response.Error("spawn", "shell-02", "Missing.Spore", "missing argument").
		WithArg("argument", "node").
		WithArg("capture", "dev.sporeos.hub").
		WithArg("cast", "dev.sporeos.shell").
		WithArg("empty", "").
		WithArg("extra", "[]").
		WithArg("module", "Spore").
		WithArg("text", "first\nsecond").
		WithFlag("error").
		WithFlag("node_error").
		WithFlag("retryable")

	out := captureErrorOutput(t, err)
	want := "\r\033[K\r\n" + strings.Join([]string{
		"  [error] ~shell-02",
		"  spawn // dev.sporeos.hub",
		"  ----------",
		"  code: Missing.Spore",
		"  what: missing argument",
		"  ----------",
		"  argument: node",
		"  empty:",
		"  extra: []",
		"  module: Spore",
		"  text:",
		"    first",
		"    second",
		"  origin: node_error",
		"  retryable",
		"",
		"",
	}, "\r\n")
	if out != want {
		t.Fatalf("printResponse() = %q, want %q", out, want)
	}
}

func TestPrintResponseMissingArgument(t *testing.T) {
	err := response.Error("spawn", "shell-02", "Missing.Spore", "missing argument").
		WithArg("argument", "node").
		WithArg("cast", "dev.sporeos.shell").
		WithFlag("error")
	want := "\r\033[K\r\n  [error] ~shell-02\r\n  spawn // \r\n  ----------\r\n" +
		"  code: Missing.Spore\r\n  what: missing argument\r\n  ----------\r\n" +
		"  argument: node\r\n\r\n"
	if got := captureErrorOutput(t, err); got != want {
		t.Fatalf("printResponse() = %q, want %q", got, want)
	}
}

func TestPrintResponseErrorDetailDivider(t *testing.T) {
	tests := []struct {
		name   string
		flag   string
		detail string
	}{
		{name: "no visible details"},
		{name: "additional flag", flag: "retryable", detail: "retryable"},
		{name: "origin flag", flag: "node_error", detail: "origin: node_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := response.Error("spawn", "h", "Missing.Spore", "missing argument").
				WithArg("cast", "dev.sporeos.shell").
				WithFlag("error")
			if tt.flag != "" {
				err.WithFlag(tt.flag)
			}
			want := "\r\033[K\r\n  [error] ~h\r\n  spawn // \r\n  ----------\r\n" +
				"  code: Missing.Spore\r\n  what: missing argument\r\n"
			if tt.detail != "" {
				want += "  ----------\r\n  " + tt.detail + "\r\n"
			}
			want += "\r\n"
			if got := captureErrorOutput(t, err); got != want {
				t.Fatalf("printResponse() = %q, want %q", got, want)
			}
		})
	}
}

func captureErrorOutput(t *testing.T, err *response.ResponseError) string {
	t.Helper()
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	old := os.Stdout
	os.Stdout = w
	printResponse(nil, err)
	os.Stdout = old
	if closeErr := w.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	out, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if closeErr := r.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	return string(out)
}
