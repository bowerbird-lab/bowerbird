// Command mobile-nsgo is the Bowerbird mobile client rewritten with NSGo.
// It connects to the desktop's existing /api/v1 HTTP and WebSocket API.
package main

import (
	"github.com/nsgo-dev/nsgo"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/ui"
)

func main() {
	nsgo.Run(ui.Root, nsgo.WindowSize(420, 800))
}
