package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const configVersion = 2

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
	Version  int           `json:"version"`
	Hosts    []Host        `json:"hosts"`
	Theme    string        `json:"theme,omitempty"`
	Mappings []PortMapping `json:"mappings,omitempty"`
}

type PortMapping struct {
	ID         string `json:"id"`
	Host       string `json:"host"`
	Name       string `json:"name"`
	LocalPort  int    `json:"local_port"`
	RemoteHost string `json:"remote_host"`
	RemotePort int    `json:"remote_port"`
}

type mode int

const (
	modeHosts mode = iota
	modeAddHost
	modeVMs
	modeAddMapping
	modeTheme
	modeUpdate
	modeBusy
)

const (
	hostTabVMs = iota
	hostTabConfig
	hostTabMappings
	hostTabCount
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
	hostTab     int
	mapCursor   int
	themeCursor int
	vms         []VM
	activeHost  Host
	updateInfo  updateInfo

	addName   string
	addTarget string
	addField  int

	addMapName       string
	addMapLocalPort  string
	addMapRemoteHost string
	addMapRemotePort string
	addMapField      int
}

type resultMsg struct {
	op     string
	output string
	vms    []VM
	err    error
}

type updateInfo struct {
	Latest string
	URL    string
}

type updateCheckMsg struct {
	info      updateInfo
	available bool
	err       error
}

type updateFinishedMsg struct {
	err error
}

var (
	defaultWidth  = 100
	defaultHeight = 30
)

const latestReleaseAPI = "https://api.github.com/repos/brontoguana/vmrelay/releases/latest"
const installCommand = "curl -fsSL https://raw.githubusercontent.com/brontoguana/vmrelay/main/install.sh | bash"

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
	startMode := modeHosts
	status := "Ready."
	if os.Getenv("VMRELAY_SKIP_UPDATE_CHECK") != "1" {
		startMode = modeBusy
		status = "Checking for updates..."
	}
	return Model{
		version:    version,
		configPath: cfgPath,
		stateDir:   stateDir,
		config:     cfg,
		mode:       startMode,
		priorMode:  modeHosts,
		status:     status,
	}, nil
}

