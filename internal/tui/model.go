package tui

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"disk-inspect/internal/scan"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	accent   = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	muted    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	bright   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	warning  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	selected = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("86")).Bold(true)
)

type scanDone struct {
	root *scan.Node
	err  error
}
type tick time.Time

type Model struct {
	ctx             context.Context
	cancel          context.CancelFunc
	path            string
	root, current   *scan.Node
	rows            []*scan.Node
	progress        *scan.Progress
	scanning        bool
	started         time.Time
	elapsed         time.Duration
	err             error
	width, height   int
	cursor, offset  int
	apparent        bool
	sortBy          int
	query           string
	filtering, help bool
	frame           int
	restorePath     string
}

func New(ctx context.Context, path string, apparent bool) *Model {
	ctx, cancel := context.WithCancel(ctx)
	return &Model{ctx: ctx, cancel: cancel, path: path, apparent: apparent || !scan.HasAllocated, width: 80, height: 24}
}

func (m *Model) Close()        { m.cancel() }
func (m *Model) Err() error    { return m.err }
func (m *Model) Init() tea.Cmd { return m.startScan() }

func pulse() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tick(t) })
}

func (m *Model) startScan() tea.Cmd {
	m.scanning, m.err = true, nil
	m.started = time.Now()
	m.progress = &scan.Progress{}
	ctx, path, progress := m.ctx, m.path, m.progress
	return tea.Batch(func() tea.Msg { root, err := scan.Scan(ctx, path, progress); return scanDone{root, err} }, pulse())
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clamp()
	case tick:
		m.frame++
		if m.scanning {
			return m, pulse()
		}
	case scanDone:
		m.scanning, m.err = false, msg.err
		m.elapsed = time.Since(m.started)
		if msg.err == nil {
			m.root, m.current = msg.root, msg.root
			// Preserve the open directory on refresh if it still exists.
			if rel, err := filepath.Rel(msg.root.Path, m.restorePath); err == nil && rel != "." {
				for _, part := range strings.Split(rel, string(filepath.Separator)) {
					var next *scan.Node
					for _, child := range m.current.Children {
						if child.Dir && child.Name == part {
							next = child
							break
						}
					}
					if next == nil {
						break
					}
					m.current = next
				}
			}
			m.cursor, m.offset = 0, 0
			m.rebuild()
		}
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			m.Close()
			return m, tea.Quit
		}
		if m.filtering {
			switch key {
			case "enter":
				m.filtering = false
			case "esc":
				m.filtering = false
				m.query = ""
			case "backspace":
				r := []rune(m.query)
				if len(r) > 0 {
					m.query = string(r[:len(r)-1])
				}
			case "ctrl+u":
				m.query = ""
			default:
				if msg.Type == tea.KeyRunes {
					m.query += safe(string(msg.Runes))
				}
			}
			m.cursor, m.offset = 0, 0
			m.rebuild()
			return m, nil
		}
		if key == "q" {
			m.Close()
			return m, tea.Quit
		}
		if key == "?" {
			m.help = !m.help
			return m, nil
		}
		if m.help {
			if key == "esc" {
				m.help = false
			}
			return m, nil
		}
		if m.scanning {
			return m, nil
		}
		if key == "r" {
			if m.current != nil {
				m.restorePath = m.current.Path
			}
			return m, m.startScan()
		}
		if m.current == nil {
			return m, nil
		}
		switch key {
		case "down", "j":
			m.cursor++
		case "up", "k":
			m.cursor--
		case "pgdown", "ctrl+d":
			m.cursor += m.pageSize()
		case "pgup", "ctrl+u":
			m.cursor -= m.pageSize()
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = len(m.rows) - 1
		case "enter", "right", "l":
			if len(m.rows) > 0 && m.rows[m.cursor].Dir {
				m.current = m.rows[m.cursor]
				m.resetView()
			}
		case "left", "backspace", "h":
			if m.current.Parent != nil {
				old := m.current
				m.current = old.Parent
				m.resetView()
				for i, row := range m.rows {
					if row == old {
						m.cursor = i
						break
					}
				}
			}
		case "~":
			m.current = m.root
			m.resetView()
		case "/":
			m.filtering = true
		case "esc":
			m.query = ""
			m.rebuild()
		case "s":
			m.sortBy = (m.sortBy + 1) % 3
			m.cursor = 0
			m.rebuild()
		case "a":
			if scan.HasAllocated {
				m.apparent = !m.apparent
				m.rebuild()
			}
		}
		m.clamp()
	}
	return m, nil
}

