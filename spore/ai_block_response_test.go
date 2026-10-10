// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"reflect"
	"testing"

	"github.com/sporeos-dev/spore-client-libs/spore_go/response"
)

func TestBlockResponseValueIsOneFieldAndPreservesContent(t *testing.T) {
	raw := "~sfe59:alias.run ok output=<<\n\nHello, world!\n - Love, Python\n\n\n>> capture=dev.sporeos.foundry.alias"

	fields := splitFields(raw)
	wantFields := []string{
		"~sfe59:alias.run",
		"ok",
		"output=<<\n\nHello, world!\n - Love, Python\n\n\n>>",
		"capture=dev.sporeos.foundry.alias",
	}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Fatalf("splitFields() = %#v, want %#v", fields, wantFields)
	}

	value := unwrapBlockValue(fields[2][len("output="):])
	if value != "\nHello, world!\n - Love, Python\n\n" {
		t.Fatalf("unwrapBlockValue() = %q", value)
	}

	complex, _, lines, warning := parseValue(value, "    ")
	if !complex || warning != "" {
		t.Fatalf("parseValue() complex=%t warning=%q, want true and empty", complex, warning)
	}
	wantLines := []string{"    ", "    Hello, world!", "     - Love, Python", "    ", "    "}
	if !reflect.DeepEqual(lines, wantLines) {
		t.Fatalf("parseValue() lines = %#v, want %#v", lines, wantLines)
	}
}

func TestParseResponseArgsPreservesClientBlockValue(t *testing.T) {
	want := "\nHello, world!\n - Love, Python\n\n"
	resp := response.New("alias.run", "sfe59").WithFlag("ok").WithArg("output", want)

	args := parseRespArgs(resp)
	if args["output"] != want {
		t.Fatalf("parseRespArgs() output = %q, want %q", args["output"], want)
	}
}
