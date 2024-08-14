package kubectl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/aarzilli/nucular"
	"github.com/aarzilli/nucular/style"
	"github.com/brettcodling/Kubessh/pkg/database"
	"github.com/brettcodling/Kubessh/pkg/notify"
	"github.com/brettcodling/systray"
)

type Pod struct {
	Name       string
	Ready      string
	Status     string
	Restarts   string
	Age        string
	Containers []Container
	CreatedAt  time.Time
}

type Container struct {
	Name  string
	Image string
	Ready string
}

var (
	podMenuItems map[string]MenuItem
	podsMenuItem *systray.MenuItem

	currentPod          Pod
	currentOpenPod      nucular.MasterWindow
	podUpdateCh         chan string
	portForwardCancel   map[string]context.CancelFunc
	portForwarding      map[string]MenuItem
	portForwardMenuItem *systray.MenuItem
	portForwardOpen     bool

	portFrom, portTo             nucular.TextEditor
	portFromString, portToString string

	selectedContainer int
)

func init() {
	podMenuItems = make(map[string]MenuItem)

	portForwarding = make(map[string]MenuItem)
	portForwardCancel = make(map[string]context.CancelFunc)
	portFrom.Flags = nucular.EditField
	portFrom.SingleLine = true
	portTo.Flags = nucular.EditField
	portTo.SingleLine = true
}

func AddPods() {
	podsMenuItem = systray.AddMenuItem("Pods:", "")
	podsMenuItem.Hide()
}

func AddPortForwarding() {
	portForwardMenuItem = systray.AddMenuItem("Port Forwarding:", "")
	portForwardMenuItem.Hide()
}

func GetPods() []Pod {
	var pods []Pod
	cmd := "kubectl get pods --no-headers"
	rawPods, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		log.Println(err)
		notify.Warning("ERROR!", err.Error())

		return pods
	}
	scanner := bufio.NewScanner(strings.NewReader(string(rawPods)))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		columns := strings.Fields(scanner.Text())
		pods = append(pods, Pod{
			Name:     columns[0],
			Ready:    columns[1],
			Status:   columns[2],
			Restarts: columns[3],
			Age:      columns[4],
		})
	}

	return pods
}

func SetPods() {
	for _, podMenuItem := range podMenuItems {
		podMenuItem.Item.Remove()
	}
	podMenuItems = make(map[string]MenuItem)
	for _, p := range GetPods() {
		addPod(p)
	}
	podsMenuItem.Show()
	watchPods()
}

func addPod(p Pod) {
	pod := podsMenuItem.AddSubMenuItem(p.getName(), "")
	podMenuItems[p.Name] = MenuItem{
		Item: pod,
	}
	go func(p Pod) {
		for {
			select {
			case <-pod.ClickedCh:
				go func(p Pod) {
					err := p.getContainers()
					if err != nil {
						log.Println(err)
						notify.Warning("ERROR!", err.Error())
					}
					currentPod = p
					openPod()
				}(p)
			}
		}
	}(p)
}

func openPod() {
	if currentOpenPod != nil {
		currentOpenPod.Close()
	}
	portFromString = database.Get("PORT-FROM-" + currentPod.Name)
	portFrom.SelectAll()
	portFrom.Text([]rune(portFromString))
	portToString = database.Get("PORT-TO-" + currentPod.Name)
	portTo.SelectAll()
	portTo.Text([]rune(portToString))
	selectedContainer = 0
	currentOpenPod = nucular.NewMasterWindow(0, "Pod: "+currentPod.Name, updatePod)
	currentOpenPod.SetStyle(style.FromTheme(style.DarkTheme, 2.0))
	currentOpenPod.Main()
}

