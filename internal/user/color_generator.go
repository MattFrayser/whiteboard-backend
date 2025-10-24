package user

import (
	"sync"

	"github.com/lucasb-eyer/go-colorful"
)

// ColorGenerator: generates distributed colors for user cursors
type ColorGenerator struct {
	counter int
	mu      sync.Mutex
}

func NewColorGenerator() *ColorGenerator {
	return &ColorGenerator{
		counter: 0,
	}
}

// NextColor: returns the next color in the golden ratio distribution sequence
func (cg *ColorGenerator) NextColor() string {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	const goldenRatio = 0.618033988749895
	hue := float64(cg.counter) * goldenRatio
	hue = hue - float64(int(hue)) // Keep fractional part
	cg.counter++

	color := colorful.Hsl(hue*360, 0.85, 0.55)
	return color.Hex()
}
