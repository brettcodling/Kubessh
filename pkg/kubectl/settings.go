package kubectl

import (
	"github.com/aarzilli/nucular"
	"github.com/aarzilli/nucular/style"
	"github.com/brettcodling/Kubessh/pkg/database"
)

var (
	windowWidth, windowHeight, tail                   nucular.TextEditor
	windowWidthString, windowHeightString, tailString string
)

func init() {
	windowWidthString = database.Get("WINDOW_WIDTH")
	if windowWidthString == "" {
		windowWidthString = "300"
	}
	windowWidth.Flags = nucular.EditField
	windowWidth.SingleLine = true

	windowHeightString = database.Get("WINDOW_HEIGHT")
	if windowHeightString == "" {
		windowHeightString = "50"
	}
	windowHeight.Flags = nucular.EditField
	windowHeight.SingleLine = true

	tailString = database.Get("TAIL")
	if tailString == "" {
		tailString = "10"
	}
	tail.Flags = nucular.EditField
	tail.SingleLine = true
}

func getWindowGeometry() string {
	return windowWidthString + "x" + windowHeightString
}

func OpenSettings() {
	windowWidth.SelectAll()
	windowWidth.Text([]rune(windowWidthString))
	windowHeight.SelectAll()
	windowHeight.Text([]rune(windowHeightString))
	tail.SelectAll()
	tail.Text([]rune(tailString))
	wnd := nucular.NewMasterWindow(0, "Settings", updateSettings)
	wnd.SetStyle(style.FromTheme(style.DarkTheme, 2.0))
	wnd.Main()
}

func updateSettings(w *nucular.Window) {
	w.Row(40).Dynamic(1)
	w.Label("Window:", "LC")
	w.Row(30).Dynamic(2)
	w.Label("Width:", "LC")
	windowWidth.Edit(w)
	w.Row(30).Dynamic(2)
	w.Label("Height:", "LC")
	windowHeight.Edit(w)
	w.Row(40).Dynamic(1)
	w.Label("Logs:", "LC")
	w.Row(30).Dynamic(2)
	w.Label("Tail:", "LC")
	tail.Edit(w)
	w.Row(30).Dynamic(1)
	if w.ButtonText("Save") {
		windowWidthString = string(windowWidth.Buffer)
		database.Set("WINDOW_WIDTH", windowWidthString)
		windowHeightString = string(windowHeight.Buffer)
		database.Set("WINDOW_HEIGHT", windowHeightString)
		tailString = string(tail.Buffer)
		database.Set("TAIL", tailString)
		w.Master().Close()
	}
}
