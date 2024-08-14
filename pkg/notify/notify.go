package notify

import (
	"github.com/brettcodling/Kubessh/pkg/directory"
	"github.com/gen2brain/beeep"
)

// Warning creates a warning notification.
func Warning(title, context string) {
	beeep.Notify(title, context, directory.Dir+"/assets/warning.png")
}