func (m Model) Init() tea.Cmd {
	if os.Getenv("VMRELAY_SKIP_UPDATE_CHECK") == "1" {
		return nil
	}
	return checkForUpdateCmd(m.version)
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
	case updateCheckMsg:
		return m.updateCheck(msg)
	case updateFinishedMsg:
		return m.updateFinished(msg)
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	w, h := m.size()
	innerW := max(20, w-2)
	innerH := max(1, h-2)
	s := m.styles()
	footer := s.faint.Render(m.helpText())
	status := m.statusLine()
	contentH := max(1, innerH-2)

	switch m.mode {
	case modeAddHost:
		b.WriteString(m.viewAddHost(innerW, contentH))
	case modeVMs:
		b.WriteString(m.viewHostDetail(innerW, contentH))
	case modeAddMapping:
		b.WriteString(m.viewAddMapping(innerW, contentH))
	case modeTheme:
		b.WriteString(m.viewThemes(innerW, contentH))
	case modeUpdate:
		b.WriteString(m.viewUpdatePrompt(innerW, contentH))
	case modeBusy:
		b.WriteString(m.viewBusy(innerW, contentH))
	default:
		b.WriteString(m.viewHosts(innerW, contentH))
	}

	lines := strings.Split(b.String(), "\n")
	if len(lines) > contentH {
		lines = lines[:contentH]
	}
	for len(lines) < contentH {
		lines = append(lines, "")
	}
	lines = append(lines, padLine(status, innerW), padLine(footer, innerW))
	return m.frame(w, h, strings.Join(lines, "\n"))
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" || msg.String() == "q" {
		return m, tea.Quit
	}
	if msg.String() == "?" {
		m.help = !m.help
		return m, nil
	}
	if msg.String() == "m" && m.mode != modeAddHost && m.mode != modeAddMapping && m.mode != modeBusy && m.mode != modeTheme {
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
	case modeAddMapping:
		return m.updateAddMappingKey(msg)
	case modeTheme:
		return m.updateThemeKey(msg)
	case modeUpdate:
		return m.updateUpdateKey(msg)
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
			m.removeHostMappings(removed.Name)
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
			m.hostTab = hostTabVMs
			m.status = "Opening " + h.Name + " VM list..."
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

func (m Model) updateAddMappingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeVMs
		m.status = "Cancelled add mapping."
		m.errText = ""
	case "tab":
		m.addMapField = (m.addMapField + 1) % 4
	case "shift+tab":
		m.addMapField = (m.addMapField + 3) % 4
	case "enter":
		if m.addMapField < 3 {
			m.addMapField++
			return m, nil
		}
		mapping, err := m.pendingMapping()
		if err != nil {
			m.errText = err.Error()
			return m, nil
		}
		m.config.Mappings = append(m.config.Mappings, mapping)
		sortMappings(m.config.Mappings)
		if err := saveConfig(m.configPath, m.config); err != nil {
			m.errText = err.Error()
			return m, nil
		}
		m.mode = modeVMs
		m.hostTab = hostTabMappings
		m.mapCursor = indexMapping(m.hostMappings(), mapping.ID)
		m.status = "Added mapping " + mapping.Name + ". Press e to start it."
		m.errText = ""
	case "backspace", "ctrl+h":
		switch m.addMapField {
		case 0:
			m.addMapName = trimLastRune(m.addMapName)
		case 1:
			m.addMapLocalPort = trimLastRune(m.addMapLocalPort)
		case 2:
			m.addMapRemoteHost = trimLastRune(m.addMapRemoteHost)
		case 3:
			m.addMapRemotePort = trimLastRune(m.addMapRemotePort)
		}
	default:
		if msg.Type == tea.KeyRunes {
			text := msg.String()
			switch m.addMapField {
			case 0:
				m.addMapName += text
			case 1:
				m.addMapLocalPort += text
			case 2:
				m.addMapRemoteHost += text
			case 3:
				m.addMapRemotePort += text
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
	case "left", "[":
		if m.hostTab > 0 {
			m.hostTab--
		}
	case "right", "]":
		if m.hostTab < hostTabCount-1 {
			m.hostTab++
		}
	case "up", "k":
		if m.hostTab == hostTabMappings {
			if m.mapCursor > 0 {
				m.mapCursor--
			}
		} else if m.hostTab == hostTabVMs && m.vmCursor > 0 {
			m.vmCursor--
		}
	case "down", "j":
		if m.hostTab == hostTabMappings {
			if m.mapCursor < len(m.hostMappings())-1 {
				m.mapCursor++
			}
		} else if m.hostTab == hostTabVMs && m.vmCursor < len(m.vms)-1 {
			m.vmCursor++
		}
	case "n":
		if m.hostTab == hostTabMappings {
			m.mode = modeAddMapping
			m.addMapName = ""
			m.addMapLocalPort = ""
			m.addMapRemoteHost = "127.0.0.1"
			m.addMapRemotePort = ""
			m.addMapField = 0
			m.status = "Add a local SSH port mapping."
			m.errText = ""
		}
	case "r":
		if m.hostTab == hostTabVMs {
			return m.loadVMs(m.activeHost)
		}
		if m.hostTab == hostTabConfig {
			return m.busy(modeVMs, "Checking "+m.activeHost.Name+"...", "check", func() resultMsg {
				out, err := checkHost(m.activeHost)
				return resultMsg{op: "check", output: out, err: err}
			})
		}
	case "s":
		return m.busy(modeVMs, "Running setup on "+m.activeHost.Name+"...", "setup", func() resultMsg {
			out, err := setupHost(m.activeHost)
			return resultMsg{op: "setup", output: out, err: err}
		})
	case "d":
		if m.hostTab == hostTabMappings {
			mapping, ok := m.selectedMapping()
			if !ok {
				return m, nil
			}
			_, _ = stopPortMapping(m.activeHost, mapping, m.stateDir)
			m.removeMapping(mapping.ID)
			if err := saveConfig(m.configPath, m.config); err != nil {
				m.errText = err.Error()
				return m, nil
			}
			if m.mapCursor >= len(m.hostMappings()) {
				m.mapCursor = max(0, len(m.hostMappings())-1)
			}
			m.status = "Removed mapping " + mapping.Name + "."
			m.errText = ""
		}
	case "e":
		if m.hostTab == hostTabMappings {
			mapping, ok := m.selectedMapping()
			if !ok {
				return m, nil
			}
			if mappingActive(m.stateDir, m.activeHost.Name, mapping.ID) {
				return m.busy(modeVMs, "Stopping mapping "+mapping.Name+"...", "mapping-stop", func() resultMsg {
					out, err := stopPortMapping(m.activeHost, mapping, m.stateDir)
					return resultMsg{op: "mapping-stop", output: out, err: err}
				})
			}
			return m.busy(modeVMs, "Starting mapping "+mapping.Name+"...", "mapping-start", func() resultMsg {
				out, err := startPortMapping(m.activeHost, mapping, m.stateDir)
				return resultMsg{op: "mapping-start", output: out, err: err}
			})
		}
	case "p":
		if m.hostTab != hostTabVMs {
			return m, nil
		}
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
		if m.hostTab != hostTabVMs {
			return m, nil
		}
		if vm, ok := m.selectedVM(); ok {
			return m.busy(modeVMs, "Force off "+vm.Name+"...", "destroy", func() resultMsg {
				out, err := lifecycle(m.activeHost, vm.Name, "destroy")
				return resultMsg{op: "destroy", output: out, err: err}
			})
		}
	case "o":
		if m.hostTab != hostTabVMs {
			return m, nil
		}
		if vm, ok := m.selectedVM(); ok {
			return m.busy(modeVMs, "Opening console for "+vm.Name+"...", "console", func() resultMsg {
				out, err := openConsole(m.activeHost, vm.Name, m.stateDir)
				return resultMsg{op: "console", output: out, err: err}
			})
		}
	case "x":
		if m.hostTab != hostTabVMs {
			return m, nil
		}
		if vm, ok := m.selectedVM(); ok {
			return m.busy(modeVMs, "Stopping console for "+vm.Name+"...", "console-down", func() resultMsg {
				out, err := closeConsole(m.activeHost, vm.Name, m.stateDir)
				return resultMsg{op: "console-down", output: out, err: err}
			})
		}
	case "a":
		if m.hostTab != hostTabVMs {
			return m, nil
		}
		if vm, ok := m.selectedVM(); ok {
			return m.busy(modeVMs, "Adopting "+vm.Name+"...", "adopt", func() resultMsg {
				out, err := setOwnership(m.activeHost, vm, false, false)
				return resultMsg{op: "adopt", output: out, err: err}
			})
		}
	case "h":
		if m.hostTab != hostTabVMs {
			return m, nil
		}
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

func (m Model) updateUpdateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "y":
		m.mode = modeBusy
		m.priorMode = modeUpdate
		m.status = "Updating to " + m.updateInfo.Latest + "..."
		m.errText = ""
		cmd := exec.Command("bash", "-lc", installCommand)
		cmd.Env = os.Environ()
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return updateFinishedMsg{err: err}
		})
	case "n", "esc":
		m.mode = modeHosts
		m.status = "Skipped update to " + m.updateInfo.Latest + "."
		m.errText = ""
	}
	return m, nil
}