func updatePod(w *nucular.Window) {
	w.Row(40).Dynamic(1)
	w.Label("Details", "LC")
	w.Row(30).Dynamic(2)
	w.Label("Ready:", "LC")
	w.Label(currentPod.Ready, "LC")
	w.Label("Restarts:", "LC")
	w.Label(currentPod.Restarts, "LC")
	w.Row(30).Dynamic(2)
	w.Label("Age:", "LC")
	w.Label(currentPod.getAge(), "LC")
	if len(currentPod.Containers) > 0 {
		w.Row(10).Dynamic(1)
		w.Row(30).Dynamic(2)
		w.Label("Containers:", "LC")
		var containers []string
		for _, container := range currentPod.Containers {
			containers = append(containers, container.Name)
		}
		selectedContainer = w.ComboSimple(containers, selectedContainer, 20)
		w.Row(30).Dynamic(2)
		w.Label("Image:", "LC")
		w.Label(currentPod.Containers[selectedContainer].Image, "LC")
		w.Row(30).Dynamic(2)
		w.Label("Ready:", "LC")
		w.Label(currentPod.Containers[selectedContainer].Ready, "LC")
		if w.ButtonText("SSH") {
			go func() {
				err := currentPod.ssh(currentPod.Containers[selectedContainer].Name)
				if err != nil {
					log.Println(err)
					notify.Warning("ERROR!", err.Error())
				}
			}()
		}
		if w.ButtonText("Logs") {
			go func() {
				err := currentPod.logs(currentPod.Containers[selectedContainer].Name)
				if err != nil {
					log.Println(err)
					notify.Warning("ERROR!", err.Error())
				}
			}()
		}
	}
	w.Row(40).Dynamic(1)
	portForwardOpen = w.TreePush(nucular.TreeNode, "Port Forwarding", false)
	if portForwardOpen {
		updatePortForward(w)
		w.TreePop()
	}
}

func updatePortForward(w *nucular.Window) {
	if portFromString != string(portFrom.Buffer) {
		portFromString = string(portFrom.Buffer)
		database.Set("PORT-FROM-"+currentPod.Name, portFromString)
		cancelPortForwarding(currentPod.Name)
	}
	if portToString != string(portTo.Buffer) {
		portToString = string(portTo.Buffer)
		database.Set("PORT-TO-"+currentPod.Name, portToString)
		cancelPortForwarding(currentPod.Name)
	}
	w.Row(30).Dynamic(2)
	w.Label("From:", "LC")
	portFrom.Edit(w)
	w.Label("To:", "LC")
	portTo.Edit(w)
	w.Row(30).Dynamic(2)
	w.Spacing(1)
	if _, ok := portForwarding[currentPod.Name]; ok {
		if w.ButtonText("Stop") {
			cancelPortForwarding(currentPod.Name)
		}
	} else {
		if w.ButtonText("Start") {
			var ctx context.Context
			ctx, portForwardCancel[currentPod.Name] = context.WithCancel(context.Background())
			go func() {
				currentPod.portForward(ctx)
			}()
			portForwardMenuItem.Show()
			menuItem := portForwardMenuItem.AddSubMenuItem(currentPod.Name, "")
			go func(p Pod) {
				for {
					select {
					case <-menuItem.ClickedCh:
						cancelPortForwarding(p.Name)
					}
				}
			}(currentPod)
			portForwarding[currentPod.Name] = MenuItem{
				Item:  menuItem,
				Title: currentPod.Name,
			}
		}
	}
}

func (pod Pod) getName() string {
	return fmt.Sprintf("%s %s Age: %s", pod.Name, pod.Ready, pod.Age)
}

func (pod Pod) getAge() string {
	seconds := time.Now().Local().UTC().Unix() - currentPod.CreatedAt.Unix()
	minutes := seconds / 60
	hours := minutes / 60
	days := hours / 24
	age := fmt.Sprintf("%ds", seconds)
	if minutes > 3 {
		age = fmt.Sprintf("%dm%ds", minutes, seconds-minutes*60)
	}
	if minutes > 10 {
		age = fmt.Sprintf("%dm", minutes)
	}
	if hours == 2 {
		age = fmt.Sprintf("%dh%dm", hours, minutes-hours*60)
	}
	if hours > 2 {
		age = fmt.Sprintf("%dh", hours)
	}
	if days > 0 {
		age = fmt.Sprintf("%dd", days)
	}

	return age
}

type PodData struct {
	Metadata PodMetadata `json:"metadata"`
	Spec     PodSpec     `json:"spec"`
	Status   PodStatus   `json:"status"`
}

type PodMetadata struct {
	CreationTimestamp string `json:"creationTimestamp"`
}

