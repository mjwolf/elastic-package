// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package llmagent

import (
	"fmt"
	"sync"
	"time"
)

// AnimatedStatus provides animated status display
type AnimatedStatus struct {
	message     string
	active      bool
	mutex       sync.Mutex
	stopCh      chan bool
	frames      []string
	frameIndex  int
	sparkleEnd  time.Time  // When the sparkle effect should end
	showSparkle bool       // Whether to show sparkle
}

// NewAnimatedStatus creates a new animated status display
func NewAnimatedStatus(message string) *AnimatedStatus {
	frames := []string{
		"[▓░░░]", // Loading bar style
		"[▓▓░░]",
		"[▓▓▓░]",
		"[▓▓▓▓]",
		"[░▓▓▓]", // Scrolling effect
		"[░░▓▓]",
		"[░░░▓]",
		"[░░░░]",
	}

	return &AnimatedStatus{
		message: message,
		frames:  frames,
		stopCh:  make(chan bool),
	}
}

// Start begins the animation
func (a *AnimatedStatus) Start() {
	a.mutex.Lock()
	if a.active {
		a.mutex.Unlock()
		return
	}
	a.active = true
	a.mutex.Unlock()

	// Hide cursor
	fmt.Print("\033[?25l")

	go a.animate()
}

// Stop ends the animation and clears the line
func (a *AnimatedStatus) Stop() {
	a.mutex.Lock()
	if !a.active {
		a.mutex.Unlock()
		return
	}
	a.active = false
	a.mutex.Unlock()

	a.stopCh <- true

	// Clear the line and show cursor
	fmt.Print("\r\033[K")
	fmt.Print("\033[?25h")
}

// Update changes the message and adds activity indication
func (a *AnimatedStatus) Update(newMessage string) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.active {
		a.message = newMessage
		// Add a brief "flash" effect by changing frame
		a.frameIndex = (a.frameIndex + 3) % len(a.frames)
	}
}

// animate runs the animation loop
func (a *AnimatedStatus) animate() {
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.mutex.Lock()
			if !a.active {
				a.mutex.Unlock()
				return
			}

			// Print the current frame
			frame := a.frames[a.frameIndex]
			fmt.Printf("\r🤖 %s %s", a.message, frame)

			// Show sparkle if it's active and not expired
			now := time.Now()
			if a.showSparkle && now.Before(a.sparkleEnd) {
				fmt.Print(" ✨")
			} else {
				if a.showSparkle && !now.Before(a.sparkleEnd) {
					a.showSparkle = false // Turn off expired sparkle
				}
				fmt.Print("   ") // Clear the sparkle space
			}

			a.frameIndex = (a.frameIndex + 1) % len(a.frames)
			a.mutex.Unlock()
		}
	}
}

// Flash creates a brief visual indication of activity
func (a *AnimatedStatus) Flash() {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.active {
		// Jump to the "full" frame for visual feedback
		a.frameIndex = 3
	}
}

// Sparkle shows the ✨ effect for 100ms to indicate LLM response
func (a *AnimatedStatus) Sparkle() {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.active {
		a.showSparkle = true
		a.sparkleEnd = time.Now().Add(100 * time.Millisecond)
	}
}

// IsActive returns whether the animation is currently running
func (a *AnimatedStatus) IsActive() bool {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.active
}

// Finish stops the animation and shows a completion message
func (a *AnimatedStatus) Finish(message string) {
	a.Stop()
	fmt.Printf("🤖 %s [▓▓▓▓] ✅\n", message)
}

// Error stops the animation and shows an error message
func (a *AnimatedStatus) Error(message string) {
	a.Stop()
	fmt.Printf("🤖 %s [✗✗✗✗] ❌\n", message)
}
