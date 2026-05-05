package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brontoguana/vmrelay/internal/app"
)

var version = "0.2.2"

func main() {
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--version", "version":
			fmt.Printf("vmrelay %s\n", version)
			return
		case "--help", "-h", "help":
			fmt.Print("Usage:\n  vmrelay\n  vmrelay --version\n\nVMRelay opens a terminal UI for remote libvirt VM management.\n")
			return
		default:
			fmt.Fprintf(os.Stderr, "vmrelay: unsupported argument %q\n\nRun vmrelay with no arguments to open the TUI.\n", arg)
			os.Exit(2)
		}
	}

	model, err := app.New(version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vmrelay: %v\n", err)
		os.Exit(1)
	}

	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "vmrelay: %v\n", err)
		os.Exit(1)
	}
}