func (m Model) updateCheck(msg updateCheckMsg) (tea.Model, tea.Cmd) {
	if msg.available {
		m.mode = modeUpdate
		m.updateInfo = msg.info
		m.status = "Update available: " + msg.info.Latest + "."
		m.errText = ""
		return m, nil
	}
	if msg.err != nil {
		m.mode = modeHosts
		m.status = "Ready. Update check unavailable."
		return m, nil
	}
	m.mode = modeHosts
	m.status = "Ready."
	return m, nil
}

func (m Model) updateFinished(msg updateFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.mode = modeUpdate
		m.status = "Update failed."
		m.errText = msg.err.Error()
		return m, nil
	}
	m.status = "Update installed. Restarting VMRelay..."
	m.errText = ""
	return m, restartCmd()
}

func (m Model) updateResult(msg resultMsg) (tea.Model, tea.Cmd) {
	m.mode = m.priorMode
	if msg.err != nil {
		m.errText = failureText(msg, m)
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
	case "mapping-start", "mapping-stop":
		m.status = strings.TrimSpace(msg.output)
		if m.status == "" {
			m.status = msg.op + " complete."
		}
	default:
		m.status = strings.TrimSpace(msg.output)
		if m.status == "" {
			m.status = msg.op + " complete."
		}
	}
	return m, nil
}

