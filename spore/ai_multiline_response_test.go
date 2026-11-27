// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"reflect"
	"testing"

	"github.com/sporeos-dev/spore-client-libs/spore_go/response"
)

func TestQuotedMultilineResponseRendersBlankLines(t *testing.T) {
	raw := `~sfe59:alias.run ok output="\nHello, world!\n - Love, Python\n\n" capture=dev.sporeos.foundry.alias`
	resp, failure := response.Parse(raw)
	if resp == nil || failure != nil {
		t.Fatal("quoted multiline response did not parse")
	}

	complex, _, lines, warning := parseValue(parseRespArgs(resp)["output"], "    ")
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
	resp := response.New("alias.run", "sfe59").WithFlag("ok").WithArg("output", want).WithArg("empty", "").WithArg("capture", "alias")

	args := parseRespArgs(resp)
	if !reflect.DeepEqual(args, map[string]string{"output": want, "empty": ""}) {
		t.Fatalf("parseRespArgs() = %#v", args)
	}
}
