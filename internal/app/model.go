package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const configVersion = 1

type Host struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

type VM struct {
	Name   string
	UUID   string
	State  string
	Owner  string
	Shared bool
}

type Config struct {
	Version int    `json:"version"`
	Hosts   []Host `json:"hosts"`
	Theme   string `json:"theme,omitempty"`
}

type mode int

const (
	modeHosts mode = iota
	modeAddHost
	modeVMs
	modeTheme
	modeBusy
)

type Model struct {
	version string

	configPath string
	stateDir   string
	config     Config

	width       int
	height      int
	mode        mode
	priorMode   mode
	themeBack   mode
	status      string
	errText     string
	help        bool
	hostCursor  int
	vmCursor    int
	themeCursor int
	vms         []VM
	activeHost  Host

	addName   string
	addTarget string
	addField  int
}

type resultMsg struct {
	op     string
	output string
	vms    []VM
	err    error
}

var (
	defaultWidth  = 100
	defaultHeight = 30
)

type theme struct {
	Name     string
	Accent   lipgloss.Color
	Border   lipgloss.Color
	Text     lipgloss.Color
	Muted    lipgloss.Color
	OK       lipgloss.Color
	Error    lipgloss.Color
	Selected lipgloss.Color
}

var themes = []theme{
	{Name: "Classic", Accent: "62", Border: "238", Text: "230", Muted: "244", OK: "42", Error: "196", Selected: "62"},
	{Name: "Ocean", Accent: "39", Border: "31", Text: "255", Muted: "110", OK: "77", Error: "203", Selected: "31"},
	{Name: "Forest", Accent: "70", Border: "65", Text: "255", Muted: "108", OK: "114", Error: "203", Selected: "64"},
	{Name: "Amber", Accent: "214", Border: "130", Text: "255", Muted: "180", OK: "112", Error: "196", Selected: "172"},
	{Name: "Graphite", Accent: "250", Border: "239", Text: "255", Muted: "245", OK: "79", Error: "203", Selected: "241"},
	{Name: "Ruby", Accent: "197", Border: "125", Text: "255", Muted: "181", OK: "78", Error: "203", Selected: "125"},
	{Name: "Violet", Accent: "141", Border: "97", Text: "255", Muted: "146", OK: "78", Error: "203", Selected: "97"},
	{Name: "Cyan", Accent: "44", Border: "37", Text: "255", Muted: "116", OK: "83", Error: "203", Selected: "37"},
	{Name: "Mono", Accent: "255", Border: "246", Text: "255", Muted: "244", OK: "255", Error: "203", Selected: "238"},
	{Name: "Night", Accent: "111", Border: "60", Text: "255", Muted: "103", OK: "84", Error: "210", Selected: "60"},
}

type styles struct {
	title    lipgloss.Style
	pane     lipgloss.Style
	faint    lipgloss.Style
	ok       lipgloss.Style
	err      lipgloss.Style
	selected lipgloss.Style
	border   lipgloss.Style
}

func New(version string) (Model, error) {
	cfgPath, stateDir, err := paths()
	if err != nil {
		return Model{}, err
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return Model{}, err
	}
	if err := importLegacyHosts(&cfg); err != nil {
		return Model{}, err
	}
	if themeIndex(cfg.Theme) < 0 {
		cfg.Theme = themes[0].Name
	}
	sortHosts(cfg.Hosts)
	if err := saveConfig(cfgPath, cfg); err != nil {
		return Model{}, err
	}
	return Model{
		version:    version,
		configPath: cfgPath,
		stateDir:   stateDir,
		config:     cfg,
		mode:       modeHosts,
		status:     "Ready.",
	}, nil
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	case resultMsg:
		return m.updateResult(msg)
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	w, h := m.size()
	innerW := max(20, w-2)
	s := m.styles()
	b.WriteString(s.faint.Render("System libvirt over SSH. noVNC and port tunnels stay loopback-bound.") + "\n\n")

	switch m.mode {
	case modeAddHost:
		b.WriteString(m.viewAddHost(innerW))
	case modeVMs:
		b.WriteString(m.viewVMs(innerW))
	case modeTheme:
		b.WriteString(m.viewThemes(innerW))
	case modeBusy:
		b.WriteString(m.viewBusy(innerW))
	default:
		b.WriteString(m.viewHosts(innerW))
	}

	if m.errText != "" {
		b.WriteString("\n" + s.err.Render(m.errText) + "\n")
	} else if m.status != "" {
		b.WriteString("\n" + s.ok.Render(m.status) + "\n")
	}
	b.WriteString("\n" + s.faint.Render(m.helpText()) + "\n")
	return m.frame(w, h, b.String())
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" || msg.String() == "q" {
		return m, tea.Quit
	}
	if msg.String() == "?" {
		m.help = !m.help
		return m, nil
	}
	if msg.String() == "m" && m.mode != modeAddHost && m.mode != modeBusy && m.mode != modeTheme {
		m.themeBack = m.mode
		m.themeCursor = max(0, themeIndex(m.config.Theme))
		m.mode = modeTheme
		m.errText = ""
		m.status = "Browse themes. Enter selects, Esc cancels."
		return m, nil
	}

	switch m.mode {
	case modeAddHost:
		return m.updateAddHostKey(msg)
	case modeVMs:
		return m.updateVMKey(msg)
	case modeTheme:
		return m.updateThemeKey(msg)
	case modeBusy:
		return m, nil
	default:
		return m.updateHostKey(msg)
	}
}

