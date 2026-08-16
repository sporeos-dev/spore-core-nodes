// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	spore "github.com/sporeos-dev/spore-client-libs/spore_go"
	"github.com/sporeos-dev/spore-client-libs/spore_go/witness"
)

const appId = "dev.sporeos.witness"

// ANSI color codes for terminal output.
const (
	colorReset     = "\033[0m"
	colorGray      = "\033[90m"
	colorGreen     = "\033[92m" // bright green
	colorMagenta   = "\033[35m" // magenta
	colorBlue      = "\033[34m"
	colorCyan      = "\033[36m"
	colorRed       = "\033[91m" // bright red
)

func main() {
	client := spore.New(appId).
		WithDefaultErrorHandler()

	client.OnWitness(func(w *witness.Witness) {
		sporeTimeStr := w.ArgIf("spore_time", "0")
		sporeTimeMs, _ := strconv.ParseInt(sporeTimeStr, 10, 64)
		t := time.UnixMilli(sporeTimeMs).Local().Format("15:04:05.000")
		color, label := kindMeta(w)
		if w.Flag("spore_node") && w.ArgIf("cast", "") != "" {
			label = fmt.Sprintf("NOD(%s)", w.ArgIf("cast", ""))
		}
		body := w.ArgIf("body", w.Body())
		fmt.Printf("%s%s  %s%s  %s\n", color, t, label, colorReset, body)
	})

	if err := client.Connect(); err != nil {
		log.Fatal("connect:", err)
	}
	defer client.Disconnect()

	fmt.Println("spore-witness: connected, watching hub traffic...")

	for {
		if err := client.Listen(); err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				break
			}
			fmt.Println("spore-witness: disconnected, reconnecting...")
		}
		client.Disconnect()

		for {
			time.Sleep(5 * time.Second)
			if err := client.Connect(); err != nil {
				fmt.Println("spore-witness: reconnect failed, retrying in 5s...")
				continue
			}
			fmt.Println("spore-witness: reconnected, watching hub traffic...")
			break
		}
	}
}

// kindMeta returns the ANSI color and short label for a witness kind.
func kindMeta(w *witness.Witness) (string, string) {
	switch {
	case w.Flag("spore_incoming"):
		return colorCyan, "IN "
	case w.Flag("spore_outgoing"):
		return colorGreen, "OUT"
	case w.Flag("spore_expanded"):
		return colorBlue, "EXP"
	case w.Flag("spore_event"):
		return colorRed, "EVT"
	case w.Flag("spore_node"):
		return colorMagenta, "NOD"
	default:
		return colorReset, "???"
	}
}