func failureText(msg resultMsg, m Model) string {
	switch msg.op {
	case "vms":
		if m.activeHost.Name != "" {
			return "Failed to open " + m.activeHost.Name + ": " + msg.err.Error()
		}
		return "Failed to open host: " + msg.err.Error()
	case "check":
		return "Host check failed: " + msg.err.Error()
	case "setup":
		return "Host setup failed: " + msg.err.Error()
	case "console":
		return "Console open failed: " + msg.err.Error()
	case "console-down":
		return "Console stop failed: " + msg.err.Error()
	case "mapping-start":
		return "Mapping start failed: " + msg.err.Error()
	case "mapping-stop":
		return "Mapping stop failed: " + msg.err.Error()
	default:
		return msg.op + " failed: " + msg.err.Error()
	}
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

func (m Model) hostMappings() []PortMapping {
	var mappings []PortMapping
	for _, mapping := range m.config.Mappings {
		if mapping.Host == m.activeHost.Name {
			if mapping.RemoteHost == "" {
				mapping.RemoteHost = "127.0.0.1"
			}
			mappings = append(mappings, mapping)
		}
	}
	sortMappings(mappings)
	return mappings
}

func (m Model) selectedMapping() (PortMapping, bool) {
	mappings := m.hostMappings()
	if len(mappings) == 0 || m.mapCursor < 0 || m.mapCursor >= len(mappings) {
		return PortMapping{}, false
	}
	return mappings[m.mapCursor], true
}

func (m *Model) removeMapping(id string) {
	next := m.config.Mappings[:0]
	for _, mapping := range m.config.Mappings {
		if mapping.ID != id {
			next = append(next, mapping)
		}
	}
	m.config.Mappings = next
}

func (m *Model) removeHostMappings(host string) {
	next := m.config.Mappings[:0]
	for _, mapping := range m.config.Mappings {
		if mapping.Host != host {
			next = append(next, mapping)
		}
	}
	m.config.Mappings = next
}

func (m Model) pendingMapping() (PortMapping, error) {
	name := strings.TrimSpace(m.addMapName)
	if name == "" || strings.ContainsAny(name, "\r\n\t") {
		return PortMapping{}, fmt.Errorf("mapping name is required")
	}
	localPort, err := parsePort(m.addMapLocalPort)
	if err != nil {
		return PortMapping{}, fmt.Errorf("local port: %w", err)
	}
	remoteHost := strings.TrimSpace(m.addMapRemoteHost)
	if remoteHost == "" {
		remoteHost = "127.0.0.1"
	}
	if strings.ContainsAny(remoteHost, "\r\n\t ") {
		return PortMapping{}, fmt.Errorf("remote host must not contain spaces")
	}
	remotePort, err := parsePort(m.addMapRemotePort)
	if err != nil {
		return PortMapping{}, fmt.Errorf("remote port: %w", err)
	}
	id := hash(fmt.Sprintf("%s-%s-%d-%s-%d-%d", m.activeHost.Name, name, localPort, remoteHost, remotePort, time.Now().UnixNano()))
	return PortMapping{ID: id, Host: m.activeHost.Name, Name: name, LocalPort: localPort, RemoteHost: remoteHost, RemotePort: remotePort}, nil
}

func parsePort(text string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0, fmt.Errorf("must be a number")
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("must be between 1 and 65535")
	}
	return port, nil
}

func (m Model) statusLine() string {
	s := m.styles()
	if m.errText != "" {
		return s.err.Render(firstLine(m.errText))
	}
	if m.status != "" {
		return s.ok.Render(firstLine(m.status))
	}
	return ""
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return text[:i]
	}
	return text
}

func (m Model) viewHosts(width, height int) string {
	paneW := max(40, width-4)
	paneH := max(3, height-2)
	return m.styles().pane.Width(paneW).Height(paneH).Render(m.hostRows(paneW - 2))
}

