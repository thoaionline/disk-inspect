package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"

	"disk-inspect/internal/scan"
	"disk-inspect/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "disk-inspect:", err)
		os.Exit(1)
	}
}

func run(args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("disk-inspect", flag.ContinueOnError)
	flags.SetOutput(errOut)
	asJSON := flags.Bool("json", false, "print a JSON summary instead of opening the TUI")
	apparent := flags.Bool("apparent", false, "start with apparent sizes instead of allocated disk usage")
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintln(errOut, "Usage: disk-inspect [flags] [directory]\n\nExplore disk usage interactively. The directory defaults to the current one.\nFlags must precede the directory.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *showVersion {
		_, err := fmt.Fprintln(out, "disk-inspect", version)
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("expected at most one directory")
	}
	path := "."
	if flags.NArg() == 1 {
		path = flags.Arg(0)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if *asJSON {
		root, err := scan.Scan(ctx, path, nil)
		if err != nil {
			return err
		}
		sort.Slice(root.Children, func(i, j int) bool {
			a, b := root.Children[i], root.Children[j]
			if a.Size(*apparent) == b.Size(*apparent) {
				return a.Name < b.Name
			}
			return a.Size(*apparent) > b.Size(*apparent)
		})
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			*scan.Node
			AllocatedSupported bool         `json:"allocated_supported"`
			Entries            []*scan.Node `json:"entries"`
		}{root, scan.HasAllocated, root.Children})
	}
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("interactive mode needs a terminal; use --json for redirected output")
	}
	m := tui.New(ctx, path, *apparent)
	defer m.Close()
	final, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx)).Run()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return err
	}
	return final.(*tui.Model).Err()
}
