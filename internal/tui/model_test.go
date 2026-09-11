package tui

import (
	"context"
	"strings"
	"testing"

	"disk-inspect/internal/scan"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func fixture(t *testing.T) *Model {
	t.Helper()
	m := New(context.Background(), "/root", false)
	t.Cleanup(m.Close)
	root := &scan.Node{Name: "root", Path: "/root", Dir: true, Allocated: 100, Apparent: 100, Files: 2, Dirs: 1}
	folder := &scan.Node{Name: "folder", Path: "/root/folder", Dir: true, Allocated: 80, Apparent: 20, Files: 1, Parent: root}
	folder.Children = []*scan.Node{{Name: "nested", Parent: folder, Allocated: 80, Apparent: 20}}
	root.Children = []*scan.Node{folder, {Name: "big.txt", Path: "/root/big.txt", Allocated: 20, Apparent: 80, Files: 1, Parent: root}}
	m.Update(scanDone{root: root})
	return m
}

func press(m *Model, key string) {
	switch key {
	case "enter":
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	case "esc":
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	default:
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	}
}

func TestNavigationFilteringAndModes(t *testing.T) {
	m := fixture(t)
	if m.rows[0].Name != "folder" {
		t.Fatal("not sorted by size")
	}
	press(m, "enter")
	if m.current.Name != "folder" || len(m.rows) != 1 {
		t.Fatal("failed to enter folder")
	}
	press(m, "h")
	if m.current != m.root || m.rows[m.cursor].Name != "folder" {
		t.Fatal("parent selection not restored")
	}
	press(m, "/")
	press(m, "BIG")
	if len(m.rows) != 1 || m.rows[0].Name != "big.txt" {
		t.Fatal("case-insensitive filter failed")
	}
	press(m, "esc")
	if len(m.rows) != 2 || m.filtering {
		t.Fatal("filter not cleared")
	}
	if scan.HasAllocated {
		press(m, "a")
		if m.rows[0].Name != "big.txt" {
			t.Fatal("apparent mode did not reorder")
		}
	}
	press(m, "s")
	if m.rows[0].Name != "big.txt" {
		t.Fatal("name sort failed")
	}
	press(m, "/")
	press(m, "does not exist")
	press(m, "enter")
	press(m, "enter")
	if len(m.rows) != 0 || m.cursor != 0 {
		t.Fatal("empty filtered view invalid")
	}
	if !strings.Contains(m.View(), "No matching") {
		t.Fatal("missing empty-filter explanation")
	}
}

func TestViewFitsAndSanitizesFilenames(t *testing.T) {
	m := fixture(t)
	m.root.Children[0].Name = "bad\x1b[2J\n\t" + strings.Repeat("界", 80)
	for _, size := range [][2]int{{20, 8}, {48, 16}, {80, 24}, {120, 40}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) > size[1] {
			t.Fatalf("view too tall: %d > %d", len(lines), size[1])
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("view too wide: %q", line)
			}
		}
		if strings.Contains(view, "\x1b[2J") {
			t.Fatal("filename injected terminal control")
		}
	}
}

func TestRefreshRestoresFolder(t *testing.T) {
	m := fixture(t)
	press(m, "enter")
	press(m, "r")
	if !m.scanning || m.restorePath != "/root/folder" {
		t.Fatal("refresh did not preserve path")
	}
	m.Update(scanDone{root: m.root})
	if m.current.Name != "folder" {
		t.Fatal("refresh lost current directory")
	}
}

func TestEscapeBackAndDismissal(t *testing.T) {
	m := fixture(t)
	m.apparent = false
	m.rebuild()
	press(m, "enter")
	folder := m.current
	press(m, "?")
	press(m, "esc")
	if m.help || m.current != folder {
		t.Fatal("escape should close help without navigating")
	}
	press(m, "/")
	press(m, "nested")
	press(m, "esc")
	if m.filtering || m.query != "" || m.current != folder {
		t.Fatal("escape should cancel filter editing without navigating")
	}
	press(m, "/")
	press(m, "nested")
	press(m, "enter")
	press(m, "esc")
	if m.query != "" || m.current != folder {
		t.Fatal("escape should clear an applied filter without navigating")
	}
	press(m, "esc")
	if m.current != m.root || m.rows[m.cursor] != folder {
		t.Fatal("escape should return to parent and restore selection")
	}
	press(m, "esc")
	if m.current != m.root {
		t.Fatal("escape should stay within scan root")
	}
}

