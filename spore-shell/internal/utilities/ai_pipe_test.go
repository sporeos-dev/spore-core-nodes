package utilities

import "testing"

func TestAppendHandleToFirstPipeStage(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"pipe", "file.open | install", "file.open ~shell-01 | install"},
		{"unspaced pipe", "file.open|install", "file.open ~shell-01 |install"},
		{"quoted pipe", `file.open text="a | b" | install`, `file.open text="a | b" ~shell-01 | install`},
		{"escaped quote", `file.open text="a \" | b" | install`, `file.open text="a \" | b" ~shell-01 | install`},
		{"single quoted pipe", `file.open text='a | b' | install`, `file.open text='a | b' ~shell-01 | install`},
		{"nested pipe", `file.open data=[{text="a | b"}] | install`, `file.open data=[{text="a | b"}] ~shell-01 | install`},
		{"multiline value with pipe", `file.open data="\na | b\n>>" | install`, `file.open data="\na | b\n>>" ~shell-01 | install`},
		{"literal pipe only", `file.open text="a | b"`, `file.open text="a | b" ~shell-01`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setCount(0)
			if got := AppendHandle(tt.input); got != tt.want {
				t.Fatalf("AppendHandle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHasHandleIgnoresValues(t *testing.T) {
	for _, input := range []string{
		`file.open path=~/notes | install`,
		`file.open text="~not-a-handle" | install`,
		`file.open data=[~not-a-handle] | install`,
		`file.open data="\n~not-a-handle\n" | install`,
		"file.open ~\u2003 | install",
	} {
		if HasHandle(input) {
			t.Errorf("HasHandle(%q) = true", input)
		}
	}
	if !HasHandle("file.open ~h | install") {
		t.Error("did not find the first-stage handle")
	}
}