func (m Model) updateHostKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.hostCursor > 0 {
			m.hostCursor--
		}
	case "down", "j":
		if m.hostCursor < len(m.config.Hosts)-1 {
			m.hostCursor++
		}
	case "a":
		m.mode = modeAddHost
		m.addName = ""
		m.addTarget = ""
		m.addField = 0
		m.errText = ""
		m.status = "Add a host name and SSH target."
	case "d":
		if len(m.config.Hosts) > 0 {
			removed := m.config.Hosts[m.hostCursor]
			m.config.Hosts = append(m.config.Hosts[:m.hostCursor], m.config.Hosts[m.hostCursor+1:]...)
			if m.hostCursor >= len(m.config.Hosts) && m.hostCursor > 0 {
				m.hostCursor--
			}
			if err := saveConfig(m.configPath, m.config); err != nil {
				m.errText = err.Error()
			} else {
				m.status = "Removed host " + removed.Name + "."
				m.errText = ""
			}
		}
	case "t":
		if h, ok := m.selectedHost(); ok {
			return m.busy(modeHosts, "Checking "+h.Name+"...", "check", func() resultMsg {
				out, err := checkHost(h)
				return resultMsg{op: "check", output: out, err: err}
			})
		}
	case "s":
		if h, ok := m.selectedHost(); ok {
			return m.busy(modeHosts, "Running setup on "+h.Name+"...", "setup", func() resultMsg {
				out, err := setupHost(h)
				return resultMsg{op: "setup", output: out, err: err}
			})
		}
	case "enter", "r":
		if h, ok := m.selectedHost(); ok {
			return m.loadVMs(h)
		}
	}
	return m, nil
}

func (m Model) updateAddHostKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeHosts
		m.status = "Cancelled add host."
		m.errText = ""
	case "tab", "shift+tab":
		m.addField = 1 - m.addField
	case "enter":
		if m.addField == 0 {
			m.addField = 1
			return m, nil
		}
		name := strings.TrimSpace(m.addName)
		target := strings.TrimSpace(m.addTarget)
		if !validName(name) {
			m.errText = "Host name must use letters, numbers, dot, dash, or underscore."
			return m, nil
		}
		if target == "" || strings.ContainsAny(target, "\t\r\n ") {
			m.errText = "SSH target must look like user@host or host."
			return m, nil
		}
		for _, h := range m.config.Hosts {
			if h.Name == name {
				m.errText = "Host already exists: " + name
				return m, nil
			}
		}
		m.config.Hosts = append(m.config.Hosts, Host{Name: name, Target: target})
		sortHosts(m.config.Hosts)
		if err := saveConfig(m.configPath, m.config); err != nil {
			m.errText = err.Error()
			return m, nil
		}
		m.mode = modeHosts
		m.status = "Added host " + name + ". Press t to test SSH or s to run setup."
		m.errText = ""
		m.hostCursor = indexHost(m.config.Hosts, name)
	case "backspace", "ctrl+h":
		if m.addField == 0 {
			m.addName = trimLastRune(m.addName)
		} else {
			m.addTarget = trimLastRune(m.addTarget)
		}
	default:
		if msg.Type == tea.KeyRunes {
			text := msg.String()
			if m.addField == 0 {
				m.addName += text
			} else {
				m.addTarget += text
			}
		}
	}
	return m, nil
}

