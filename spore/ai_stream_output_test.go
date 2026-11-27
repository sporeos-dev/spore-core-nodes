// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"os"
	"testing"
)

func TestPrintStreamChunkWritesImmediatelyWithoutSeparators(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = old
		r.Close()
		w.Close()
	})

	for _, chunk := range []string{"Hello", " world\n", "  \xc3", "\xa9"} {
		if err := printStreamChunk(chunk); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, len(chunk))
		if _, err := io.ReadFull(r, got); err != nil {
			t.Fatal(err)
		}
		if string(got) != chunk {
			t.Fatalf("output = %q, want %q", got, chunk)
		}
	}
}

func TestPrintStreamChunkReportsWriteFailure(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = old
		r.Close()
	})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := printStreamChunk("output"); err == nil {
		t.Fatal("expected a write error")
	}
}
