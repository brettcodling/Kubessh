package main

import (
	"log"
	"log/syslog"
	"os"
	"strconv"
	"time"

	"github.com/brettcodling/Kubessh/pkg/database"
	"github.com/brettcodling/Kubessh/pkg/directory"
	"github.com/brettcodling/Kubessh/pkg/kubectl"
	"github.com/brettcodling/systray"
)

func init() {
	if os.Getenv("DISABLE_SYSLOG") != "1" {
		syslog, err := syslog.New(syslog.LOG_INFO, "Kubessh")
		if err != nil {
			log.Fatal("Unable to connect to syslog")
		}
		log.SetOutput(syslog)
	}
}

func main() {
	defer database.DB.Close()

	delay := os.Getenv(("DELAY_STARTUP"))
	if delay != "" {
		delaySeconds, err := strconv.Atoi(delay)
		if err == nil && delaySeconds > 0 {
			log.Printf("Delaying startup for %d seconds\n", delaySeconds)
			time.Sleep(time.Second * time.Duration(delaySeconds))
		}
	}

	connected := kubectl.CheckConnection()

	systray.Run(func() {
		systray.SetIcon(getIcon())
		kubectl.AddContexts()
		kubectl.AddNamespaces()
		kubectl.AddPods()
		kubectl.AddPortForwarding()
		systray.AddSeparator()
		settings := systray.AddMenuItem("Settings", "")
		refreshItem := systray.AddMenuItem("Refresh", "")
		quit := systray.AddMenuItem("Quit", "")
		go func() {
			for {
				select {
				case <-settings.ClickedCh:
					kubectl.OpenSettings()
				case <-refreshItem.ClickedCh:
					refresh(false)
				case <-quit.ClickedCh:
					systray.Quit()
				}
			}
		}()
		if connected {
			refresh(true)
		}
	}, func() {})
}

func getIcon() []byte {
	image, err := os.ReadFile(directory.Dir + "/assets/logo.png")
	if err != nil {
		log.Println(err)

		return []byte{}
	}

	return image
}

func refresh(bypass bool) {
	if bypass || kubectl.CheckConnection() {
		kubectl.SetContexts()
		kubectl.SetNamespaces()
		kubectl.SetPods()
	}
}