func (m Model) updateVMKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "b", "esc":
		m.mode = modeHosts
		m.status = "Back to hosts."
		m.errText = ""
	case "up", "k":
		if m.vmCursor > 0 {
			m.vmCursor--
		}
	case "down", "j":
		if m.vmCursor < len(m.vms)-1 {
			m.vmCursor++
		}
	case "r":
		return m.loadVMs(m.activeHost)
	case "s":
		return m.busy(modeVMs, "Running setup on "+m.activeHost.Name+"...", "setup", func() resultMsg {
			out, err := setupHost(m.activeHost)
			return resultMsg{op: "setup", output: out, err: err}
		})
	case "p":
		if vm, ok := m.selectedVM(); ok {
			action := "start"
			if strings.Contains(strings.ToLower(vm.State), "running") {
				action = "shutdown"
			}
			return m.busy(modeVMs, action+" "+vm.Name+"...", action, func() resultMsg {
				out, err := lifecycle(m.activeHost, vm.Name, action)
				return resultMsg{op: action, output: out, err: err}
			})
		}
	case "f":
		if vm, ok := m.selectedVM(); ok {
			return m.busy(modeVMs, "Force off "+vm.Name+"...", "destroy", func() resultMsg {
				out, err := lifecycle(m.activeHost, vm.Name, "destroy")
				return resultMsg{op: "destroy", output: out, err: err}
			})
		}
	case "o":
		if vm, ok := m.selectedVM(); ok {
			return m.busy(modeVMs, "Opening console for "+vm.Name+"...", "console", func() resultMsg {
				out, err := openConsole(m.activeHost, vm.Name, m.stateDir)
				return resultMsg{op: "console", output: out, err: err}
			})
		}
	case "x":
		if vm, ok := m.selectedVM(); ok {
			return m.busy(modeVMs, "Stopping console for "+vm.Name+"...", "console-down", func() resultMsg {
				out, err := closeConsole(m.activeHost, vm.Name, m.stateDir)
				return resultMsg{op: "console-down", output: out, err: err}
			})
		}
	case "a":
		if vm, ok := m.selectedVM(); ok {
			return m.busy(modeVMs, "Adopting "+vm.Name+"...", "adopt", func() resultMsg {
				out, err := setOwnership(m.activeHost, vm, false, false)
				return resultMsg{op: "adopt", output: out, err: err}
			})
		}
	case "h":
		if vm, ok := m.selectedVM(); ok {
			return m.busy(modeVMs, "Toggling shared flag for "+vm.Name+"...", "share", func() resultMsg {
				out, err := setOwnership(m.activeHost, vm, !vm.Shared, true)
				return resultMsg{op: "share", output: out, err: err}
			})
		}
	}
	return m, nil
}

func (m Model) updateThemeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "b":
		m.mode = m.themeBack
		m.status = "Theme unchanged."
		m.errText = ""
	case "up", "k":
		if m.themeCursor > 0 {
			m.themeCursor--
		}
	case "down", "j":
		if m.themeCursor < len(themes)-1 {
			m.themeCursor++
		}
	case "enter":
		m.config.Theme = themes[m.themeCursor].Name
		if err := saveConfig(m.configPath, m.config); err != nil {
			m.errText = err.Error()
			return m, nil
		}
		m.mode = m.themeBack
		m.status = "Theme set to " + m.config.Theme + "."
		m.errText = ""
	}
	return m, nil
}

func (m Model) updateResult(msg resultMsg) (tea.Model, tea.Cmd) {
	m.mode = m.priorMode
	if msg.err != nil {
		m.errText = msg.err.Error()
		if msg.output != "" {
			m.errText += "\n" + strings.TrimSpace(msg.output)
		}
		return m, nil
	}
	m.errText = ""
	switch msg.op {
	case "vms":
		m.vms = msg.vms
		if m.vmCursor >= len(m.vms) {
			m.vmCursor = max(0, len(m.vms)-1)
		}
		m.status = fmt.Sprintf("Loaded %d VMs from %s.", len(m.vms), m.activeHost.Name)
	case "start", "shutdown", "destroy", "adopt", "share":
		m.status = strings.TrimSpace(msg.output)
		if m.status == "" {
			m.status = msg.op + " complete."
		}
		return m.loadVMs(m.activeHost)
	default:
		m.status = strings.TrimSpace(msg.output)
		if m.status == "" {
			m.status = msg.op + " complete."
		}
	}
	return m, nil
}

func (m Model) busy(back mode, status, op string, fn func() resultMsg) (tea.Model, tea.Cmd) {
	m.priorMode = back
	m.mode = modeBusy
	m.status = status
	m.errText = ""
	return m, func() tea.Msg { return fn() }
}

func (m Model) loadVMs(h Host) (tea.Model, tea.Cmd) {
	m.activeHost = h
	m.priorMode = modeVMs
	m.mode = modeBusy
	m.status = "Loading VMs from " + h.Name + "..."
	m.errText = ""
	return m, func() tea.Msg {
		vms, out, err := listVMs(h)
		return resultMsg{op: "vms", output: out, vms: vms, err: err}
	}
}

