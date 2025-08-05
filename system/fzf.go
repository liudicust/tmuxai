package system

import (
	"fmt"
	"strings"

	"github.com/jroimartin/gocui"
)

var (
	finalSelection []string
	userCancelled  bool
)

// InteractiveSelect displays an interactive list for the user to select from.
// It takes a slice of items to display and a map of already selected items.
// It returns a slice of the newly selected items.
func InteractiveSelect(items []string, initiallySelected map[string]struct{}) ([]string, error) {
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		return nil, err
	}
	defer g.Close()

	// Reset global state
	finalSelection = nil
	userCancelled = true

	selected := make(map[string]struct{})
	for item := range initiallySelected {
		selected[item] = struct{}{}
	}

	g.SetManagerFunc(layout(items, selected))

	if err := keybindings(g, items, selected); err != nil {
		return nil, err
	}

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		return nil, err
	}

	if userCancelled {
		return nil, nil // Return nil, nil for cancellation, similar to fzf behavior
	}

	return finalSelection, nil
}

func layout(items []string, selected map[string]struct{}) func(*gocui.Gui) error {
	return func(g *gocui.Gui) error {
		maxX, maxY := g.Size()
		width := 60
		height := len(items) + 2
		if height > 20 {
			height = 20
		}

		x0, y0 := maxX/2-width/2, maxY/2-height/2
		x1, y1 := maxX/2+width/2, maxY/2+height/2

		if v, err := g.SetView("select", x0, y0, x1, y1); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Title = "Select Servers (Space: toggle, Enter: confirm, q/Ctrl-C: quit)"
			v.Highlight = true
			v.SelBgColor = gocui.ColorGreen
			v.SelFgColor = gocui.ColorBlack
			v.SetCursor(0, 0)
			v.Clear()
			fmt.Fprint(v, string(formatItems(items, selected)))
			if _, err := g.SetCurrentView("select"); err != nil {
				return err
			}
		}
		return nil
	}
}

func keybindings(g *gocui.Gui, items []string, selected map[string]struct{}) error {
	quit := func(g *gocui.Gui, v *gocui.View) error {
		return gocui.ErrQuit
	}

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	if err := g.SetKeybinding("select", 'q', gocui.ModNone, quit); err != nil {
		return err
	}
	if err := g.SetKeybinding("select", gocui.KeyArrowDown, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			cursorDown(v, len(items))
			return nil
		}); err != nil {
		return err
	}
	if err := g.SetKeybinding("select", 'j', gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			cursorDown(v, len(items))
			return nil
		}); err != nil {
		return err
	}
	if err := g.SetKeybinding("select", gocui.KeyArrowUp, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			cursorUp(v)
			return nil
		}); err != nil {
		return err
	}
	if err := g.SetKeybinding("select", 'k', gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			cursorUp(v)
			return nil
		}); err != nil {
		return err
	}
	if err := g.SetKeybinding("select", gocui.KeySpace, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			toggleSelection(v, items, selected)
			v.Clear()
			fmt.Fprint(v, string(formatItems(items, selected)))
			return nil
		}); err != nil {
		return err
	}
	if err := g.SetKeybinding("select", gocui.KeyEnter, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			userCancelled = false
			finalSelection = []string{}
			for item := range selected {
				finalSelection = append(finalSelection, item)
			}
			return gocui.ErrQuit
		}); err != nil {
		return err
	}
	return nil
}

func cursorDown(v *gocui.View, numItems int) {
	if v != nil {
		cx, cy := v.Cursor()
		if cy < numItems-1 {
			v.SetCursor(cx, cy+1)
		}
	}
}

func cursorUp(v *gocui.View) {
	if v != nil {
		cx, cy := v.Cursor()
		if cy > 0 {
			v.SetCursor(cx, cy-1)
		}
	}
}

func toggleSelection(v *gocui.View, items []string, selected map[string]struct{}) {
	_, cy := v.Cursor()
	if cy >= 0 && cy < len(items) {
		item := items[cy]
		if _, ok := selected[item]; ok {
			delete(selected, item)
		} else {
			selected[item] = struct{}{}
		}
	}
}

func formatItems(items []string, selected map[string]struct{}) []byte {
	var b strings.Builder
	for _, item := range items {
		prefix := "[ ]"
		if _, ok := selected[item]; ok {
			prefix = "[x]"
		}
		fmt.Fprintf(&b, "%s %s\n", prefix, item)
	}
	return []byte(b.String())
}
