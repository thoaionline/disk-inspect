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