func (m Model) hostRows(width int) string {
	var b strings.Builder
	s := m.styles()
	b.WriteString("Hosts\n\n")
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

func (m Model) viewAddHost(width, height int) string {
	nameCursor := " "
	targetCursor := " "
	if m.addField == 0 {
		nameCursor = ">"
	} else {
		targetCursor = ">"
	}
	body := fmt.Sprintf("Add Host\n\n%s Name:   %s\n%s Target: %s\n\nEnter moves/saves. Esc cancels.",
		nameCursor, m.addName, targetCursor, m.addTarget)
	return m.styles().pane.Width(max(40, width-4)).Height(max(3, height-2)).Render(body)
}

func (m Model) viewHostDetail(width, height int) string {
	var b strings.Builder
	s := m.styles()
	b.WriteString(fmt.Sprintf("Host: %s  %s\n\n", m.activeHost.Name, s.faint.Render(m.activeHost.Target)))
	b.WriteString(m.tabLine(width-4) + "\n\n")
	switch m.hostTab {
	case hostTabConfig:
		b.WriteString(m.viewHostConfig(width-4, height-5))
	case hostTabMappings:
		b.WriteString(m.viewMappings(width-4, height-5))
	default:
		b.WriteString(m.viewVMs(width-4, height-5))
	}
	return s.pane.Width(max(50, width-4)).Height(max(3, height-2)).Render(strings.TrimRight(b.String(), "\n"))
}

func (m Model) tabLine(width int) string {
	tabs := []string{"VMs", "Config", "Mappings"}
	parts := make([]string, 0, len(tabs))
	s := m.styles()
	for i, tab := range tabs {
		label := " " + tab + " "
		if i == m.hostTab {
			label = s.selected.Render(label)
		}
		parts = append(parts, label)
	}
	return clipText(strings.Join(parts, " "), width)
}

func (m Model) viewVMs(width, height int) string {
	var b strings.Builder
	s := m.styles()
	bodyW := max(50, width)
	nameW := max(24, bodyW-44)
	if len(m.vms) == 0 {
		b.WriteString("No VMs found under qemu:///system.")
		return b.String()
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
	return fitLines(strings.TrimRight(b.String(), "\n"), width, height)
}

func (m Model) viewHostConfig(width, height int) string {
	lines := []string{
		"Connection",
		"  Name:   " + m.activeHost.Name,
		"  Target: " + m.activeHost.Target,
		"",
		"Remote",
		"  Libvirt URI: qemu:///system",
		"  Ownership:  /var/lib/vmrelay/ownership.tsv",
		"  Setup:      press s to install/check required packages",
		"  Check:      press r to run a host readiness check",
		"",
		"Local State",
		"  Config:   " + m.configPath,
		"  Runtime:  " + m.stateDir,
		"  Theme:    " + m.config.Theme,
	}
	return fitLines(strings.Join(lines, "\n"), width, height)
}

func (m Model) viewMappings(width, height int) string {
	var b strings.Builder
	mappings := m.hostMappings()
	if len(mappings) == 0 {
		b.WriteString("No local port mappings configured for this host.\n\nPress n to add one.")
		return fitLines(b.String(), width, height)
	}
	nameW := max(12, min(24, width-52))
	b.WriteString("  " + cell("Name", nameW) + " " + cell("Local", 16) + " " + cell("Remote", 24) + " " + cell("Status", 8) + "\n")
	b.WriteString("  " + strings.Repeat("-", nameW) + " " + strings.Repeat("-", 16) + " " + strings.Repeat("-", 24) + " " + strings.Repeat("-", 8) + "\n")
	s := m.styles()
	for i, mapping := range mappings {
		cursor := " "
		if i == m.mapCursor {
			cursor = ">"
		}
		status := "stopped"
		if mappingActive(m.stateDir, m.activeHost.Name, mapping.ID) {
			status = "active"
		}
		local := fmt.Sprintf("127.0.0.1:%d", mapping.LocalPort)
		remote := fmt.Sprintf("%s:%d", mapping.RemoteHost, mapping.RemotePort)
		row := cursor + " " + cell(mapping.Name, nameW) + " " + cell(local, 16) + " " + cell(remote, 24) + " " + cell(status, 8)
		if i == m.mapCursor {
			row = s.selected.Render(row)
		}
		b.WriteString(row + "\n")
	}
	return fitLines(strings.TrimRight(b.String(), "\n"), width, height)
}

func (m Model) viewThemes(width, height int) string {
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
	return s.pane.Width(max(50, width-4)).Height(max(3, height-2)).Render(strings.TrimRight(b.String(), "\n"))
}

func (m Model) viewUpdatePrompt(width, height int) string {
	var b strings.Builder
	b.WriteString("Update Available\n\n")
	b.WriteString("Installed: " + m.version + "\n")
	b.WriteString("Available: " + m.updateInfo.Latest + "\n")
	if m.updateInfo.URL != "" {
		b.WriteString("Release:   " + m.updateInfo.URL + "\n")
	}
	b.WriteString("\nPress Enter to update and restart VMRelay, or n to skip for now.")
	return m.styles().pane.Width(max(50, width-4)).Height(max(3, height-2)).Render(b.String())
}

func (m Model) viewBusy(width, height int) string {
	return m.styles().pane.Width(max(40, width-4)).Height(max(3, height-2)).Render("Working\n\n" + m.status)
}

func (m Model) viewAddMapping(width, height int) string {
	cursors := []string{" ", " ", " ", " "}
	cursors[m.addMapField] = ">"
	body := fmt.Sprintf("Add Local Mapping for %s\n\n%s Name:        %s\n%s Local port:  %s\n%s Remote host: %s\n%s Remote port: %s\n\nLocal maps bind 127.0.0.1 on this machine and forward over SSH to the remote host.\nEnter moves/saves. Tab switches fields. Esc cancels.",
		m.activeHost.Name,
		cursors[0], m.addMapName,
		cursors[1], m.addMapLocalPort,
		cursors[2], m.addMapRemoteHost,
		cursors[3], m.addMapRemotePort)
	return m.styles().pane.Width(max(50, width-4)).Height(max(3, height-2)).Render(fitLines(body, width-6, height-4))
}

func (m Model) helpText() string {
	if !m.help {
		switch m.mode {
		case modeVMs:
			switch m.hostTab {
			case hostTabConfig:
				return "?: help  m: themes  b: hosts  left/right: tabs  r: check  s: setup  q: quit"
			case hostTabMappings:
				return "?: help  m: themes  b: hosts  left/right: tabs  n: add  e: start/stop  d: remove  q: quit"
			default:
				return "?: help  m: themes  b: hosts  left/right: tabs  r: refresh  p/f: power  o/x: console  a/h: ownership"
			}
		case modeAddHost:
			return "tab: switch field  enter: next/save  esc: cancel  q: quit"
		case modeAddMapping:
			return "tab: switch field  enter: next/save  esc: cancel  q: quit"
		case modeTheme:
			return "up/down: browse themes  enter: select  esc/b: back  q: quit"
		case modeUpdate:
			return "enter/y: update and restart  n/esc: skip  q: quit"
		default:
			return "?: help  m: themes  a: add host  enter/r: open host  t: test  s: setup  d: remove  q: quit"
		}
	}
	return "Hosts: a add, m themes, enter open host, t test, s setup, d remove. Host detail: left/right tabs. VMs: p lifecycle, o console. Mappings: n add, e start/stop, d remove."
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
		pane:     lipgloss.NewStyle().Padding(0, 1),
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

func fitLines(text string, width, height int) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = clipText(line, width)
	}
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
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
	for i := range cfg.Mappings {
		if cfg.Mappings[i].RemoteHost == "" {
			cfg.Mappings[i].RemoteHost = "127.0.0.1"
		}
		if cfg.Mappings[i].ID == "" {
			cfg.Mappings[i].ID = hash(fmt.Sprintf("%s-%s-%d-%s-%d", cfg.Mappings[i].Host, cfg.Mappings[i].Name, cfg.Mappings[i].LocalPort, cfg.Mappings[i].RemoteHost, cfg.Mappings[i].RemotePort))
		}
	}
	return cfg, nil
}

func saveConfig(path string, cfg Config) error {
	cfg.Version = configVersion
	sortHosts(cfg.Hosts)
	sortMappings(cfg.Mappings)
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

func sortMappings(mappings []PortMapping) {
	sort.SliceStable(mappings, func(i, j int) bool {
		if mappings[i].Host != mappings[j].Host {
			return mappings[i].Host < mappings[j].Host
		}
		if mappings[i].Name != mappings[j].Name {
			return mappings[i].Name < mappings[j].Name
		}
		return mappings[i].LocalPort < mappings[j].LocalPort
	})
}

func indexHost(hosts []Host, name string) int {
	for i, h := range hosts {
		if h.Name == name {
			return i
		}
	}
	return 0
}

func indexMapping(mappings []PortMapping, id string) int {
	for i, mapping := range mappings {
		if mapping.ID == id {
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

func startPortMapping(h Host, mapping PortMapping, stateDir string) (string, error) {
	if mapping.LocalPort == 0 || mapping.RemotePort == 0 {
		return "", fmt.Errorf("mapping ports are not configured")
	}
	if mapping.RemoteHost == "" {
		mapping.RemoteHost = "127.0.0.1"
	}
	if !portFree(mapping.LocalPort) {
		return "", fmt.Errorf("local port %d is already in use", mapping.LocalPort)
	}
	ctl := mappingControlPath(stateDir, h.Name, mapping.ID)
	_ = os.Remove(ctl)
	args := []string{
		"-f", "-N", "-M", "-S", ctl,
		"-o", "BatchMode=yes",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ControlPersist=yes",
		"-L", fmt.Sprintf("127.0.0.1:%d:%s:%d", mapping.LocalPort, mapping.RemoteHost, mapping.RemotePort),
		h.Target,
	}
	if out, err := runCommand(20*time.Second, "ssh", args...); err != nil {
		return strings.TrimSpace(out), fmt.Errorf("failed to start SSH mapping tunnel: %w", err)
	}
	return fmt.Sprintf("Started %s: 127.0.0.1:%d -> %s:%d.", mapping.Name, mapping.LocalPort, mapping.RemoteHost, mapping.RemotePort), nil
}

func stopPortMapping(h Host, mapping PortMapping, stateDir string) (string, error) {
	ctl := mappingControlPath(stateDir, h.Name, mapping.ID)
	if _, err := os.Stat(ctl); errors.Is(err, os.ErrNotExist) {
		return "Mapping " + mapping.Name + " is not running.", nil
	}
	out, err := runCommand(10*time.Second, "ssh", "-S", ctl, "-O", "exit", h.Target)
	_ = os.Remove(ctl)
	if err != nil {
		return strings.TrimSpace(out), err
	}
	return "Stopped mapping " + mapping.Name + ".", nil
}

func mappingActive(stateDir, host, id string) bool {
	_, err := os.Stat(mappingControlPath(stateDir, host, id))
	return err == nil
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

func mappingControlPath(stateDir, host, id string) string {
	return filepath.Join(stateDir, "mapping-"+hash(host+"-"+id)+".ctl")
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

func checkForUpdateCmd(current string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseAPI, nil)
		if err != nil {
			return updateCheckMsg{err: err}
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "vmrelay/"+current)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return updateCheckMsg{err: err}
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return updateCheckMsg{err: fmt.Errorf("release check returned %s", resp.Status)}
		}
		var release struct {
			TagName string `json:"tag_name"`
			URL     string `json:"html_url"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return updateCheckMsg{err: err}
		}
		latest := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
		info := updateInfo{Latest: latest, URL: release.URL}
		return updateCheckMsg{info: info, available: versionGreater(latest, current)}
	}
}

func restartCmd() tea.Cmd {
	return func() tea.Msg {
		exe, err := os.Executable()
		if err != nil {
			return updateFinishedMsg{err: err}
		}
		argv := append([]string{exe}, os.Args[1:]...)
		if err := syscall.Exec(exe, argv, os.Environ()); err != nil {
			return updateFinishedMsg{err: err}
		}
		return nil
	}
}

func versionGreater(latest, current string) bool {
	latestParts := versionParts(latest)
	currentParts := versionParts(current)
	for i := 0; i < max(len(latestParts), len(currentParts)); i++ {
		var a, b int
		if i < len(latestParts) {
			a = latestParts[i]
		}
		if i < len(currentParts) {
			b = currentParts[i]
		}
		if a != b {
			return a > b
		}
	}
	return false
}

func versionParts(version string) []int {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if version == "" {
		return nil
	}
	fields := strings.Split(version, ".")
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		n := 0
		for _, r := range field {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		parts = append(parts, n)
	}
	return parts
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
