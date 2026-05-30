// Copyright 2026 mharr
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	spore "github.com/sporeos-dev/spore-client-libs/go"
)

const appId = "dev.sporeos.dialog"

func main() {
	fmt.Println("Starting dialog node")

	client := spore.NewClient(appId)

	client.HandleRequest("file.open", func(call *spore.Call) {
		var path string
		var err error

		if call.HasArg("ext") {
			extensions := parseExtensions(call.Arg("ext"))
			path, err = openFileWithExtensions(extensions)
		} else {
			path, err = openFile()
		}

		if err != nil {
			respondWithError(call, err)
			return
		}
		if replyErr := call.Reply(map[string]string{"path": path}); replyErr != nil {
			log.Println("reply error:", replyErr)
		}
	})

	client.HandleRequest("dir.open", func(call *spore.Call) {
		path, err := openDirectory()
		if err != nil {
			respondWithError(call, err)
			return
		}
		if replyErr := call.Reply(map[string]string{"path": path}); replyErr != nil {
			log.Println("reply error:", replyErr)
		}
	})

	client.HandleRequest("file.save", func(call *spore.Call) {
		path, err := saveFile()
		if err != nil {
			respondWithError(call, err)
			return
		}
		if replyErr := call.Reply(map[string]string{"path": path}); replyErr != nil {
			log.Println("reply error:", replyErr)
		}
	})

	if err := client.Connect(); err != nil {
		log.Fatal("connect:", err)
	}
	defer client.Close()

	fmt.Println("Connected as", appId)

	if err := client.Listen(); err != nil {
		log.Println("disconnected:", err)
	}
}

// respondWithError sends the appropriate error response for a dialog operation.
// Cancelled dialogs (user dismissed) are sent as Cancel, unsupported platforms
// as RouteNotImplemented, and all other failures as Runtime errors.
func respondWithError(call *spore.Call, err error) {
	var replyErr error
	switch {
	case isUserCancelled(err):
		replyErr = call.Cancel()
	case errors.Is(err, errors.ErrUnsupported):
		replyErr = call.Error(spore.ErrorCodeRouteNotImplemented, "dialog commands are not supported on this platform")
	default:
		replyErr = call.Error(spore.ErrorCodeRuntime, err.Error())
	}
	if replyErr != nil {
		log.Println("reply error:", replyErr)
	}
}

// parseExtensions parses the ext argument value into a slice of extension strings.
// Accepts both array syntax (e.g. "[ '.txt' '.go' ]") and a bare extension (".txt").
func parseExtensions(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Fields(raw)
	exts := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(p, "'\"")
		if p != "" {
			exts = append(exts, p)
		}
	}
	return exts
}