func (m Model) selectedHost() (Host, bool) {
	if len(m.config.Hosts) == 0 || m.hostCursor < 0 || m.hostCursor >= len(m.config.Hosts) {
		return Host{}, false
	}
	return m.config.Hosts[m.hostCursor], true
}

func (m Model) selectedVM() (VM, bool) {
	if len(m.vms) == 0 || m.vmCursor < 0 || m.vmCursor >= len(m.vms) {
		return VM{}, false
	}
	return m.vms[m.vmCursor], true
}

func (m Model) viewHosts(width int) string {
	var b strings.Builder
	b.WriteString(m.styles().pane.Width(max(40, width-4)).Render(m.hostRows(width - 8)))
	return b.String()
}

func (m Model) hostRows(width int) string {
	var b strings.Builder
	s := m.styles()
	b.WriteString("Hosts\n\n")
	b.WriteString(button("Theme: "+m.currentTheme().Name, "m", width) + "\n\n")
	if len(m.config.Hosts) == 0 {
		b.WriteString("No hosts configured.\n\nPress a to add the first host.")
		return b.String()
	}
	for i, h := range m.config.Hosts {
		cursor := " "
		if i == m.hostCursor {
			cursor = ">"
		}
		b.WriteString(cursor + " " + cell(h.Name, 18) + " " + s.faint.Render(clipText(h.Target, max(10, width-22))) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) viewAddHost(width int) string {
	nameCursor := " "
	targetCursor := " "
	if m.addField == 0 {
		nameCursor = ">"
	} else {
		targetCursor = ">"
	}
	body := fmt.Sprintf("Add Host\n\n%s Name:   %s\n%s Target: %s\n\nEnter moves/saves. Esc cancels.",
		nameCursor, m.addName, targetCursor, m.addTarget)
	return m.styles().pane.Width(max(40, width-4)).Render(body)
}

func (m Model) viewVMs(width int) string {
	var b strings.Builder
	s := m.styles()
	bodyW := max(50, width-8)
	nameW := max(24, bodyW-44)
	b.WriteString(fmt.Sprintf("Host: %s  %s\n\n", m.activeHost.Name, s.faint.Render(m.activeHost.Target)))
	if len(m.vms) == 0 {
		b.WriteString("No VMs found under qemu:///system.")
		return s.pane.Width(max(50, width-4)).Render(b.String())
	}
	b.WriteString("  " + cell("VM", nameW) + " " + cell("State", 12) + " " + cell("Owner", 14) + " " + cell("Visibility", 10) + "\n")
	b.WriteString("  " + strings.Repeat("-", nameW) + " " + strings.Repeat("-", 12) + " " + strings.Repeat("-", 14) + " " + strings.Repeat("-", 10) + "\n")
	for i, vm := range m.vms {
		cursor := " "
		if i == m.vmCursor {
			cursor = ">"
		}
		shared := "private"
		if vm.Shared {
			shared = "shared"
		}
		row := cursor + " " + cell(vm.Name, nameW) + " " + cell(vm.State, 12) + " " + cell(ownerLabel(vm.Owner), 14) + " " + cell(shared, 10)
		if i == m.vmCursor {
			row = s.selected.Render(row)
		}
		b.WriteString(row + "\n")
	}
	return s.pane.Width(max(50, width-4)).Render(strings.TrimRight(b.String(), "\n"))
}

func (m Model) viewThemes(width int) string {
	var b strings.Builder
	s := m.styles()
	b.WriteString("Themes\n\n")
	for i, theme := range themes {
		cursor := " "
		if i == m.themeCursor {
			cursor = ">"
		}
		line := fmt.Sprintf("%s %s", cursor, cell(theme.Name, 14))
		swatch := lipgloss.NewStyle().Foreground(theme.Accent).Render("██")
		line += " " + swatch + " " + lipgloss.NewStyle().Foreground(theme.Muted).Render("VMRelay host manager")
		if i == m.themeCursor {
			line = s.selected.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\nEnter selects. Esc returns without changing the saved theme.")
	return s.pane.Width(max(50, width-4)).Render(strings.TrimRight(b.String(), "\n"))
}

func (m Model) viewBusy(width int) string {
	return m.styles().pane.Width(max(40, width-4)).Render("Working\n\n" + m.status)
}

func (m Model) helpText() string {
	if !m.help {
		switch m.mode {
		case modeVMs:
			return "?: help  m: themes  b: hosts  r: refresh  p: start/shutdown  f: force off  o: open console  x: stop console"
		case modeAddHost:
			return "tab: switch field  enter: next/save  esc: cancel  q: quit"
		case modeTheme:
			return "up/down: browse themes  enter: select  esc/b: back  q: quit"
		default:
			return "?: help  m: themes  a: add host  enter/r: view VMs  t: test  s: setup  d: remove  q: quit"
		}
	}
	return "Hosts: a add, m themes, enter view, t test SSH/libvirt, s install/check host packages, d remove. VMs: p start/shutdown, f force off, o open noVNC console, x stop console, a adopt, h share/private, r refresh."
}

func ownerLabel(owner string) string {
	if owner == "" {
		return "unmanaged"
	}
	return owner
}

func (m Model) size() (int, int) {
	w, h := m.width, m.height
	if w <= 0 {
		w = defaultWidth
	}
	if h <= 0 {
		h = defaultHeight
	}
	if w < 30 {
		w = 30
	}
	if h < 10 {
		h = 10
	}
	return w, h
}

func (m Model) currentTheme() theme {
	name := m.config.Theme
	if m.mode == modeTheme && m.themeCursor >= 0 && m.themeCursor < len(themes) {
		name = themes[m.themeCursor].Name
	}
	idx := themeIndex(name)
	if idx < 0 {
		idx = 0
	}
	return themes[idx]
}

func (m Model) styles() styles {
	t := m.currentTheme()
	return styles{
		title:    lipgloss.NewStyle().Bold(true).Foreground(t.Text).Background(t.Accent),
		pane:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Border).Padding(0, 1),
		faint:    lipgloss.NewStyle().Foreground(t.Muted),
		ok:       lipgloss.NewStyle().Foreground(t.OK),
		err:      lipgloss.NewStyle().Foreground(t.Error),
		selected: lipgloss.NewStyle().Foreground(t.Text).Background(t.Selected),
		border:   lipgloss.NewStyle().Foreground(t.Border),
	}
}

func themeIndex(name string) int {
	for i, theme := range themes {
		if strings.EqualFold(theme.Name, name) {
			return i
		}
	}
	return -1
}

func (m Model) frame(width, height int, body string) string {
	s := m.styles()
	title := " VMRelay " + m.version + " "
	innerW := max(1, width-2)
	innerH := max(1, height-2)
	top := titledBorderTop(innerW, title, s)
	bottom := s.border.Render("╰" + strings.Repeat("─", innerW) + "╯")

	body = lipgloss.NewStyle().MaxWidth(innerW).Render(body)
	lines := strings.Split(body, "\n")
	if len(lines) > innerH {
		lines = lines[:innerH]
	}

	var b strings.Builder
	b.WriteString(top + "\n")
	for i := 0; i < innerH; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		b.WriteString(s.border.Render("│") + padLine(line, innerW) + s.border.Render("│"))
		if i < innerH-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n" + bottom)
	return b.String()
}

func titledBorderTop(width int, title string, s styles) string {
	titleW := lipgloss.Width(title)
	if titleW >= width {
		clipped := clipText(title, max(1, width))
		return s.border.Render("╭") + s.title.Render(clipped) + s.border.Render("╮")
	}
	left := (width - titleW) / 2
	right := width - titleW - left
	return s.border.Render("╭"+strings.Repeat("─", left)) + s.title.Render(title) + s.border.Render(strings.Repeat("─", right)+"╮")
}

func padLine(line string, width int) string {
	lineW := lipgloss.Width(line)
	if lineW > width {
		line = clipText(line, width)
		lineW = lipgloss.Width(line)
	}
	if lineW < width {
		line += strings.Repeat(" ", width-lineW)
	}
	return line
}

func button(label, key string, width int) string {
	text := "[" + key + "] " + label
	return cell(text, min(max(18, lipgloss.Width(text)), max(18, width)))
}

func cell(text string, width int) string {
	if width <= 0 {
		return ""
	}
	text = clipText(text, width)
	return text + strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
}

func clipText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	limit := width - 3
	var b strings.Builder
	for _, r := range text {
		next := b.String() + string(r)
		if lipgloss.Width(next) > limit {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + "..."
}

func paths() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(home, ".local", "state")
	}
	configDir := filepath.Join(configHome, "vmrelay")
	stateDir := filepath.Join(stateHome, "vmrelay")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return "", "", err
	}
	return filepath.Join(configDir, "config.json"), stateDir, nil
}

func loadConfig(path string) (Config, error) {
	cfg := Config{Version: configVersion}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Version == 0 {
		cfg.Version = configVersion
	}
	return cfg, nil
}

func saveConfig(path string, cfg Config) error {
	cfg.Version = configVersion
	sortHosts(cfg.Hosts)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func importLegacyHosts(cfg *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	files, err := filepath.Glob(filepath.Join(configHome, "vmrelay", "hosts.d", "*.conf"))
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, h := range cfg.Hosts {
		seen[h.Name] = true
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".conf")
		if seen[name] {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		target := parseShellValue(string(data), "SSH_TARGET")
		if target == "" {
			continue
		}
		cfg.Hosts = append(cfg.Hosts, Host{Name: name, Target: target})
	}
	return nil
}

func parseShellValue(content, key string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, key+"=") {
			continue
		}
		value := strings.TrimPrefix(line, key+"=")
		value = strings.Trim(value, `"`)
		value = strings.ReplaceAll(value, `\"`, `"`)
		value = strings.ReplaceAll(value, `\\`, `\`)
		return value
	}
	return ""
}

func sortHosts(hosts []Host) {
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Name < hosts[j].Name })
}

func indexHost(hosts []Host, name string) int {
	for i, h := range hosts {
		if h.Name == name {
			return i
		}
	}
	return 0
}

func validName(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func trimLastRune(s string) string {
	if s == "" {
		return s
	}
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

func checkHost(h Host) (string, error) {
	script := `
set -u
printf 'Host: %s\n' "$(hostname -f 2>/dev/null || hostname)"
printf 'User: %s\n' "$(whoami)"
printf 'OS: %s\n' "$(if [ -r /etc/os-release ]; then . /etc/os-release; printf "%s" "${PRETTY_NAME:-unknown}"; else uname -s; fi)"
command -v virsh >/dev/null && printf 'virsh: yes\n' || printf 'virsh: missing\n'
virsh -c qemu:///system uri >/dev/null 2>&1 && printf 'libvirt system: yes\n' || printf 'libvirt system: unavailable\n'
[ -e /dev/kvm ] && printf 'KVM: yes\n' || printf 'KVM: missing\n'
command -v virt-install >/dev/null && printf 'virt-install: yes\n' || printf 'virt-install: missing\n'
command -v qemu-img >/dev/null && printf 'qemu-img: yes\n' || printf 'qemu-img: missing\n'
command -v websockify >/dev/null && printf 'websockify: yes\n' || printf 'websockify: missing\n'
[ -d /usr/share/novnc ] && printf 'noVNC: yes\n' || printf 'noVNC: missing\n'
[ -r /var/lib/vmrelay/ownership.tsv ] && printf 'VMRelay ownership: yes\n' || printf 'VMRelay ownership: not initialized\n'
`
	return ssh(h.Target, script, 20*time.Second)
}

func setupHost(h Host) (string, error) {
	script := `
set -euo pipefail
if command -v apt-get >/dev/null 2>&1; then
  sudo -n apt-get update
  sudo -n apt-get install -y qemu-kvm libvirt-daemon-system libvirt-clients virtinst qemu-utils novnc websockify
else
  echo "Automatic setup currently supports apt-based hosts. Install KVM/libvirt/virt-install/qemu-utils/novnc/websockify manually."
fi
group=libvirt
if ! getent group "$group" >/dev/null 2>&1; then group=libvirt-qemu; fi
sudo -n install -d -m 2775 -o root -g "$group" /var/lib/vmrelay
sudo -n touch /var/lib/vmrelay/ownership.tsv
sudo -n chown root:"$group" /var/lib/vmrelay/ownership.tsv
sudo -n chmod 0664 /var/lib/vmrelay/ownership.tsv
echo "Host setup complete. VMRelay ownership state is /var/lib/vmrelay/ownership.tsv."
`
	return ssh(h.Target, script, 15*time.Minute)
}

func listVMs(h Host) ([]VM, string, error) {
	script := `
set -euo pipefail
policy=/var/lib/vmrelay/ownership.tsv
if [ ! -r "$policy" ]; then policy=/dev/null; fi
virsh -c qemu:///system list --all --name | sed '/^$/d' | while IFS= read -r name; do
  uuid="$(virsh -c qemu:///system domuuid "$name" 2>/dev/null || true)"
  state="$(virsh -c qemu:///system domstate "$name" 2>/dev/null | tr '\n' ' ' | sed 's/[[:space:]]*$//' || true)"
  owner=""
  shared="0"
  if [ -n "$uuid" ]; then
    line="$(awk -F '\t' -v id="$uuid" '$1 == id { print; exit }' "$policy" 2>/dev/null || true)"
    if [ -n "$line" ]; then
      owner="$(printf '%s\n' "$line" | awk -F '\t' '{print $2}')"
      shared="$(printf '%s\n' "$line" | awk -F '\t' '{print $3}')"
    fi
  fi
  printf 'VMRELAY_VM\t%s\t%s\t%s\t%s\t%s\n' "$name" "$uuid" "$state" "$owner" "$shared"
done
`
	out, err := ssh(h.Target, script, 45*time.Second)
	if err != nil {
		return nil, out, err
	}
	vms := parseVMListOutput(out)
	sort.Slice(vms, func(i, j int) bool { return vms[i].Name < vms[j].Name })
	return vms, out, nil
}

func parseVMListOutput(out string) []VM {
	var vms []VM
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) == 0 || parts[0] != "VMRELAY_VM" {
			continue
		}
		parts = parts[1:]
		for len(parts) < 5 {
			parts = append(parts, "")
		}
		vms = append(vms, VM{
			Name:   parts[0],
			UUID:   parts[1],
			State:  parts[2],
			Owner:  parts[3],
			Shared: parts[4] == "1" || strings.EqualFold(parts[4], "true"),
		})
	}
	return vms
}

func lifecycle(h Host, vmName, action string) (string, error) {
	if action != "start" && action != "shutdown" && action != "destroy" {
		return "", fmt.Errorf("unsupported action: %s", action)
	}
	script := fmt.Sprintf("set -euo pipefail\nvirsh -c qemu:///system %s %s\n", shellWord(action), shellQuote(vmName))
	return ssh(h.Target, script, 45*time.Second)
}

func setOwnership(h Host, vm VM, shared bool, keepOwner bool) (string, error) {
	if vm.UUID == "" {
		return "", fmt.Errorf("VM %s has no UUID", vm.Name)
	}
	sharedValue := "0"
	if shared {
		sharedValue = "1"
	}
	owner := vm.Owner
	if owner == "" || owner == "unmanaged" || !keepOwner {
		owner = "$(whoami)"
	} else {
		owner = shellQuote(owner)
	}
	script := fmt.Sprintf(`
set -euo pipefail
policy=/var/lib/vmrelay/ownership.tsv
[ -e "$policy" ] || sudo -n touch "$policy"
tmp="$(mktemp)"
if [ -r "$policy" ]; then awk -F '\t' -v id=%s '$1 != id { print }' "$policy" >"$tmp"; fi
printf '%%s\t%%s\t%%s\t%%s\n' %s %s %s '' >>"$tmp"
if [ -w "$policy" ]; then
  cat "$tmp" >"$policy"
else
  sudo -n cp "$tmp" "$policy"
  sudo -n chmod 0664 "$policy"
fi
rm -f "$tmp"
echo "Ownership updated for %s."
`, shellQuote(vm.UUID), shellQuote(vm.UUID), owner, shellQuote(sharedValue), vm.Name)
	return ssh(h.Target, script, 30*time.Second)
}

func openConsole(h Host, vmName, stateDir string) (string, error) {
	localPort := stablePort("local:"+h.Name+":"+vmName, 4500, 1000)
	remotePort := stablePort("remote:"+h.Name+":"+vmName, 6080, 1000)
	if !portFree(localPort) {
		return "", fmt.Errorf("local port %d is already in use", localPort)
	}

	script := fmt.Sprintf(`
set -euo pipefail
vm=%s
remote_port=%d
display="$(virsh -c qemu:///system domdisplay "$vm" 2>/dev/null || true)"
case "$display" in
  vnc://*)
    target="${display#vnc://}"
    target="${target%%/*}"
    host="${target%%:*}"
    port="${target##*:}"
    if [ "$host" = "$port" ]; then host=127.0.0.1; fi
    case "$port" in
      ''|*[!0-9]*) echo "Unsupported VNC display: $display" >&2; exit 1 ;;
    esac
    if [ "$port" -lt 100 ]; then port=$((5900 + port)); fi
    ;;
  *)
    echo "VM has no VNC graphics console available. Shut it down and change graphics to VNC, then retry." >&2
    exit 1
    ;;
esac
[ -d /usr/share/novnc ] || { echo "noVNC is missing; run setup for this host." >&2; exit 1; }
command -v websockify >/dev/null 2>&1 || { echo "websockify is missing; run setup for this host." >&2; exit 1; }
pidfile="/tmp/vmrelay-novnc-${remote_port}.pid"
logfile="/tmp/vmrelay-novnc-${remote_port}.log"
if [ -s "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
  echo "noVNC already running on 127.0.0.1:${remote_port}"
else
  rm -f "$pidfile" "$logfile"
  nohup websockify --web=/usr/share/novnc "127.0.0.1:${remote_port}" "${host}:${port}" >"$logfile" 2>&1 </dev/null &
  echo $! >"$pidfile"
  sleep 1
  if ! kill -0 "$(cat "$pidfile")" 2>/dev/null; then
    cat "$logfile" >&2 || true
    exit 1
  fi
  echo "Started noVNC on 127.0.0.1:${remote_port}"
fi
`, shellQuote(vmName), remotePort)
	out, err := ssh(h.Target, script, 30*time.Second)
	if err != nil {
		return out, err
	}

	ctl := consoleControlPath(stateDir, h.Name, vmName)
	_ = os.Remove(ctl)
	args := []string{
		"-f", "-N", "-M", "-S", ctl,
		"-o", "BatchMode=yes",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ControlPersist=yes",
		"-L", fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d", localPort, remotePort),
		h.Target,
	}
	if tunnelOut, err := runCommand(20*time.Second, "ssh", args...); err != nil {
		return out + tunnelOut, fmt.Errorf("failed to start SSH console tunnel: %w", err)
	}
	state := consoleState{Host: h.Name, Target: h.Target, VM: vmName, LocalPort: localPort, RemotePort: remotePort}
	if err := writeConsoleState(stateDir, state); err != nil {
		return out, err
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/vnc.html?autoconnect=1&resize=scale", localPort)
	opened := openBrowser(url)
	if opened {
		out += "\nConsole URL: " + url + "\nBrowser: requested local console URL"
	} else {
		out += "\nConsole URL: " + url
	}
	return strings.TrimSpace(out), nil
}

func closeConsole(h Host, vmName, stateDir string) (string, error) {
	ctl := consoleControlPath(stateDir, h.Name, vmName)
	var lines []string
	if _, err := os.Stat(ctl); err == nil {
		out, err := runCommand(10*time.Second, "ssh", "-S", ctl, "-O", "exit", h.Target)
		lines = append(lines, strings.TrimSpace(out))
		if err != nil {
			lines = append(lines, "SSH tunnel stop reported: "+err.Error())
		}
	}
	state, _ := readConsoleState(stateDir, h.Name, vmName)
	remotePort := state.RemotePort
	if remotePort == 0 {
		remotePort = stablePort("remote:"+h.Name+":"+vmName, 6080, 1000)
	}
	script := fmt.Sprintf(`
pidfile="/tmp/vmrelay-novnc-%d.pid"
if [ -s "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
  kill "$(cat "$pidfile")" 2>/dev/null || true
  rm -f "$pidfile"
  echo "Stopped remote noVNC on port %d."
else
  echo "No remote noVNC process recorded for port %d."
fi
`, remotePort, remotePort, remotePort)
	out, err := ssh(h.Target, script, 15*time.Second)
	lines = append(lines, strings.TrimSpace(out))
	_ = os.Remove(ctl)
	_ = os.Remove(consoleStatePath(stateDir, h.Name, vmName))
	return strings.TrimSpace(strings.Join(lines, "\n")), err
}

type consoleState struct {
	Host       string `json:"host"`
	Target     string `json:"target"`
	VM         string `json:"vm"`
	LocalPort  int    `json:"local_port"`
	RemotePort int    `json:"remote_port"`
}

func writeConsoleState(stateDir string, state consoleState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(consoleStatePath(stateDir, state.Host, state.VM), append(data, '\n'), 0o600)
}

func readConsoleState(stateDir, host, vm string) (consoleState, error) {
	var state consoleState
	data, err := os.ReadFile(consoleStatePath(stateDir, host, vm))
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(data, &state)
	return state, err
}

func consoleControlPath(stateDir, host, vm string) string {
	return filepath.Join(stateDir, "console-"+hash(host+"-"+vm)+".ctl")
}

func consoleStatePath(stateDir, host, vm string) string {
	return filepath.Join(stateDir, "console-"+hash(host+"-"+vm)+".json")
}

func stablePort(key string, base, span int) int {
	sum := sha1.Sum([]byte(key))
	n := int(sum[0])<<8 + int(sum[1])
	return base + n%span
}

func hash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

func portFree(port int) bool {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func ssh(target, script string, timeout time.Duration) (string, error) {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=8",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		target,
		"bash", "-s",
	}
	return runCommandInput(timeout, script, "ssh", args...)
}

func runCommand(timeout time.Duration, name string, args ...string) (string, error) {
	return runCommandInput(timeout, "", name, args...)
}

func runCommandInput(timeout time.Duration, input, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("%s timed out", name)
	}
	return string(out), err
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func shellWord(s string) string {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '-' {
			continue
		}
		return shellQuote(s)
	}
	return s
}

func openBrowser(url string) bool {
	if os.Getenv("VMRELAY_OPEN_BROWSER") == "0" {
		return false
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		for _, opener := range []string{"xdg-open", "gio", "sensible-browser", "wslview"} {
			if path, err := exec.LookPath(opener); err == nil {
				if opener == "gio" {
					cmd = exec.Command(path, "open", url)
				} else {
					cmd = exec.Command(path, url)
				}
				break
			}
		}
	case "windows":
		cmd = exec.Command("cmd", "/C", "start", "", url)
	}
	if cmd == nil {
		return false
	}
	return cmd.Start() == nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