func (m *Model) resetView() { m.query = ""; m.cursor, m.offset = 0, 0; m.rebuild() }

func (m *Model) rebuild() {
	m.rows = nil
	if m.current == nil {
		return
	}
	for _, child := range m.current.Children {
		if strings.Contains(strings.ToLower(safe(child.Name)), strings.ToLower(m.query)) {
			m.rows = append(m.rows, child)
		}
	}
	sort.Slice(m.rows, func(i, j int) bool {
		a, b := m.rows[i], m.rows[j]
		switch m.sortBy {
		case 0:
			if a.Size(m.apparent) != b.Size(m.apparent) {
				return a.Size(m.apparent) > b.Size(m.apparent)
			}
		case 2:
			if a.Files != b.Files {
				return a.Files > b.Files
			}
		}
		return a.Name < b.Name
	})
	m.clamp()
}

func (m *Model) pageSize() int { return max(1, m.height-13) }
func (m *Model) clamp() {
	m.cursor = max(0, min(m.cursor, len(m.rows)-1))
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.pageSize() {
		m.offset = m.cursor - m.pageSize() + 1
	}
	m.offset = max(0, min(m.offset, max(0, len(m.rows)-m.pageSize())))
}

func (m *Model) View() string {
	if m.width < 48 || m.height < 16 {
		return fit("disk-inspect\nResize terminal to at least 48 × 16.\nq / ctrl+c quit", m.width, m.height)
	}
	mode := "allocated"
	if m.apparent {
		mode = "apparent"
	}
	lines := []string{bright.Render(" DISK INSPECT") + muted.Render("  /  find where your space goes"), ""}
	if m.help {
		lines = append(lines, accent.Render(" Keyboard controls"), "", " ↑/↓  j/k       Move selection", " →/enter  l     Open directory", " ←/backspace h  Parent directory", " ~              Return to scan root", " pgup/pgdn g/G  Page / first / last", " /              Filter entries by name", " enter / esc    Apply / clear filter", " s              Sort: size → name → files", " a              Toggle allocated / apparent size", " r              Rescan root, preserve open folder", " ? / esc        Close help", " q / ctrl+c     Quit", "", muted.Render(" Symlinks are not followed. Hidden entries are included."))
		return fit(strings.Join(lines, "\n"), m.width, m.height)
	}
	if m.scanning {
		spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}[m.frame%10]
		lines = append(lines, " "+accent.Render(spinner+" Scanning")+"  "+safe(m.path), "", fmt.Sprintf(" %s entries  ·  %s found  ·  %s", number(m.progress.Entries.Load()), Bytes(m.progress.Bytes.Load()), time.Since(m.started).Round(time.Second)), "", muted.Render(" Totals appear when the scan completes."), muted.Render(fmt.Sprintf(" %d read errors so far", m.progress.Errors.Load())), "", " q quit  ·  ? help")
		return fit(strings.Join(lines, "\n"), m.width, m.height)
	}
	if m.err != nil {
		lines = append(lines, warning.Render(" Scan failed"), " "+safe(m.err.Error()), "", " r retry  ·  q quit")
		return fit(strings.Join(lines, "\n"), m.width, m.height)
	}
	if m.current == nil {
		return " Preparing scan…"
	}
	n := m.current
	lines = append(lines, " "+accent.Render(safe(n.Path)), fmt.Sprintf(" %s %s  ·  %s files  ·  %s folders", bright.Render(Bytes(n.Size(m.apparent))), mode, number(n.Files), number(n.Dirs)))
	status := fmt.Sprintf(" Scan %s  ·  sort: %s", m.elapsed.Round(time.Millisecond), []string{"size ↓", "name ↑", "files ↓"}[m.sortBy])
	if n.Errors > 0 {
		status += warning.Render(fmt.Sprintf("  ·  %d read errors — totals incomplete", n.Errors))
	}
	lines = append(lines, muted.Render(status))
	filter := " / filter entries"
	if m.query != "" || m.filtering {
		filter = " / " + safe(m.query)
		if m.filtering {
			filter += "▏"
		}
	}
	lines = append(lines, accent.Render(filter), "", muted.Render("       SIZE   SHARE  "+func() string {
		if m.width >= 76 {
			return "              "
		}
		return ""
	}()+"NAME"))
	page := m.pageSize()
	for i := m.offset; i < m.offset+page; i++ {
		if i >= len(m.rows) {
			if i == m.offset {
				lines = append(lines, muted.Render("  No entries"))
			} else {
				lines = append(lines, "")
			}
			continue
		}
		row := m.rows[i]
		share := float64(0)
		if n.Size(m.apparent) > 0 {
			share = float64(row.Size(m.apparent)) / float64(n.Size(m.apparent))
		}
		marker := "  "
		if i == m.cursor {
			marker = "› "
		}
		prefix := fmt.Sprintf("%s%9s %6.1f%%  ", marker, Bytes(row.Size(m.apparent)), share*100)
		if m.width >= 76 {
			filled := min(12, max(0, int(math.Round(share*12))))
			prefix += strings.Repeat("━", filled) + strings.Repeat("·", 12-filled) + "  "
		}
		name := safe(row.Name)
		if row.Dir {
			name += "/"
		}
		if row.Symlink {
			name += " ↗"
		}
		if row.Errors > 0 {
			name += " !"
		}
		line := ansi.Truncate(prefix+name, m.width-1, "…")
		if i == m.cursor {
			line = selected.Width(m.width - 1).Render(line)
		} else if row.Dir {
			line = accent.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, muted.Render(strings.Repeat("─", m.width-1)))
	detail := " Empty directory"
	if len(m.rows) > 0 {
		row := m.rows[m.cursor]
		detail = fmt.Sprintf(" %s allocated  ·  %s apparent  ·  %s files", Bytes(row.Allocated), Bytes(row.Apparent), number(row.Files))
		if row.Error != "" {
			detail = warning.Render(" " + safe(row.Error))
		} else if row.Symlink {
			detail += "  ·  symlink, not followed"
		}
	} else if n.Error != "" {
		detail = warning.Render(" " + safe(n.Error))
	} else if m.query != "" {
		detail = " No matching entries; esc clears the filter"
	}
	lines = append(lines, detail)
	position := 0
	if len(m.rows) > 0 {
		position = m.cursor + 1
	}
	lines = append(lines, muted.Render(fmt.Sprintf(" %d/%d entries  ·  folder totals include its own metadata", position, len(m.rows))), " ↑↓ move  enter open  ← back  / filter  ? help  q quit")
	return fit(strings.Join(lines, "\n"), m.width, m.height)
}

// Never allow filenames or filesystem errors to inject terminal control codes.
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, s)
}

func fit(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > max(0, height) {
		lines = lines[:max(0, height)]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(0, width), "…")
	}
	return strings.Join(lines, "\n")
}

func Bytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	v, unit := float64(n), 0
	for v >= 1024 && unit < 6 {
		v /= 1024
		unit++
	}
	return fmt.Sprintf("%.1f %siB", v, []string{"", "K", "M", "G", "T", "P", "E"}[unit])
}

func number(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
