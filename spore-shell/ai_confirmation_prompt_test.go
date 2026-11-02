// Copyright 2026 Matt Harrison
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestConfirmationPromptDuringInputTransitions(t *testing.T) {
	reset := func() {
		pendingConfirmMu.Lock()
		outputMutex.Lock()
		pendingConfirm = nil
		inputConfirm = nil
		inputBuf = nil
		inputCursor = 0
		currentPrompt = ""
		waitingForInput = false
		outputMutex.Unlock()
		pendingConfirmMu.Unlock()
	}
	t.Cleanup(reset)

	t.Run("confirmation before input starts", func(t *testing.T) {
		reset()
		pc := &pendingConfirmState{}
		startConfirmation(pc)
		startInput("[connected]>: ")
		if currentPrompt != confirmRespondPrompt || inputConfirm != pc {
			t.Fatalf("prompt=%q confirmation=%p, want confirmation prompt and %p", currentPrompt, inputConfirm, pc)
		}
	})

	t.Run("confirmation during input", func(t *testing.T) {
		reset()
		startInput("[connected]>: ")
		inputBuf = []rune("unfinished command")
		pc := &pendingConfirmState{}
		startConfirmation(pc)
		if currentPrompt != confirmRespondPrompt || inputConfirm != pc || len(inputBuf) != 0 {
			t.Fatal("confirmation did not replace the active command prompt and input")
		}
		inputBuf = []rune("y")
		outputMutex.Lock()
		line, confirm := finishInputLocked()
		outputMutex.Unlock()
		if line != "y" || confirm != pc {
			t.Fatalf("finished input = %q, %p; want y, %p", line, confirm, pc)
		}
	})

	t.Run("confirmation after command submitted", func(t *testing.T) {
		reset()
		startInput("[connected]>: ")
		inputBuf = []rune("store.get")
		outputMutex.Lock()
		line, confirm := finishInputLocked()
		outputMutex.Unlock()
		pc := &pendingConfirmState{}
		startConfirmation(pc)
		if line != "store.get" || confirm != nil {
			t.Fatal("a later confirmation was associated with an already submitted command")
		}
		startInput("[connected]>: ")
		if currentPrompt != confirmRespondPrompt || inputConfirm != pc {
			t.Fatal("the pending confirmation was not shown on the next prompt")
		}
	})
}
