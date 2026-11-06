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

func TestPrintCancelledResponse(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	printResponse(response.New("file.open", "h").WithFlag("ok").WithFlag("cancelled"), nil)
	os.Stdout = old
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "[cancelled] ~h") || strings.Contains(string(out), "[ok]") {
		t.Fatalf("unexpected output: %q", out)
	}
}
