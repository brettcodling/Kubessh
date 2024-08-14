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

type MenuItem struct {
	Item  *systray.MenuItem
	Title string
}

type Context struct {
	Name      string
	Namespace string
	InUse     bool
}

var (
	Contexts         []*Context
	contextMenuItems []MenuItem
	contextsMenuItem *systray.MenuItem
)

func AddContexts() {
	contextsMenuItem = systray.AddMenuItem("", "")
	contextsMenuItem.Hide()
}

func GetContexts() []*Context {
	Contexts = []*Context{}
	cmd := "kubectl config get-contexts --no-headers"
	rawContexts, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		log.Println(err)
		notify.Warning("ERROR!", err.Error())

		return Contexts
	}
	scanner := bufio.NewScanner(strings.NewReader(string(rawContexts)))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		var context Context
		columns := strings.Fields(scanner.Text())
		if columns[0] == "*" {
			context.InUse = true
			columns = columns[1:]
		}
		context.Name = columns[0]
		if len(columns) == 4 {
			context.Namespace = columns[3]
		}
		Contexts = append(Contexts, &context)
	}

	return Contexts
}

func getCurrentContext() *Context {
	cmd := "kubectl config current-context"
	rawCurrentContext, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		log.Println(err)
		notify.Warning("ERROR!", err.Error())

		return &Context{}
	}

	current := strings.TrimSpace(string(rawCurrentContext))
	for _, context := range Contexts {
		if context.Name == current {
			return context
		}
	}

	return &Context{}
}

func SetContexts() {
	for _, contextMenuItem := range contextMenuItems {
		contextMenuItem.Item.Remove()
	}
	contextMenuItems = []MenuItem{}
	for _, c := range GetContexts() {
		context := contextsMenuItem.AddSubMenuItem("", "")
		item := MenuItem{
			Item:  context,
			Title: c.Name,
		}
		if c.InUse {
			item.Title = "* " + item.Title
		}
		item.Item.SetTitle(item.Title)
		contextMenuItems = append(contextMenuItems, item)
		go func(c *Context) {
			for {
				select {
				case <-context.ClickedCh:
					if !c.InUse {
						go func(c *Context) {
							c.Use()
							for _, contextItem := range contextMenuItems {
								contextItem.Title = strings.TrimLeft(contextItem.Title, "* ")
								contextItem.Item.SetTitle(contextItem.Title)
							}
							context.SetTitle("* " + c.Name)
							contextsMenuItem.SetTitle("Context: " + c.Name)
							SetNamespaces()
						}(c)
					}
				}
			}
		}(c)
	}

	contextsMenuItem.SetTitle("Context: " + getCurrentContext().Name)
	contextsMenuItem.Show()
}

func (context Context) Use() error {
	if context.Name == getCurrentContext().Name {
		return nil
	}

	cmd := "kubectl config use-context \"" + context.Name + "\""
	rawResult, err := exec.Command("bash", "-c", cmd).CombinedOutput()
	result := strings.TrimSpace(string(rawResult))
	if len(result) < 19 || result[0:19] != "Switched to context" {
		if len(result) >= 5 && result[:5] == "error" {
			return errors.New(result)
		}
	}
	if err != nil {
		return err
	}

	for key, c := range Contexts {
		if c.InUse {
			c.InUse = false
		}
		if context.Name == c.Name {
			c.InUse = true
		}
		Contexts[key] = c
	}

	return nil
}