func TestSelectionStaysVisible(t *testing.T) {
	m := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	for i := 0; i < 30; i++ {
		m.root.Children = append(m.root.Children, &scan.Node{Name: "entry", Parent: m.root})
	}
	m.rebuild()
	press(m, "G")
	if m.cursor != len(m.rows)-1 || m.cursor >= m.offset+m.pageSize() {
		t.Fatal("last entry not visible")
	}
	press(m, "g")
	press(m, "k")
	if m.cursor != 0 || m.offset != 0 {
		t.Fatal("selection moved before first row")
	}
}

func TestRootShareAcrossNavigation(t *testing.T) {
	m := fixture(t)
	m.apparent = false
	m.rebuild()
	assertShare := func(want string) {
		t.Helper()
		if got := ansi.Strip(m.rootShare()); !strings.Contains(got, want) {
			t.Fatalf("root share = %q, want %q", got, want)
		}
	}
	assertShare("80.0%")
	press(m, "j")
	assertShare("20.0%")
	press(m, "k")
	press(m, "enter")
	// The nested file is 100% of its parent but only 80% of the root.
	assertShare("80.0%")
	m.apparent = true
	m.rebuild()
	assertShare("20.0%")
	press(m, "/")
	press(m, "nested")
	assertShare("20.0%")
	press(m, "missing")
	assertShare("no selection")
	press(m, "esc")
	m.root.Apparent = 0
	assertShare("0.0%")
	m.root.Errors = 1
	assertShare("(partial)")
}

func TestRootShareVisibleInSmallTerminal(t *testing.T) {
	m := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 48, Height: 16})
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Selected / root") || !strings.Contains(view, "q quit") {
		t.Fatalf("root bar or footer clipped: %s", view)
	}
}

func TestMouseWheelNavigation(t *testing.T) {
	m := fixture(t)
	m.apparent = false
	m.rebuild()
	wheel := func(button tea.MouseButton, shift bool) {
		m.Update(tea.MouseMsg{Button: button, Action: tea.MouseActionPress, Shift: shift})
	}
	wheel(tea.MouseButtonWheelDown, false)
	if m.cursor != 1 {
		t.Fatal("wheel down did not move selection")
	}
	wheel(tea.MouseButtonWheelRight, false)
	if m.current != m.root {
		t.Fatal("wheel right entered a file")
	}
	wheel(tea.MouseButtonWheelUp, false)
	if m.cursor != 0 {
		t.Fatal("wheel up did not move selection")
	}
	wheel(tea.MouseButtonWheelRight, false)
	if m.current.Name != "folder" {
		t.Fatal("wheel right did not open folder")
	}
	wheel(tea.MouseButtonWheelLeft, false)
	if m.current != m.root || m.rows[m.cursor].Name != "folder" {
		t.Fatal("wheel left did not restore parent selection")
	}
	wheel(tea.MouseButtonWheelLeft, false)
	if m.current != m.root {
		t.Fatal("wheel left escaped scan root")
	}
	wheel(tea.MouseButtonWheelDown, true)
	if m.current.Name != "folder" {
		t.Fatal("shift+wheel down did not open folder")
	}
	wheel(tea.MouseButtonWheelUp, true)
	if m.current != m.root {
		t.Fatal("shift+wheel up did not return to parent")
	}
	press(m, "/")
	press(m, "missing")
	press(m, "enter")
	wheel(tea.MouseButtonWheelRight, false)
	if m.current != m.root || m.cursor != 0 {
		t.Fatal("wheel navigation mishandled empty filter")
	}
}

func TestMouseWheelIgnoresInactiveViews(t *testing.T) {
	for _, state := range []string{"filter", "help", "scan", "release", "motion", "click"} {
		t.Run(state, func(t *testing.T) {
			m := fixture(t)
			msg := tea.MouseMsg{Button: tea.MouseButtonWheelRight, Action: tea.MouseActionPress}
			switch state {
			case "filter":
				m.filtering = true
			case "help":
				m.help = true
			case "scan":
				m.scanning = true
			case "release":
				msg.Action = tea.MouseActionRelease
			case "motion":
				msg.Action = tea.MouseActionMotion
			case "click":
				msg.Button = tea.MouseButtonLeft
			}
			m.Update(msg)
			if m.current != m.root || m.cursor != 0 {
				t.Fatal("unexpected mouse navigation")
			}
		})
	}
}
