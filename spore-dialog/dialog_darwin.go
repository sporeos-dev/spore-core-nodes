// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package main

import (
	"errors"
	"os/exec"
	"strings"
)

// isUserCancelled reports whether the error was caused by the user dismissing
// the dialog. osascript exits with code 1 and writes "User canceled." to stderr
// when the user presses Cancel or closes the dialog.
func isUserCancelled(err error) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		stderr := strings.ToLower(string(exitErr.Stderr))
		return strings.Contains(stderr, "canceled") || strings.Contains(stderr, "cancelled")
	}
	return false
}

// openFile opens a file picker dialog and returns the selected path.
func openFile() (string, error) {
	cmd := exec.Command("osascript", "-e", `POSIX path of (choose file)`)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// openFileWithExtensions opens a file picker dialog and returns the selected path,
// validating that the chosen file ends with one of the given extensions.
// The prompt lists the accepted extensions so the user knows what to pick.
func openFileWithExtensions(extensions []string) (string, error) {
	if len(extensions) == 0 {
		return openFile()
	}

	prompt := "Select a file (" + strings.Join(extensions, ", ") + ")"
	script := `POSIX path of (choose file with prompt "` + prompt + `")`

	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	filePath := strings.TrimSpace(string(output))

	for _, ext := range extensions {
		if strings.HasSuffix(filePath, ext) {
			return filePath, nil
		}
	}
	return "", errors.New("selected file does not match required extensions: " + strings.Join(extensions, ", "))
}

// openDirectory opens a directory picker dialog and returns the selected path.
func openDirectory() (string, error) {
	cmd := exec.Command("osascript", "-e", `POSIX path of (choose folder)`)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// saveFile opens a save dialog and returns the chosen file path.
func saveFile() (string, error) {
	cmd := exec.Command("osascript", "-e", `POSIX path of (choose file name)`)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
