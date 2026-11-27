// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"reflect"
	"testing"

	"github.com/sporeos-dev/spore-client-libs/spore_go/publish"
	"github.com/sporeos-dev/spore-client-libs/spore_go/response"
)

func TestQuotedMultilineResponseRendersBlankLines(t *testing.T) {
	raw := `~shell-74:alias.run ok output="\nHello, world!\n - Love, Python\n\n" capture=dev.sporeos.foundry.alias`
	resp, failure := response.Parse(raw)
	if resp == nil || failure != nil {
		t.Fatal("quoted multiline response did not parse")
	}

	args, _ := parseRespArgs(resp)
	complex, _, lines, warning := parseValue(args["output"], "    ")
	if !complex || warning != "" {
		t.Fatalf("parseValue() complex=%t warning=%q, want true and empty", complex, warning)
	}
	wantLines := []string{"    ", "    Hello, world!", "     - Love, Python", "    ", "    "}
	if !reflect.DeepEqual(lines, wantLines) {
		t.Fatalf("parseValue() lines = %#v, want %#v", lines, wantLines)
	}
}

func TestParseResponseArgsPreservesClientMultilineValue(t *testing.T) {
	want := "\n\"hello\" << >> \\n\r\n"
	resp := response.New("alias.run", "shell-74").WithFlag("ok").WithFlag("ready").
		WithArg("output", want).WithArg("empty", "").WithArg("capture", "alias")

	args, flags := parseRespArgs(resp)
	if !reflect.DeepEqual(args, map[string]string{"output": want, "empty": ""}) || !reflect.DeepEqual(flags, []string{"ready"}) {
		t.Fatalf("parseRespArgs() = %#v, %#v", args, flags)
	}
}

func TestParsePublishDataPreservesMultilineValue(t *testing.T) {
	want := "first\nsecond\n"
	p := publish.New("example.events").WithArg("body", want).WithArg("cast", "example").WithFlag("ready")
	args, flags := parsePublishData(p)
	if !reflect.DeepEqual(args, map[string]string{"body": want}) || !reflect.DeepEqual(flags, []string{"ready"}) {
		t.Fatalf("parsePublishData() = %#v, %#v", args, flags)
	}
}
