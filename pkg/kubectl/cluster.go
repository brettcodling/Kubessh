package kubectl

import (
	"os/exec"

	"github.com/brettcodling/Kubessh/pkg/notify"
)

func CheckConnection() bool {
	_, err := exec.Command("bash", "-c", "kubectl cluster-info --request-timeout='5s'").Output()
	if err != nil {
		notify.Warning("ERROR!", err.Error())
		return false
	}

	return true
}
