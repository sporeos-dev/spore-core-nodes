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
	want := strings.Join([]string{
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
	}, "\n")
	if out != want {
		t.Fatalf("printResponse() = %q, want %q", out, want)
	}
}

func TestPrintResponseMissingArgument(t *testing.T) {
	err := response.Error("spawn", "shell-02", "Missing.Spore", "missing argument").
		WithArg("argument", "node").
		WithArg("cast", "dev.sporeos.shell").
		WithFlag("error")
	want := "  [error] ~shell-02\n  spawn // \n  ----------\n" +
		"  code: Missing.Spore\n  what: missing argument\n  ----------\n  argument: node\n"
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
			want := "  [error] ~h\n  spawn // \n  ----------\n" +
				"  code: Missing.Spore\n  what: missing argument\n"
			if tt.detail != "" {
				want += "  ----------\n  " + tt.detail + "\n"
			}
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
	old := os.Stderr
	os.Stderr = w
	printResponse(nil, err)
	os.Stderr = old
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
