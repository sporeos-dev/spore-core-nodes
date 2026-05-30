// Copyright 2026 mharr
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin

package main

import "errors"

// isUserCancelled always returns false on non-macOS platforms since dialog
// commands are not implemented.
func isUserCancelled(err error) bool {
	return false
}

func openFile() (string, error) {
	return "", errors.ErrUnsupported
}

func openFileWithExtensions(extensions []string) (string, error) {
	return "", errors.ErrUnsupported
}

func openDirectory() (string, error) {
	return "", errors.ErrUnsupported
}

func saveFile() (string, error) {
	return "", errors.ErrUnsupported
}