type PodSpec struct {
	Containers []PodContainer `json:"containers"`
}

type PodStatus struct {
	ContainerStatuses []PodContainerStatus `json:"containerStatuses"`
}

type PodContainer struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

type PodContainerStatus struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

func (pod *Pod) getContainers() error {
	cmd := fmt.Sprintf("kubectl get pods %s -o json", pod.Name)
	rawPod, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		return err
	}
	var podData PodData
	err = json.Unmarshal(rawPod, &podData)
	if err != nil {
		return err
	}
	for _, container := range podData.Spec.Containers {
		ready := "false"
		for _, containerStatus := range podData.Status.ContainerStatuses {
			if containerStatus.Name == container.Name {
				if containerStatus.Ready {
					ready = "true"
				}
			}
		}
		pod.Containers = append(pod.Containers, Container{
			Name:  container.Name,
			Image: container.Image,
			Ready: ready,
		})
	}
	pod.CreatedAt, err = time.ParseInLocation(time.RFC3339, podData.Metadata.CreationTimestamp, time.UTC)
	if err != nil {
		return err
	}

	return nil
}

func watchPods() {
	if podUpdateCh != nil {
		return
	}
	podUpdateCh = make(chan string)
	go func() {
		var last string
		tick := time.Tick(1 * time.Second)
		for {
			select {
			case <-tick:
				rawPodNames, err := exec.Command("bash", "-c", fmt.Sprintf("kubectl get pods -o jsonpath='{.items[*].metadata.name}'")).Output()
				if err != nil {
					log.Println(err)
					notify.Warning("ERROR!", err.Error())

					return
				}
				if last == "" {
					last = string(rawPodNames)
				} else {
					if last != string(rawPodNames) {
						last = string(rawPodNames)
						podUpdateCh <- string(rawPodNames)
					}
				}
			}
		}
	}()
	go func() {
		for {
			select {
			case podNamesString := <-podUpdateCh:
				podNames := strings.Split(podNamesString, " ")
				currentPodExists := false
				newPods := false
				missingPods := false
				for _, pod := range podNames {
					if pod == currentPod.Name {
						currentPodExists = true
					}
					if _, ok := podMenuItems[pod]; !ok {
						newPods = true
					}
				}
				for pod := range podMenuItems {
					missing := true
					for _, podName := range podNames {
						if pod == podName {
							missing = false
							break
						}
					}
					if missing {
						missingPods = true
						break
					}
				}
				if newPods || missingPods {
					if currentPod.Name != "" && currentOpenPod != nil && !currentPodExists {
						notify.Warning("Warning!", "Pods have updated. Closing current window in 15 seconds...")
						tick := time.Tick(15 * time.Second)
						go func() {
							closed := false
							for {
								select {
								case <-tick:
									currentOpenPod.Close()
									closed = true
								}
								if closed {
									break
								}
							}
						}()
					}
					SetPods()
				}
			}
		}
	}()
}

func (pod Pod) ssh(container string) error {
	return exec.Command("xterm", "-title", "SSH: "+currentPod.Name+" "+container, "-geometry", getWindowGeometry(), "-e", "kubectl exec -it -c "+container+" "+pod.Name+" -- bash").Run()
}

func (pod Pod) logs(container string) error {
	return exec.Command("xterm", "-title", "Logs: "+currentPod.Name+" "+container, "-geometry", getWindowGeometry(), "-e", "kubectl logs -f --tail="+tailString+" --timestamps=true -c "+container+" "+pod.Name).Run()
}

func (pod Pod) portForward(ctx context.Context) {
	exec.CommandContext(ctx, "bash", "-c", fmt.Sprintf("kubectl port-forward %s %s:%s", currentPod.Name, portFromString, portToString)).Run()
}

func cancelPortForwarding(name string) {
	if _, ok := portForwarding[currentPod.Name]; ok {
		if cancelFunc, ok := portForwardCancel[name]; ok {
			cancelFunc()
		}
		portForwarding[name].Item.Remove()
		delete(portForwarding, name)
		if len(portForwarding) < 1 {
			portForwardMenuItem.Hide()
		}
	}
}
