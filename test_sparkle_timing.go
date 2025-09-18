// Test file to verify the sparkle timing works correctly
package main

import (
	"fmt"
	"time"

	"github.com/elastic/elastic-package/internal/llmagent"
)

func main() {
	fmt.Println("Testing sparkle timing (100ms duration)...")
	
	// Create animation
	animation := llmagent.NewAnimatedStatus("Testing sparkle...")
	animation.Start()
	
	// Let it run for a bit without sparkle
	fmt.Println("Running animation without sparkle for 1 second...")
	time.Sleep(1 * time.Second)
	
	// Trigger sparkle (should show for 100ms)
	fmt.Println("Triggering sparkle (should show for 100ms)...")
	animation.Sparkle()
	
	// Wait 150ms to see sparkle appear and disappear
	time.Sleep(150 * time.Millisecond)
	
	// Trigger another sparkle
	fmt.Println("Triggering another sparkle...")
	animation.Sparkle()
	
	// Wait to see it again
	time.Sleep(150 * time.Millisecond)
	
	// Stop animation
	animation.Finish("Sparkle timing test completed")
	
	fmt.Println("✅ Sparkle timing test completed!")
}
