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

func TestStreamChunksKeepPromptAndFinalResponse(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	waitingForInput = true
	currentPrompt = "[p]>: "
	inputBuf = []rune("typing")
	inputCursor = 3
	streamAtPrompt = false
	t.Cleanup(func() {
		os.Stdout = old
		r.Close()
		w.Close()
		waitingForInput = false
		streamAtPrompt = false
		currentPrompt = ""
		inputBuf = nil
		inputCursor = 0
	})

	check := func(want string) {
		t.Helper()
		got := make([]byte, len(want))
		if _, err := io.ReadFull(r, got); err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
	prompt := "[p]>: typing\033[3D"
	printStreamChunk("h", "output", "Hel")
	check("\r\033[K  [stream] ~h output\r\nHel\0337\r\n" + prompt)
	printStreamChunk("h", "output", "lo\n")
	check("\r\033[K\0338lo\r\n\0337" + prompt)
	printStreamChunk("other", "output", "next")
	check("\r\033[K  [stream] ~other output\r\nnext\0337\r\n" + prompt)
	printAbovePrompt("notice")
	check("\r\033[Knotice\r\n" + prompt)
	printStreamChunk("h", "output", "again")
	check("\r\033[K  [stream] ~h output\r\nagain\0337\r\n" + prompt)

	resp := response.New("alias.run", "h").WithFlag("ok").WithArg("output", "Hello\nagain")
	printResponse(resp, nil)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	remaining, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(remaining), "  [ok] ~h") ||
		!strings.Contains(string(remaining), "    Hello\r\n    again") {
		t.Fatalf("final response missing completed output: %q", remaining)
	}
}

func TestStreamChunksPreserveSplitBytesAndCRLF(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	waitingForInput = true
	currentPrompt = "> "
	inputBuf = nil
	inputCursor = 0
	streamAtPrompt = false
	t.Cleanup(func() {
		os.Stdout = old
		r.Close()
		w.Close()
		waitingForInput = false
		streamAtPrompt = false
		currentPrompt = ""
	})

	printStreamChunk("h", "output", "\xc3")
	printStreamChunk("h", "output", "\xa9\r")
	printStreamChunk("h", "output", "\n")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	want := "\r\033[K  [stream] ~h output\r\n\xc3\0337\r\n> " +
		"\r\033[K\0338\xa9\r\0337\r\n> " +
		"\r\033[K\0338\n\0337> "
	if string(got) != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
