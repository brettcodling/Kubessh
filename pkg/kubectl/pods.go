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
	currentPod          Pod
	currentOpenPod      nucular.MasterWindow
	currentOpenPods     nucular.MasterWindow
	pods                []*Pod
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
	portForwarding = make(map[string]MenuItem)
	portForwardCancel = make(map[string]context.CancelFunc)
	portFrom.Flags = nucular.EditField
	portFrom.SingleLine = true
	portTo.Flags = nucular.EditField
	portTo.SingleLine = true
}

func AddPortForwarding() {
	portForwardMenuItem = systray.AddMenuItem("Port Forwarding:", "")
	portForwardMenuItem.Hide()
}

func getPods() {
	pods = []*Pod{}
	cmd := "kubectl get pods --no-headers"
	rawPods, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		log.Println(err)
		notify.Warning("ERROR!", err.Error())

		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(rawPods)))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		columns := strings.Fields(scanner.Text())
		pod := Pod{
			Name:     columns[0],
			Ready:    columns[1],
			Status:   columns[2],
			Restarts: columns[3],
			Age:      columns[4],
		}
		pod.getContainers()
		pods = append(pods, &pod)
	}
}

func OpenPods() {
	if currentOpenPods != nil {
		currentOpenPods.Close()
	}
	getPods()
	watchPods()
	currentOpenPods = nucular.NewMasterWindow(0, "Pods: "+getCurrentContext().Name, updatePods)
	currentOpenPods.SetStyle(style.FromTheme(style.DarkTheme, 2.0))
	currentOpenPods.Main()
}

func updatePods(w *nucular.Window) {
	for _, pod := range pods {
		w.Row(30).Dynamic(1)
		podOpen := w.TreePush(nucular.TreeNode, pod.Name, false)
		if podOpen {
			w.Row(25).Dynamic(8)
			podDetails(w, *pod)
			w.Spacing(1)
			if w.ButtonText(">>") {
				go func(pod Pod) {
					currentPod = pod
					openPod()
				}(*pod)
			}
			w.TreePop()
		}
	}
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

func podDetails(w *nucular.Window, pod Pod) {
	w.Label("Ready:", "LC")
	w.Label(pod.Ready, "LC")
	w.Label("Restarts:", "LC")
	w.Label(pod.Restarts, "LC")
	w.Label("Age:", "LC")
	w.Label(pod.getAge(), "LC")
}

func updatePod(w *nucular.Window) {
	w.Row(40).Dynamic(1)
	w.Label("Details", "LC")
	w.Row(30).Dynamic(2)
	podDetails(w, currentPod)
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
	seconds := time.Now().Local().UTC().Unix() - pod.CreatedAt.Unix()
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
				if !newPods {
					for _, pod := range podNames {
						isNew := true
						for _, existingPod := range pods {
							if existingPod.Name == pod {
								isNew = false
								break
							}
						}
						newPods = isNew
						if newPods || currentPodExists {
							break
						}
					}
				}
				if !missingPods {
					for _, pod := range pods {
						missing := true
						for _, podName := range podNames {
							if pod.Name == podName {
								missing = false
								break
							}
						}
						missingPods = missing
						if missingPods {
							break
						}
					}
				}
				if newPods || missingPods || !currentPodExists {
					getPods()
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
