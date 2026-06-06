// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"errors"
	"os/exec"
	"strings"
)

// isUserCancelled reports whether the error was caused by the user dismissing
// the dialog. The PowerShell command writes "canceled" to stderr when the user
// presses Cancel or closes the dialog.
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
	script := "Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.OpenFileDialog; $f.Title = 'Select File'; if ($f.ShowDialog() -eq 'OK') { [Console]::Out.Write($f.FileName) } else { [Console]::Error.Write('canceled'); exit 1 }"
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
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

	var filterParts []string
	for _, ext := range extensions {
		ext = strings.TrimPrefix(ext, ".")
		filterParts = append(filterParts, "*."+ext)
	}
	patternList := strings.Join(filterParts, ";")
	extList := strings.Join(filterParts, ", ")
	filterStr := "Supported Files (" + extList + ")|" + patternList + "|All Files (*.*)|*.*"

	script := "Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.OpenFileDialog; $f.Title = 'Select File'; $f.Filter = '" + filterStr + "'; if ($f.ShowDialog() -eq 'OK') { [Console]::Out.Write($f.FileName) } else { [Console]::Error.Write('canceled'); exit 1 }"

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	filePath := strings.TrimSpace(string(output))

	for _, ext := range extensions {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		if strings.HasSuffix(strings.ToLower(filePath), strings.ToLower(ext)) {
			return filePath, nil
		}
	}
	return "", errors.New("selected file does not match required extensions: " + strings.Join(extensions, ", "))
}

// openDirectory opens a directory picker dialog and returns the selected path.
func openDirectory() (string, error) {
	script := "Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.FolderBrowserDialog; $f.Description = 'Select Folder'; if ($f.ShowDialog() -eq 'OK') { [Console]::Out.Write($f.SelectedPath) } else { [Console]::Error.Write('canceled'); exit 1 }"
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// saveFile opens a save dialog and returns the chosen file path.
func saveFile() (string, error) {
	script := "Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.SaveFileDialog; $f.Title = 'Save As'; if ($f.ShowDialog() -eq 'OK') { [Console]::Out.Write($f.FileName) } else { [Console]::Error.Write('canceled'); exit 1 }"
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
