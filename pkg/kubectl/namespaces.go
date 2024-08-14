package kubectl

import (
	"bufio"
	"errors"
	"log"
	"os/exec"
	"strings"

	"github.com/brettcodling/Kubessh/pkg/notify"
	"github.com/brettcodling/systray"
)

type Namespace struct {
	Name  string
	InUse bool
}

var (
	Namespaces         []*Namespace
	namespaceMenuItems []MenuItem
	namespacesMenuItem *systray.MenuItem
)

func AddNamespaces() {
	namespacesMenuItem = systray.AddMenuItem("", "")
	namespacesMenuItem.Hide()
}

func getCurrentNamespace() *Namespace {
	currentContext := getCurrentContext()
	if currentContext.Namespace == "" {
		return &Namespace{}
	}

	for _, namespace := range Namespaces {
		if namespace.Name == currentContext.Namespace {
			return namespace
		}
	}

	return &Namespace{
		Name:  currentContext.Namespace,
		InUse: true,
	}
}

func GetNamespaces() []*Namespace {
	Namespaces = []*Namespace{}
	currentNamespace := getCurrentNamespace().Name
	cmd := "kubectl get namespaces -o name"
	rawNamespaces, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		log.Println(err)
		notify.Warning("ERROR!", err.Error())

		return Namespaces
	}
	scanner := bufio.NewScanner(strings.NewReader(string(rawNamespaces)))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		namespace := Namespace{
			Name: scanner.Text()[10:],
		}
		namespace.InUse = namespace.Name == currentNamespace
		Namespaces = append(Namespaces, &namespace)
	}

	return Namespaces
}

func SetNamespaces() {
	if namespacesMenuItem == nil {
		namespacesMenuItem = systray.AddMenuItem("", "")
	}
	for _, namespaceMenuItem := range namespaceMenuItems {
		namespaceMenuItem.Item.Remove()
	}
	namespaceMenuItems = []MenuItem{}
	for _, n := range GetNamespaces() {
		namespace := namespacesMenuItem.AddSubMenuItem("", "")
		item := MenuItem{
			Item:  namespace,
			Title: n.Name,
		}
		if n.InUse {
			item.Title = "* " + item.Title
		}
		item.Item.SetTitle(item.Title)
		namespaceMenuItems = append(namespaceMenuItems, item)
		go func(n *Namespace) {
			for {
				select {
				case <-namespace.ClickedCh:
					if !n.InUse {
						go func(n *Namespace) {
							n.Use()
							for _, namespaceItem := range namespaceMenuItems {
								namespaceItem.Title = strings.TrimLeft(namespaceItem.Title, "* ")
								namespaceItem.Item.SetTitle(namespaceItem.Title)
							}
							namespace.SetTitle("* " + n.Name)
							namespacesMenuItem.SetTitle("Namespace: " + n.Name)
						}(n)
					}
				}
			}
		}(n)
	}

	namespacesMenuItem.SetTitle("Namespace: " + getCurrentNamespace().Name)
	namespacesMenuItem.Show()
}

func (namespace Namespace) Use() error {
	currentNamespace := getCurrentNamespace()
	if namespace.Name == currentNamespace.Name {
		return nil
	}

	cmd := "kubectl config set-context --current --namespace \"" + namespace.Name + "\""
	rawResult, err := exec.Command("bash", "-c", cmd).CombinedOutput()
	result := strings.TrimSpace(string(rawResult))
	if len(result) < 9 || result[:7] != "Context" || result[len(result)-9:] != "modified." {
		if len(result) >= 5 && result[:5] == "error" {
			return errors.New(result)
		}
	}
	if err != nil {
		return err
	}
	GetContexts()

	for key, n := range Namespaces {
		if n.InUse {
			n.InUse = false
		}
		if namespace.Name == n.Name {
			n.InUse = true
		}
		Namespaces[key] = n
	}

	return nil
}
