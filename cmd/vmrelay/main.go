package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brontoguana/vmrelay/internal/app"
)

var version = "0.2.37"

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

	finalModel, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "vmrelay: %v\n", err)
		os.Exit(1)
	}
	if m, ok := finalModel.(app.Model); ok && m.UpdateRequested() {
		if err := runInstallerAndRestart(); err != nil {
			fmt.Fprintf(os.Stderr, "vmrelay: update failed: %v\n", err)
			os.Exit(1)
		}
	}
}

func runInstallerAndRestart() error {
	terminal := installerTerminal()
	defer terminal.close()

	fmt.Fprintln(terminal.err(), "VMRelay is updating. If prompted, enter your local sudo password.")
	repairInstallerTerminal()
	cmd := exec.Command("bash", "-lc", app.InstallCommand())
	cmd.Env = os.Environ()
	cmd.Stdin = terminal.in()
	cmd.Stdout = terminal.out()
	cmd.Stderr = terminal.err()
	if err := cmd.Run(); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	fmt.Fprintln(terminal.err(), "VMRelay updated. Restarting...")
	argv := append([]string{exe}, os.Args[1:]...)
	return syscall.Exec(exe, argv, os.Environ())
}

type installerIO struct {
	tty *os.File
}

func installerTerminal() installerIO {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return installerIO{}
	}
	return installerIO{tty: tty}
}

func (io installerIO) in() *os.File {
	if io.tty != nil {
		return io.tty
	}
	return os.Stdin
}

func (io installerIO) out() *os.File {
	if io.tty != nil {
		return io.tty
	}
	return os.Stdout
}

func (io installerIO) err() *os.File {
	if io.tty != nil {
		return io.tty
	}
	return os.Stderr
}

func (io installerIO) close() {
	if io.tty != nil {
		_ = io.tty.Close()
	}
}

func repairInstallerTerminal() {
	cmd := exec.Command("sh", "-c", "stty sane < /dev/tty > /dev/tty 2>&1 || true")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}
