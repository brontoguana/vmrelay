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
	"net/url"
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

type VMDetail struct {
	VM        VM
	Autostart string
	CPUs      string
	Memory    string
	Graphics  string
	Disks     []VMDisk
	NICs      []VMNIC
}

type VMDisk struct {
	Type   string
	Device string
	Target string
	Source string
}

type VMNIC struct {
	Interface string
	Type      string
	Source    string
	Model     string
	MAC       string
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
	modeVMDetail
	modeAddMapping
	modeCreateVM
	modeAddDisk
	modeImportDisk
	modeAddNIC
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

const (
	vmTabSummary = iota
	vmTabDisks
	vmTabNICs
	vmTabActions
	vmTabCount
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
	vmTab       int
	mapCursor   int
	diskCursor  int
	nicCursor   int
	themeCursor int
	vms         []VM
	vmDetail    VMDetail
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

	createVMName     string
	createVMMemory   string
	createVMCPUs     string
	createVMDiskSize string
	createVMDiskBus  string
	createVMISO      string
	createVMNetwork  string
	createVMFirmware string
	createVMShared   string
	createVMField    int

	addDiskPath   string
	addDiskSize   string
	addDiskTarget string
	addDiskField  int

	importDiskSource string
	importDiskDest   string
	importDiskTarget string
	importDiskField  int

	addNICSource string
	addNICModel  string
	addNICField  int
}

type resultMsg struct {
	op     string
	output string
	vms    []VM
	detail VMDetail
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
	case modeVMDetail:
		b.WriteString(m.viewVMDetail(innerW, contentH))
	case modeAddMapping:
		b.WriteString(m.viewAddMapping(innerW, contentH))
	case modeCreateVM:
		b.WriteString(m.viewCreateVM(innerW, contentH))
	case modeAddDisk:
		b.WriteString(m.viewAddDisk(innerW, contentH))
	case modeImportDisk:
		b.WriteString(m.viewImportDisk(innerW, contentH))
	case modeAddNIC:
		b.WriteString(m.viewAddNIC(innerW, contentH))
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
	if msg.String() == "m" && m.mode != modeAddHost && m.mode != modeAddMapping && m.mode != modeCreateVM && m.mode != modeAddDisk && m.mode != modeImportDisk && m.mode != modeAddNIC && m.mode != modeBusy && m.mode != modeTheme {
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
	case modeVMDetail:
		return m.updateVMDetailKey(msg)
	case modeAddMapping:
		return m.updateAddMappingKey(msg)
	case modeCreateVM:
		return m.updateCreateVMKey(msg)
	case modeAddDisk:
		return m.updateAddDiskKey(msg)
	case modeImportDisk:
		return m.updateImportDiskKey(msg)
	case modeAddNIC:
		return m.updateAddNICKey(msg)
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

func (m Model) updateCreateVMKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeVMs
		m.status = "Cancelled VM creation."
		m.errText = ""
	case "tab":
		m.createVMField = (m.createVMField + 1) % 9
	case "shift+tab":
		m.createVMField = (m.createVMField + 8) % 9
	case "enter":
		if m.createVMField < 8 {
			m.createVMField++
			return m, nil
		}
		req, err := m.pendingVMCreate()
		if err != nil {
			m.errText = err.Error()
			return m, nil
		}
		return m.busy(modeVMs, "Creating VM "+req.Name+" on "+m.activeHost.Name+"...", "vm-create", func() resultMsg {
			out, err := createVM(m.activeHost, req)
			return resultMsg{op: "vm-create", output: out, err: err}
		})
	case "backspace", "ctrl+h":
		switch m.createVMField {
		case 0:
			m.createVMName = trimLastRune(m.createVMName)
		case 1:
			m.createVMMemory = trimLastRune(m.createVMMemory)
		case 2:
			m.createVMCPUs = trimLastRune(m.createVMCPUs)
		case 3:
			m.createVMDiskSize = trimLastRune(m.createVMDiskSize)
		case 4:
			m.createVMDiskBus = trimLastRune(m.createVMDiskBus)
		case 5:
			m.createVMISO = trimLastRune(m.createVMISO)
		case 6:
			m.createVMNetwork = trimLastRune(m.createVMNetwork)
		case 7:
			m.createVMFirmware = trimLastRune(m.createVMFirmware)
		case 8:
			m.createVMShared = trimLastRune(m.createVMShared)
		}
	default:
		if msg.Type == tea.KeyRunes {
			switch m.createVMField {
			case 0:
				m.createVMName += msg.String()
			case 1:
				m.createVMMemory += msg.String()
			case 2:
				m.createVMCPUs += msg.String()
			case 3:
				m.createVMDiskSize += msg.String()
			case 4:
				m.createVMDiskBus += msg.String()
			case 5:
				m.createVMISO += msg.String()
			case 6:
				m.createVMNetwork += msg.String()
			case 7:
				m.createVMFirmware += msg.String()
			case 8:
				m.createVMShared += msg.String()
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
	case "enter":
		if m.hostTab == hostTabVMs {
			if vm, ok := m.selectedVM(); ok {
				m.vmTab = vmTabSummary
				m.diskCursor = 0
				m.nicCursor = 0
				return m.loadVMDetail(m.activeHost, vm)
			}
		}
	case "n":
		if m.hostTab == hostTabConfig {
			m.mode = modeCreateVM
			m.createVMName = ""
			m.createVMMemory = "4"
			m.createVMCPUs = "2"
			m.createVMDiskSize = "64"
			m.createVMDiskBus = "sata"
			m.createVMISO = ""
			m.createVMNetwork = "default"
			m.createVMFirmware = "uefi"
			m.createVMShared = "n"
			m.createVMField = 0
			m.status = "Create a new VM from a remote ISO."
			m.errText = ""
		} else if m.hostTab == hostTabMappings {
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

func (m Model) updateVMDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "b", "esc":
		m.mode = modeVMs
		m.status = "Back to " + m.activeHost.Name + "."
		m.errText = ""
	case "left", "[":
		if m.vmTab > 0 {
			m.vmTab--
		}
	case "right", "]":
		if m.vmTab < vmTabCount-1 {
			m.vmTab++
		}
	case "up", "k":
		switch m.vmTab {
		case vmTabDisks:
			if m.diskCursor > 0 {
				m.diskCursor--
			}
		case vmTabNICs:
			if m.nicCursor > 0 {
				m.nicCursor--
			}
		}
	case "down", "j":
		switch m.vmTab {
		case vmTabDisks:
			if m.diskCursor < len(m.vmDetail.Disks)-1 {
				m.diskCursor++
			}
		case vmTabNICs:
			if m.nicCursor < len(m.vmDetail.NICs)-1 {
				m.nicCursor++
			}
		}
	case "r":
		return m.loadVMDetail(m.activeHost, m.vmDetail.VM)
	case "p":
		vm := m.vmDetail.VM
		action := "start"
		if strings.Contains(strings.ToLower(vm.State), "running") {
			action = "shutdown"
		}
		return m.busy(modeVMDetail, action+" "+vm.Name+"...", action, func() resultMsg {
			out, err := lifecycle(m.activeHost, vm.Name, action)
			return resultMsg{op: action, output: out, err: err}
		})
	case "f":
		vm := m.vmDetail.VM
		return m.busy(modeVMDetail, "Force off "+vm.Name+"...", "destroy", func() resultMsg {
			out, err := lifecycle(m.activeHost, vm.Name, "destroy")
			return resultMsg{op: "destroy", output: out, err: err}
		})
	case "o":
		vm := m.vmDetail.VM
		return m.busy(modeVMDetail, "Opening console for "+vm.Name+"...", "console", func() resultMsg {
			out, err := openConsole(m.activeHost, vm.Name, m.stateDir)
			return resultMsg{op: "console", output: out, err: err}
		})
	case "c":
		vm := m.vmDetail.VM
		return m.busy(modeVMDetail, "Stopping console for "+vm.Name+"...", "console-down", func() resultMsg {
			out, err := closeConsole(m.activeHost, vm.Name, m.stateDir)
			return resultMsg{op: "console-down", output: out, err: err}
		})
	case "a":
		vm := m.vmDetail.VM
		return m.busy(modeVMDetail, "Adopting "+vm.Name+"...", "adopt", func() resultMsg {
			out, err := setOwnership(m.activeHost, vm, false, false)
			return resultMsg{op: "adopt", output: out, err: err}
		})
	case "h":
		vm := m.vmDetail.VM
		return m.busy(modeVMDetail, "Toggling shared flag for "+vm.Name+"...", "share", func() resultMsg {
			out, err := setOwnership(m.activeHost, vm, !vm.Shared, true)
			return resultMsg{op: "share", output: out, err: err}
		})
	case "n":
		switch m.vmTab {
		case vmTabDisks:
			m.mode = modeAddDisk
			m.addDiskPath = ""
			m.addDiskSize = "20"
			m.addDiskTarget = ""
			m.addDiskField = 0
			m.status = "Create and attach a qcow2 disk."
			m.errText = ""
		case vmTabNICs:
			m.mode = modeAddNIC
			m.addNICSource = "default"
			m.addNICModel = "virtio"
			m.addNICField = 0
			m.status = "Attach a network interface."
			m.errText = ""
		}
	case "i":
		if m.vmTab == vmTabDisks {
			m.mode = modeImportDisk
			m.importDiskSource = ""
			m.importDiskDest = ""
			m.importDiskTarget = ""
			m.importDiskField = 0
			m.status = "Import, convert if needed, and attach a disk."
			m.errText = ""
		}
	case "x":
		switch m.vmTab {
		case vmTabDisks:
			disk, ok := m.selectedDisk()
			if !ok || disk.Target == "" {
				return m, nil
			}
			vm := m.vmDetail.VM
			return m.busy(modeVMDetail, "Detaching disk "+disk.Target+" from "+vm.Name+"...", "disk-detach", func() resultMsg {
				out, err := detachDisk(m.activeHost, vm.Name, disk)
				return resultMsg{op: "disk-detach", output: out, err: err}
			})
		case vmTabNICs:
			nic, ok := m.selectedNIC()
			if !ok || nic.MAC == "" {
				return m, nil
			}
			vm := m.vmDetail.VM
			return m.busy(modeVMDetail, "Detaching NIC "+nic.MAC+" from "+vm.Name+"...", "nic-detach", func() resultMsg {
				out, err := detachNIC(m.activeHost, vm.Name, nic)
				return resultMsg{op: "nic-detach", output: out, err: err}
			})
		}
	case "enter":
		if m.vmTab == vmTabDisks {
			disk, ok := m.selectedDisk()
			if !ok || disk.Target == "" {
				return m, nil
			}
			vm := m.vmDetail.VM
			return m.busy(modeVMDetail, "Setting "+disk.Target+" as boot disk for "+vm.Name+"...", "disk-boot", func() resultMsg {
				out, err := setBootDisk(m.activeHost, vm.Name, disk)
				return resultMsg{op: "disk-boot", output: out, err: err}
			})
		}
	}
	return m, nil
}

func (m Model) updateAddDiskKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeVMDetail
		m.status = "Cancelled disk creation."
		m.errText = ""
	case "tab":
		m.addDiskField = (m.addDiskField + 1) % 3
	case "shift+tab":
		m.addDiskField = (m.addDiskField + 2) % 3
	case "enter":
		if m.addDiskField < 2 {
			m.addDiskField++
			return m, nil
		}
		req, err := m.pendingDiskCreate()
		if err != nil {
			m.errText = err.Error()
			return m, nil
		}
		vm := m.vmDetail.VM
		return m.busy(modeVMDetail, "Creating disk for "+vm.Name+"...", "disk-create", func() resultMsg {
			out, err := createAndAttachDisk(m.activeHost, vm.Name, req)
			return resultMsg{op: "disk-create", output: out, err: err}
		})
	case "backspace", "ctrl+h":
		switch m.addDiskField {
		case 0:
			m.addDiskSize = trimLastRune(m.addDiskSize)
		case 1:
			m.addDiskPath = trimLastRune(m.addDiskPath)
		case 2:
			m.addDiskTarget = trimLastRune(m.addDiskTarget)
		}
	default:
		if msg.Type == tea.KeyRunes {
			switch m.addDiskField {
			case 0:
				m.addDiskSize += msg.String()
			case 1:
				m.addDiskPath += msg.String()
			case 2:
				m.addDiskTarget += msg.String()
			}
		}
	}
	return m, nil
}

func (m Model) updateImportDiskKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeVMDetail
		m.status = "Cancelled disk import."
		m.errText = ""
	case "tab":
		m.importDiskField = (m.importDiskField + 1) % 3
	case "shift+tab":
		m.importDiskField = (m.importDiskField + 2) % 3
	case "enter":
		if m.importDiskField < 2 {
			m.importDiskField++
			return m, nil
		}
		req, err := m.pendingDiskImport()
		if err != nil {
			m.errText = err.Error()
			return m, nil
		}
		vm := m.vmDetail.VM
		return m.busy(modeVMDetail, "Importing disk for "+vm.Name+"...", "disk-import", func() resultMsg {
			out, err := importAndAttachDisk(m.activeHost, vm.Name, req)
			return resultMsg{op: "disk-import", output: out, err: err}
		})
	case "backspace", "ctrl+h":
		switch m.importDiskField {
		case 0:
			m.importDiskSource = trimLastRune(m.importDiskSource)
		case 1:
			m.importDiskDest = trimLastRune(m.importDiskDest)
		case 2:
			m.importDiskTarget = trimLastRune(m.importDiskTarget)
		}
	default:
		if msg.Type == tea.KeyRunes {
			switch m.importDiskField {
			case 0:
				m.importDiskSource += msg.String()
			case 1:
				m.importDiskDest += msg.String()
			case 2:
				m.importDiskTarget += msg.String()
			}
		}
	}
	return m, nil
}

func (m Model) updateAddNICKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeVMDetail
		m.status = "Cancelled NIC attach."
		m.errText = ""
	case "tab", "shift+tab":
		m.addNICField = 1 - m.addNICField
	case "enter":
		if m.addNICField == 0 {
			m.addNICField = 1
			return m, nil
		}
		req, err := m.pendingNICAdd()
		if err != nil {
			m.errText = err.Error()
			return m, nil
		}
		vm := m.vmDetail.VM
		return m.busy(modeVMDetail, "Attaching NIC to "+vm.Name+"...", "nic-add", func() resultMsg {
			out, err := attachNIC(m.activeHost, vm.Name, req)
			return resultMsg{op: "nic-add", output: out, err: err}
		})
	case "backspace", "ctrl+h":
		if m.addNICField == 0 {
			m.addNICSource = trimLastRune(m.addNICSource)
		} else {
			m.addNICModel = trimLastRune(m.addNICModel)
		}
	default:
		if msg.Type == tea.KeyRunes {
			if m.addNICField == 0 {
				m.addNICSource += msg.String()
			} else {
				m.addNICModel += msg.String()
			}
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
	case "vm-detail":
		m.vmDetail = msg.detail
		if m.diskCursor >= len(m.vmDetail.Disks) {
			m.diskCursor = max(0, len(m.vmDetail.Disks)-1)
		}
		if m.nicCursor >= len(m.vmDetail.NICs) {
			m.nicCursor = max(0, len(m.vmDetail.NICs)-1)
		}
		m.status = "Loaded " + m.vmDetail.VM.Name + "."
	case "start", "shutdown", "destroy", "adopt", "share":
		m.status = strings.TrimSpace(msg.output)
		if m.status == "" {
			m.status = msg.op + " complete."
		}
		if m.priorMode == modeVMDetail {
			return m.loadVMDetail(m.activeHost, m.vmDetail.VM)
		}
		return m.loadVMs(m.activeHost)
	case "vm-create":
		m.status = strings.TrimSpace(msg.output)
		if m.status == "" {
			m.status = "VM created."
		}
		m.hostTab = hostTabVMs
		return m.loadVMs(m.activeHost)
	case "disk-create", "disk-import", "disk-detach", "disk-boot", "nic-add", "nic-detach":
		m.status = strings.TrimSpace(msg.output)
		if m.status == "" {
			m.status = msg.op + " complete."
		}
		return m.loadVMDetail(m.activeHost, m.vmDetail.VM)
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
	case "vm-detail":
		return "Failed to load VM detail: " + msg.err.Error()
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
	case "vm-create":
		return "VM creation failed: " + msg.err.Error()
	case "disk-create":
		return "Disk creation failed: " + msg.err.Error()
	case "disk-import":
		return "Disk import failed: " + msg.err.Error()
	case "disk-detach":
		return "Disk detach failed: " + msg.err.Error()
	case "disk-boot":
		return "Boot disk update failed: " + msg.err.Error()
	case "nic-add":
		return "NIC attach failed: " + msg.err.Error()
	case "nic-detach":
		return "NIC detach failed: " + msg.err.Error()
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

func (m Model) loadVMDetail(h Host, vm VM) (tea.Model, tea.Cmd) {
	m.activeHost = h
	m.vmDetail.VM = vm
	m.priorMode = modeVMDetail
	m.mode = modeBusy
	m.status = "Loading " + vm.Name + "..."
	m.errText = ""
	return m, func() tea.Msg {
		detail, out, err := getVMDetail(h, vm)
		return resultMsg{op: "vm-detail", output: out, detail: detail, err: err}
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

func (m Model) selectedDisk() (VMDisk, bool) {
	if len(m.vmDetail.Disks) == 0 || m.diskCursor < 0 || m.diskCursor >= len(m.vmDetail.Disks) {
		return VMDisk{}, false
	}
	return m.vmDetail.Disks[m.diskCursor], true
}

func (m Model) selectedNIC() (VMNIC, bool) {
	if len(m.vmDetail.NICs) == 0 || m.nicCursor < 0 || m.nicCursor >= len(m.vmDetail.NICs) {
		return VMNIC{}, false
	}
	return m.vmDetail.NICs[m.nicCursor], true
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

func (m Model) pendingVMCreate() (vmCreateRequest, error) {
	name := strings.TrimSpace(m.createVMName)
	if !validName(name) {
		return vmCreateRequest{}, fmt.Errorf("VM name must use letters, numbers, dot, dash, or underscore")
	}
	memoryGiB, err := strconv.Atoi(strings.TrimSpace(m.createVMMemory))
	if err != nil || memoryGiB < 1 || memoryGiB > 1024 {
		return vmCreateRequest{}, fmt.Errorf("memory must be 1-1024 GiB")
	}
	cpus, err := strconv.Atoi(strings.TrimSpace(m.createVMCPUs))
	if err != nil || cpus < 1 || cpus > 256 {
		return vmCreateRequest{}, fmt.Errorf("CPUs must be 1-256")
	}
	diskGiB, err := strconv.Atoi(strings.TrimSpace(m.createVMDiskSize))
	if err != nil || diskGiB < 1 || diskGiB > 65536 {
		return vmCreateRequest{}, fmt.Errorf("disk size must be 1-65536 GiB")
	}
	diskBus := strings.ToLower(strings.TrimSpace(m.createVMDiskBus))
	if diskBus == "" {
		diskBus = "sata"
	}
	switch diskBus {
	case "sata", "virtio", "scsi", "ide":
	default:
		return vmCreateRequest{}, fmt.Errorf("disk bus must be sata, virtio, scsi, or ide")
	}
	iso := strings.TrimSpace(m.createVMISO)
	if err := validateRequiredAbsPath(iso, "ISO path"); err != nil {
		return vmCreateRequest{}, err
	}
	network := strings.TrimSpace(m.createVMNetwork)
	if network == "" {
		network = "default"
	}
	if strings.ContainsAny(network, "\r\n\t ") {
		return vmCreateRequest{}, fmt.Errorf("network must not contain spaces")
	}
	firmware := strings.ToLower(strings.TrimSpace(m.createVMFirmware))
	if firmware == "" {
		firmware = "uefi"
	}
	if firmware != "uefi" && firmware != "bios" {
		return vmCreateRequest{}, fmt.Errorf("firmware must be uefi or bios")
	}
	sharedText := strings.ToLower(strings.TrimSpace(m.createVMShared))
	shared := sharedText == "y" || sharedText == "yes" || sharedText == "true" || sharedText == "1" || sharedText == "shared"
	if sharedText != "" && !shared && sharedText != "n" && sharedText != "no" && sharedText != "false" && sharedText != "0" && sharedText != "private" {
		return vmCreateRequest{}, fmt.Errorf("shared must be y/n")
	}
	return vmCreateRequest{
		Name:      name,
		MemoryMiB: memoryGiB * 1024,
		CPUs:      cpus,
		DiskGiB:   diskGiB,
		DiskBus:   diskBus,
		ISO:       iso,
		Network:   network,
		Firmware:  firmware,
		Shared:    shared,
	}, nil
}

type diskCreateRequest struct {
	SizeGiB int
	Path    string
	Target  string
}

type vmCreateRequest struct {
	Name      string
	MemoryMiB int
	CPUs      int
	DiskGiB   int
	DiskBus   string
	ISO       string
	Network   string
	Firmware  string
	Shared    bool
}

type diskImportRequest struct {
	Source string
	Dest   string
	Target string
}

type nicAddRequest struct {
	Source string
	Model  string
}

func (m Model) pendingDiskCreate() (diskCreateRequest, error) {
	size, err := strconv.Atoi(strings.TrimSpace(m.addDiskSize))
	if err != nil || size < 1 || size > 65536 {
		return diskCreateRequest{}, fmt.Errorf("disk size must be 1-65536 GiB")
	}
	path := strings.TrimSpace(m.addDiskPath)
	if err := validateOptionalAbsPath(path, "disk path"); err != nil {
		return diskCreateRequest{}, err
	}
	target := strings.TrimSpace(m.addDiskTarget)
	if err := validateTarget(target); err != nil {
		return diskCreateRequest{}, err
	}
	return diskCreateRequest{SizeGiB: size, Path: path, Target: target}, nil
}

func (m Model) pendingDiskImport() (diskImportRequest, error) {
	source := strings.TrimSpace(m.importDiskSource)
	if err := validateRequiredAbsPath(source, "source disk path"); err != nil {
		return diskImportRequest{}, err
	}
	dest := strings.TrimSpace(m.importDiskDest)
	if err := validateOptionalAbsPath(dest, "destination path"); err != nil {
		return diskImportRequest{}, err
	}
	target := strings.TrimSpace(m.importDiskTarget)
	if err := validateTarget(target); err != nil {
		return diskImportRequest{}, err
	}
	return diskImportRequest{Source: source, Dest: dest, Target: target}, nil
}

func (m Model) pendingNICAdd() (nicAddRequest, error) {
	source := strings.TrimSpace(m.addNICSource)
	if source == "" || strings.ContainsAny(source, "\r\n\t ") {
		return nicAddRequest{}, fmt.Errorf("network source is required and must not contain spaces")
	}
	model := strings.TrimSpace(m.addNICModel)
	if model == "" {
		model = "virtio"
	}
	if strings.ContainsAny(model, "\r\n\t ") {
		return nicAddRequest{}, fmt.Errorf("NIC model must not contain spaces")
	}
	return nicAddRequest{Source: source, Model: model}, nil
}

func validateRequiredAbsPath(path, label string) error {
	if path == "" {
		return fmt.Errorf("%s is required", label)
	}
	return validateOptionalAbsPath(path, label)
}

func validateOptionalAbsPath(path, label string) error {
	if path == "" {
		return nil
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("%s must be an absolute remote path", label)
	}
	if strings.ContainsAny(path, "\r\n\t") {
		return fmt.Errorf("%s must not contain control whitespace", label)
	}
	return nil
}

func validateTarget(target string) error {
	if target == "" {
		return nil
	}
	if len(target) < 3 || !strings.HasPrefix(target, "vd") {
		return fmt.Errorf("target should look like vdb, vdc, etc")
	}
	for _, r := range target {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		return fmt.Errorf("target should use lowercase letters and numbers")
	}
	return nil
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

func (m Model) viewVMDetail(width, height int) string {
	var b strings.Builder
	s := m.styles()
	vm := m.vmDetail.VM
	b.WriteString(fmt.Sprintf("VM: %s  %s\n\n", vm.Name, s.faint.Render(m.activeHost.Name+" / "+vm.State)))
	b.WriteString(m.vmTabLine(width-4) + "\n\n")
	bodyH := height - 5
	switch m.vmTab {
	case vmTabDisks:
		b.WriteString(m.viewVMDisks(width-4, bodyH))
	case vmTabNICs:
		b.WriteString(m.viewVMNICs(width-4, bodyH))
	case vmTabActions:
		b.WriteString(m.viewVMActions(width-4, bodyH))
	default:
		b.WriteString(m.viewVMSummary(width-4, bodyH))
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

func (m Model) vmTabLine(width int) string {
	tabs := []string{"Summary", "Disks", "NICs", "Actions"}
	parts := make([]string, 0, len(tabs))
	s := m.styles()
	for i, tab := range tabs {
		label := " " + tab + " "
		if i == m.vmTab {
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

func (m Model) viewVMSummary(width, height int) string {
	vm := m.vmDetail.VM
	shared := "private"
	if vm.Shared {
		shared = "shared"
	}
	lines := []string{
		"Identity",
		"  Name:       " + vm.Name,
		"  UUID:       " + valueOr(vm.UUID, "unknown"),
		"  State:      " + valueOr(vm.State, "unknown"),
		"  Owner:      " + ownerLabel(vm.Owner),
		"  Visibility: " + shared,
		"",
		"Runtime",
		"  CPUs:       " + valueOr(m.vmDetail.CPUs, "unknown"),
		"  Memory:     " + valueOr(m.vmDetail.Memory, "unknown"),
		"  Autostart:  " + valueOr(m.vmDetail.Autostart, "unknown"),
		"  Graphics:   " + valueOr(m.vmDetail.Graphics, "none"),
		"",
		"Inventory",
		fmt.Sprintf("  Disks:      %d", len(m.vmDetail.Disks)),
		fmt.Sprintf("  NICs:       %d", len(m.vmDetail.NICs)),
	}
	return fitLines(strings.Join(lines, "\n"), width, height)
}

func (m Model) viewVMDisks(width, height int) string {
	if len(m.vmDetail.Disks) == 0 {
		return fitLines("No disks reported by libvirt.\n\nPress n to create a qcow2 disk or i to import an existing disk.", width, height)
	}
	var b strings.Builder
	s := m.styles()
	targetW := 8
	deviceW := 8
	sourceW := max(20, width-targetW-deviceW-8)
	b.WriteString("  " + cell("Target", targetW) + " " + cell("Device", deviceW) + " " + cell("Source", sourceW) + "\n")
	b.WriteString("  " + strings.Repeat("-", targetW) + " " + strings.Repeat("-", deviceW) + " " + strings.Repeat("-", sourceW) + "\n")
	for i, disk := range m.vmDetail.Disks {
		cursor := " "
		if i == m.diskCursor {
			cursor = ">"
		}
		row := cursor + " " + cell(disk.Target, targetW) + " " + cell(valueOr(disk.Device, disk.Type), deviceW) + " " + cell(disk.Source, sourceW)
		if i == m.diskCursor {
			row = s.selected.Render(row)
		}
		b.WriteString(row + "\n")
	}
	return fitLines(strings.TrimRight(b.String(), "\n"), width, height)
}

func (m Model) viewVMNICs(width, height int) string {
	if len(m.vmDetail.NICs) == 0 {
		return fitLines("No NICs reported by libvirt.\n\nPress n to attach a NIC to a libvirt network.", width, height)
	}
	var b strings.Builder
	s := m.styles()
	sourceW := max(10, min(20, width-46))
	b.WriteString("  " + cell("MAC", 18) + " " + cell("Type", 8) + " " + cell("Source", sourceW) + " " + cell("Model", 10) + "\n")
	b.WriteString("  " + strings.Repeat("-", 18) + " " + strings.Repeat("-", 8) + " " + strings.Repeat("-", sourceW) + " " + strings.Repeat("-", 10) + "\n")
	for i, nic := range m.vmDetail.NICs {
		cursor := " "
		if i == m.nicCursor {
			cursor = ">"
		}
		row := cursor + " " + cell(nic.MAC, 18) + " " + cell(nic.Type, 8) + " " + cell(nic.Source, sourceW) + " " + cell(nic.Model, 10)
		if i == m.nicCursor {
			row = s.selected.Render(row)
		}
		b.WriteString(row + "\n")
	}
	return fitLines(strings.TrimRight(b.String(), "\n"), width, height)
}

func (m Model) viewVMActions(width, height int) string {
	action := "start"
	if strings.Contains(strings.ToLower(m.vmDetail.VM.State), "running") {
		action = "shutdown"
	}
	lines := []string{
		"Power",
		"  p: " + action,
		"  f: force off",
		"",
		"Console",
		"  o: open browser console",
		"  c: stop console tunnel",
		"",
		"Ownership",
		"  a: adopt unmanaged VM",
		"  h: toggle shared/private",
		"",
		"Refresh",
		"  r: reload detail",
	}
	return fitLines(strings.Join(lines, "\n"), width, height)
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
		"  Create VM:  press n to create a VM from a remote ISO",
		"",
		"Local State",
		"  Config:   " + m.configPath,
		"  Runtime:  " + m.stateDir,
		"  Theme:    " + m.config.Theme,
	}
	return fitLines(strings.Join(lines, "\n"), width, height)
}

func (m Model) viewCreateVM(width, height int) string {
	cursors := []string{" ", " ", " ", " ", " ", " ", " ", " ", " "}
	cursors[m.createVMField] = ">"
	body := fmt.Sprintf("Create VM on %s\n\n%s Name:      %s\n%s Memory GiB:%s\n%s CPUs:      %s\n%s Disk GiB:  %s\n%s Disk bus:  %s\n%s ISO path:  %s\n%s Network:   %s\n%s Firmware:  %s\n%s Shared:    %s\n\nISO path is on the remote host. Disk bus can be sata, virtio, scsi, or ide; sata is safest for Windows installers. Firmware is uefi or bios. Shared is y/n.\nVMRelay creates /var/lib/libvirt/images/<name>.qcow2, starts a VNC install VM, and records ownership for the remote SSH user.\nEnter moves/saves. Tab switches fields. Esc cancels.",
		m.activeHost.Name,
		cursors[0], m.createVMName,
		cursors[1], m.createVMMemory,
		cursors[2], m.createVMCPUs,
		cursors[3], m.createVMDiskSize,
		cursors[4], m.createVMDiskBus,
		cursors[5], m.createVMISO,
		cursors[6], m.createVMNetwork,
		cursors[7], m.createVMFirmware,
		cursors[8], m.createVMShared)
	return m.styles().pane.Width(max(54, width-4)).Height(max(3, height-2)).Render(fitLines(body, width-6, height-4))
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

func (m Model) viewAddDisk(width, height int) string {
	cursors := []string{" ", " ", " "}
	cursors[m.addDiskField] = ">"
	body := fmt.Sprintf("Create Disk for %s\n\n%s Size GiB:    %s\n%s Image path:   %s\n%s Target dev:   %s\n\nLeave image path blank to create /var/lib/libvirt/images/<vm>-<target>.qcow2.\nLeave target blank to use the next available virtio disk target.\nEnter moves/saves. Tab switches fields. Esc cancels.",
		m.vmDetail.VM.Name,
		cursors[0], m.addDiskSize,
		cursors[1], m.addDiskPath,
		cursors[2], m.addDiskTarget)
	return m.styles().pane.Width(max(50, width-4)).Height(max(3, height-2)).Render(fitLines(body, width-6, height-4))
}

func (m Model) viewImportDisk(width, height int) string {
	cursors := []string{" ", " ", " "}
	cursors[m.importDiskField] = ">"
	body := fmt.Sprintf("Import Disk for %s\n\n%s Source path:  %s\n%s Dest qcow2:   %s\n%s Target dev:   %s\n\nSource is a remote host path. VMRelay detects the format with qemu-img; non-qcow2 sources are converted to qcow2 before attach.\nLeave destination blank to import under /var/lib/libvirt/images.\nEnter moves/saves. Tab switches fields. Esc cancels.",
		m.vmDetail.VM.Name,
		cursors[0], m.importDiskSource,
		cursors[1], m.importDiskDest,
		cursors[2], m.importDiskTarget)
	return m.styles().pane.Width(max(50, width-4)).Height(max(3, height-2)).Render(fitLines(body, width-6, height-4))
}

func (m Model) viewAddNIC(width, height int) string {
	sourceCursor := " "
	modelCursor := " "
	if m.addNICField == 0 {
		sourceCursor = ">"
	} else {
		modelCursor = ">"
	}
	body := fmt.Sprintf("Attach NIC to %s\n\n%s Network: %s\n%s Model:   %s\n\nNetwork is a libvirt network name, usually default. Model defaults to virtio.\nEnter moves/saves. Tab switches fields. Esc cancels.",
		m.vmDetail.VM.Name,
		sourceCursor, m.addNICSource,
		modelCursor, m.addNICModel)
	return m.styles().pane.Width(max(50, width-4)).Height(max(3, height-2)).Render(fitLines(body, width-6, height-4))
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
				return "?: help  m: themes  b: hosts  left/right: tabs  n: create VM  r: check  s: setup"
			case hostTabMappings:
				return "?: help  m: themes  b: hosts  left/right: tabs  n: add  e: start/stop  d: remove  q: quit"
			default:
				return "?: help  m: themes  b: hosts  enter: VM detail  left/right: tabs  r: refresh  p/f: power  o/x: console"
			}
		case modeVMDetail:
			switch m.vmTab {
			case vmTabDisks:
				return "?: help  m: themes  b/esc: host  left/right: tabs  enter: boot disk  n/i: add/import  x: detach"
			case vmTabNICs:
				return "?: help  m: themes  b/esc: host  left/right: tabs  n: add NIC  x: detach  r: refresh"
			case vmTabActions:
				return "?: help  m: themes  b/esc: host  left/right: tabs  p/f: power  o/c: console  a/h: ownership"
			default:
				return "?: help  m: themes  b/esc: host  left/right: tabs  r: refresh  p/f: power  o: console"
			}
		case modeAddHost:
			return "tab: switch field  enter: next/save  esc: cancel  q: quit"
		case modeAddMapping:
			return "tab: switch field  enter: next/save  esc: cancel  q: quit"
		case modeCreateVM:
			return "tab: switch field  enter: next/save  esc: cancel  q: quit"
		case modeAddDisk:
			return "tab: switch field  enter: next/save  esc: cancel  q: quit"
		case modeImportDisk:
			return "tab: switch field  enter: next/save  esc: cancel  q: quit"
		case modeAddNIC:
			return "tab: switch field  enter: next/save  esc: cancel  q: quit"
		case modeTheme:
			return "up/down: browse themes  enter: select  esc/b: back  q: quit"
		case modeUpdate:
			return "enter/y: update and restart  n/esc: skip  q: quit"
		default:
			return "?: help  m: themes  a: add host  enter/r: open host  t: test  s: setup  d: remove  q: quit"
		}
	}
	return "Hosts: a add, m themes, enter open host. Host detail: enter opens VM detail. VM detail: disks n/i/x, NICs n/x, actions p/f/o/c/a/h. Mappings: n add, e start/stop, d remove."
}

func ownerLabel(owner string) string {
	if owner == "" {
		return "unmanaged"
	}
	return owner
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
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
if [ -d /usr/share/OVMF ] || [ -d /usr/share/ovmf ] || [ -e /usr/share/qemu/OVMF.fd ]; then printf 'OVMF/UEFI: yes\n'; else printf 'OVMF/UEFI: missing\n'; fi
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
  sudo -n apt-get install -y qemu-kvm libvirt-daemon-system libvirt-clients virtinst qemu-utils ovmf novnc websockify
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

func getVMDetail(h Host, vm VM) (VMDetail, string, error) {
	script := fmt.Sprintf(`
set -euo pipefail
vm=%s
policy=/var/lib/vmrelay/ownership.tsv
if [ ! -r "$policy" ]; then policy=/dev/null; fi
uuid="$(virsh -c qemu:///system domuuid "$vm" 2>/dev/null || true)"
state="$(virsh -c qemu:///system domstate "$vm" 2>/dev/null | tr '\n' ' ' | sed 's/[[:space:]]*$//' || true)"
owner=""
shared="0"
if [ -n "$uuid" ]; then
  line="$(awk -F '\t' -v id="$uuid" '$1 == id { print; exit }' "$policy" 2>/dev/null || true)"
  if [ -n "$line" ]; then
    owner="$(printf '%%s\n' "$line" | awk -F '\t' '{print $2}')"
    shared="$(printf '%%s\n' "$line" | awk -F '\t' '{print $3}')"
  fi
fi
info="$(virsh -c qemu:///system dominfo "$vm" 2>/dev/null || true)"
cpus="$(printf '%%s\n' "$info" | awk -F: '$1 == "CPU(s)" { gsub(/^[[:space:]]+/, "", $2); print $2; exit }')"
memory="$(printf '%%s\n' "$info" | awk -F: '$1 == "Max memory" { gsub(/^[[:space:]]+/, "", $2); print $2; exit }')"
autostart="$(printf '%%s\n' "$info" | awk -F: '$1 == "Autostart" { gsub(/^[[:space:]]+/, "", $2); print $2; exit }')"
graphics="$(virsh -c qemu:///system domdisplay "$vm" 2>/dev/null || true)"
printf 'VMRELAY_DETAIL\t%%s\t%%s\t%%s\t%%s\t%%s\t%%s\t%%s\t%%s\t%%s\n' "$vm" "$uuid" "$state" "$owner" "$shared" "$autostart" "$cpus" "$memory" "$graphics"
virsh -c qemu:///system domblklist "$vm" --details 2>/dev/null | awk 'NR > 2 && NF >= 4 { source=$4; for (i=5; i<=NF; i++) source=source " " $i; printf "VMRELAY_DISK\t%%s\t%%s\t%%s\t%%s\n", $1, $2, $3, source }'
virsh -c qemu:///system domiflist "$vm" 2>/dev/null | awk 'NR > 2 && NF >= 5 { printf "VMRELAY_NIC\t%%s\t%%s\t%%s\t%%s\t%%s\n", $1, $2, $3, $4, $5 }'
`, shellQuote(vm.Name))
	out, err := ssh(h.Target, script, 45*time.Second)
	if err != nil {
		return VMDetail{}, out, err
	}
	detail := parseVMDetailOutput(out)
	if detail.VM.Name == "" {
		detail.VM = vm
	}
	return detail, out, nil
}

func parseVMDetailOutput(out string) VMDetail {
	var detail VMDetail
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		switch parts[0] {
		case "VMRELAY_DETAIL":
			for len(parts) < 10 {
				parts = append(parts, "")
			}
			detail.VM = VM{
				Name:   parts[1],
				UUID:   parts[2],
				State:  parts[3],
				Owner:  parts[4],
				Shared: parts[5] == "1" || strings.EqualFold(parts[5], "true"),
			}
			detail.Autostart = parts[6]
			detail.CPUs = parts[7]
			detail.Memory = parts[8]
			detail.Graphics = parts[9]
		case "VMRELAY_DISK":
			for len(parts) < 5 {
				parts = append(parts, "")
			}
			detail.Disks = append(detail.Disks, VMDisk{Type: parts[1], Device: parts[2], Target: parts[3], Source: parts[4]})
		case "VMRELAY_NIC":
			for len(parts) < 6 {
				parts = append(parts, "")
			}
			detail.NICs = append(detail.NICs, VMNIC{Interface: parts[1], Type: parts[2], Source: parts[3], Model: parts[4], MAC: parts[5]})
		}
	}
	return detail
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

func createVM(h Host, req vmCreateRequest) (string, error) {
	sharedValue := "0"
	if req.Shared {
		sharedValue = "1"
	}
	script := fmt.Sprintf(`
set -euo pipefail
name=%s
memory=%d
cpus=%d
disk_size=%d
disk_bus=%s
iso=%s
network=%s
firmware=%s
shared=%s

command -v virt-install >/dev/null 2>&1 || { echo "virt-install is missing; run setup for this host." >&2; exit 1; }
command -v qemu-img >/dev/null 2>&1 || { echo "qemu-img is missing; run setup for this host." >&2; exit 1; }
virsh -c qemu:///system dominfo "$name" >/dev/null 2>&1 && { echo "VM already exists: $name" >&2; exit 1; }
virsh -c qemu:///system net-info "$network" >/dev/null 2>&1 || { echo "Libvirt network not found: $network" >&2; exit 1; }
case "$iso" in /*) ;; *) echo "ISO path must be absolute: $iso" >&2; exit 1 ;; esac
[ -e "$iso" ] || { echo "ISO path does not exist: $iso" >&2; exit 1; }
if [ "$firmware" = "uefi" ]; then
  if [ ! -d /usr/share/OVMF ] && [ ! -d /usr/share/ovmf ] && [ ! -e /usr/share/qemu/OVMF.fd ]; then
    echo "UEFI firmware is missing; run setup or install ovmf on the host." >&2
    exit 1
  fi
fi

safe="$(printf '%%s' "$name" | tr -c 'A-Za-z0-9_.-' '_')"
disk="/var/lib/libvirt/images/${safe}.qcow2"
if [ -e "$disk" ]; then echo "Disk already exists: $disk" >&2; exit 1; fi
sudo -n install -d -m 0775 /var/lib/libvirt/images
sudo -n qemu-img create -f qcow2 "$disk" "${disk_size}G"
sudo -n chown libvirt-qemu:kvm "$disk" 2>/dev/null || sudo -n chown qemu:qemu "$disk" 2>/dev/null || true
sudo -n chmod 0660 "$disk" 2>/dev/null || true

args=(
  --connect qemu:///system
  --name "$name"
  --memory "$memory"
  --vcpus "$cpus"
	  --disk "path=${disk},format=qcow2,bus=${disk_bus},cache=none"
	  --network "network=${network},model=virtio"
	  --graphics vnc,listen=127.0.0.1
	  --input type=tablet,bus=usb
	  --video virtio
	  --cdrom "$iso"
  --os-variant detect=on,require=off
  --noautoconsole
  --wait 0
)
if [ "$firmware" = "uefi" ]; then
  args+=(--boot uefi)
fi
if ! virt-install "${args[@]}"; then
  sudo -n rm -f "$disk" 2>/dev/null || true
  exit 1
fi

uuid="$(virsh -c qemu:///system domuuid "$name")"
policy=/var/lib/vmrelay/ownership.tsv
[ -e "$policy" ] || sudo -n touch "$policy"
tmp="$(mktemp)"
if [ -r "$policy" ]; then awk -F '\t' -v id="$uuid" '$1 != id { print }' "$policy" >"$tmp"; fi
printf '%%s\t%%s\t%%s\t%%s\n' "$uuid" "$(whoami)" "$shared" '' >>"$tmp"
if [ -w "$policy" ]; then
  cat "$tmp" >"$policy"
else
  sudo -n cp "$tmp" "$policy"
  sudo -n chmod 0664 "$policy"
fi
rm -f "$tmp"
echo "Created VM ${name}. Open its console to complete the OS installer."
`, shellQuote(req.Name), req.MemoryMiB, req.CPUs, req.DiskGiB, shellQuote(req.DiskBus), shellQuote(req.ISO), shellQuote(req.Network), shellQuote(req.Firmware), shellQuote(sharedValue))
	return ssh(h.Target, script, 10*time.Minute)
}

func createAndAttachDisk(h Host, vmName string, req diskCreateRequest) (string, error) {
	script := fmt.Sprintf(`
set -euo pipefail
vm=%s
size=%d
path=%s
target=%s
next_target() {
  existing="$(virsh -c qemu:///system domblklist "$vm" --details 2>/dev/null | awk 'NR > 2 { print $3 }')"
  for letter in b c d e f g h i j k l m n o p q r s t u v w x y z; do
    candidate="vd${letter}"
    if ! printf '%%s\n' "$existing" | grep -qx "$candidate"; then
      printf '%%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}
if [ -z "$target" ]; then target="$(next_target)"; fi
if [ -z "$target" ]; then echo "No free virtio disk target is available." >&2; exit 1; fi
if [ -z "$path" ]; then
  safe="$(printf '%%s' "$vm" | tr -c 'A-Za-z0-9_.-' '_')"
  path="/var/lib/libvirt/images/${safe}-${target}.qcow2"
fi
case "$path" in /*) ;; *) echo "Disk path must be absolute: $path" >&2; exit 1 ;; esac
if [ -e "$path" ]; then echo "Disk already exists: $path" >&2; exit 1; fi
sudo -n install -d -m 0775 "$(dirname "$path")"
sudo -n qemu-img create -f qcow2 "$path" "${size}G"
sudo -n chown libvirt-qemu:kvm "$path" 2>/dev/null || sudo -n chown qemu:qemu "$path" 2>/dev/null || true
sudo -n chmod 0660 "$path" 2>/dev/null || true
flags="--config"
if virsh -c qemu:///system domstate "$vm" 2>/dev/null | grep -qi '^running'; then flags="--live --config"; fi
virsh -c qemu:///system attach-disk "$vm" "$path" "$target" --targetbus virtio --subdriver qcow2 --cache none $flags
echo "Created and attached ${path} as ${target}."
`, shellQuote(vmName), req.SizeGiB, shellQuote(req.Path), shellQuote(req.Target))
	return ssh(h.Target, script, 3*time.Minute)
}

func importAndAttachDisk(h Host, vmName string, req diskImportRequest) (string, error) {
	script := fmt.Sprintf(`
set -euo pipefail
vm=%s
source=%s
dest=%s
target=%s
next_target() {
  existing="$(virsh -c qemu:///system domblklist "$vm" --details 2>/dev/null | awk 'NR > 2 { print $3 }')"
  for letter in b c d e f g h i j k l m n o p q r s t u v w x y z; do
    candidate="vd${letter}"
    if ! printf '%%s\n' "$existing" | grep -qx "$candidate"; then
      printf '%%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}
case "$source" in /*) ;; *) echo "Source path must be absolute: $source" >&2; exit 1 ;; esac
[ -e "$source" ] || { echo "Source disk does not exist: $source" >&2; exit 1; }
if [ -z "$target" ]; then target="$(next_target)"; fi
if [ -z "$target" ]; then echo "No free virtio disk target is available." >&2; exit 1; fi
if [ -z "$dest" ]; then
  safe="$(printf '%%s' "$vm" | tr -c 'A-Za-z0-9_.-' '_')"
  dest="/var/lib/libvirt/images/${safe}-${target}.qcow2"
fi
case "$dest" in /*) ;; *) echo "Destination path must be absolute: $dest" >&2; exit 1 ;; esac
format="$(qemu-img info "$source" 2>/dev/null | awk -F: '$1 == "file format" { gsub(/^[[:space:]]+/, "", $2); print $2; exit }')"
[ -n "$format" ] || { echo "Could not detect source format with qemu-img: $source" >&2; exit 1; }
if [ "$format" = "qcow2" ] && [ "$source" = "$dest" ]; then
  echo "Using existing qcow2 source without conversion."
else
  if [ -e "$dest" ]; then echo "Destination already exists: $dest" >&2; exit 1; fi
  sudo -n install -d -m 0775 "$(dirname "$dest")"
  if [ "$format" = "qcow2" ]; then
    sudo -n cp --reflink=auto "$source" "$dest" 2>/dev/null || sudo -n cp "$source" "$dest"
    echo "Copied qcow2 source to $dest."
  else
    sudo -n qemu-img convert -p -f "$format" -O qcow2 "$source" "$dest"
    echo "Converted $format source to qcow2 at $dest."
  fi
fi
sudo -n chown libvirt-qemu:kvm "$dest" 2>/dev/null || sudo -n chown qemu:qemu "$dest" 2>/dev/null || true
sudo -n chmod 0660 "$dest" 2>/dev/null || true
flags="--config"
if virsh -c qemu:///system domstate "$vm" 2>/dev/null | grep -qi '^running'; then flags="--live --config"; fi
virsh -c qemu:///system attach-disk "$vm" "$dest" "$target" --targetbus virtio --subdriver qcow2 --cache none $flags
echo "Imported and attached ${dest} as ${target}."
`, shellQuote(vmName), shellQuote(req.Source), shellQuote(req.Dest), shellQuote(req.Target))
	return ssh(h.Target, script, 2*time.Hour)
}

func detachDisk(h Host, vmName string, disk VMDisk) (string, error) {
	if disk.Target == "" {
		return "", fmt.Errorf("disk target is missing")
	}
	script := fmt.Sprintf(`
set -euo pipefail
vm=%s
target=%s
flags="--config"
if virsh -c qemu:///system domstate "$vm" 2>/dev/null | grep -qi '^running'; then flags="--live --config"; fi
virsh -c qemu:///system detach-disk "$vm" "$target" $flags
echo "Detached disk ${target}. Disk image was not deleted."
`, shellQuote(vmName), shellQuote(disk.Target))
	return ssh(h.Target, script, 45*time.Second)
}

func setBootDisk(h Host, vmName string, disk VMDisk) (string, error) {
	if disk.Target == "" {
		return "", fmt.Errorf("disk target is missing")
	}
	script := fmt.Sprintf(`
set -euo pipefail
vm=%s
target=%s
command -v python3 >/dev/null 2>&1 || { echo "python3 is required on the host to update VM boot order." >&2; exit 1; }
tmp="$(mktemp)"
cleanup() { rm -f "$tmp"; }
trap cleanup EXIT
virsh -c qemu:///system dumpxml "$vm" >"$tmp"
python3 - "$tmp" "$target" <<'PY'
import sys
import xml.etree.ElementTree as ET

path, target = sys.argv[1], sys.argv[2]
tree = ET.parse(path)
root = tree.getroot()

os_el = root.find("os")
if os_el is not None:
    for boot in list(os_el.findall("boot")):
        os_el.remove(boot)

devices = root.find("devices")
found = False
if devices is not None:
    for dev in list(devices):
        for boot in list(dev.findall("boot")):
            dev.remove(boot)
    for disk in devices.findall("disk"):
        target_el = disk.find("target")
        if disk.get("device") == "disk" and target_el is not None and target_el.get("dev") == target:
            ET.SubElement(disk, "boot", {"order": "1"})
            found = True
            break

if not found:
    sys.stderr.write(f"Disk target not found in VM XML: {target}\n")
    sys.exit(2)

tree.write(path, encoding="unicode")
PY
virsh -c qemu:///system define "$tmp" >/dev/null
state="$(virsh -c qemu:///system domstate "$vm" 2>/dev/null || true)"
echo "Set ${target} as the first boot disk for ${vm}."
case "$state" in
  running*) echo "Power off and start the VM for the new boot order to take effect." ;;
esac
`, shellQuote(vmName), shellQuote(disk.Target))
	return ssh(h.Target, script, 45*time.Second)
}

func attachNIC(h Host, vmName string, req nicAddRequest) (string, error) {
	script := fmt.Sprintf(`
set -euo pipefail
vm=%s
source=%s
model=%s
flags="--config"
if virsh -c qemu:///system domstate "$vm" 2>/dev/null | grep -qi '^running'; then flags="--live --config"; fi
virsh -c qemu:///system attach-interface "$vm" --type network --source "$source" --model "$model" $flags
echo "Attached ${model} NIC on network ${source}."
`, shellQuote(vmName), shellQuote(req.Source), shellQuote(req.Model))
	return ssh(h.Target, script, 45*time.Second)
}

func detachNIC(h Host, vmName string, nic VMNIC) (string, error) {
	if nic.MAC == "" {
		return "", fmt.Errorf("NIC MAC address is missing")
	}
	nicType := nic.Type
	if nicType == "" {
		nicType = "network"
	}
	script := fmt.Sprintf(`
set -euo pipefail
vm=%s
nic_type=%s
mac=%s
flags="--config"
if virsh -c qemu:///system domstate "$vm" 2>/dev/null | grep -qi '^running'; then flags="--live --config"; fi
virsh -c qemu:///system detach-interface "$vm" --type "$nic_type" --mac "$mac" $flags
echo "Detached NIC ${mac}."
`, shellQuote(vmName), shellQuote(nicType), shellQuote(nic.MAC))
	return ssh(h.Target, script, 45*time.Second)
}

func openConsole(h Host, vmName, stateDir string) (string, error) {
	preferredLocalPort := stablePort("local:"+h.Name+":"+vmName, 4500, 1000)
	localPort, adjusted := firstFreePort(preferredLocalPort, 100)
	if localPort == 0 {
		return "", fmt.Errorf("no free local console port found near %d", preferredLocalPort)
	}
	remotePort := stablePort("remote:"+h.Name+":"+vmName, 6080, 1000)

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
	consoleURL := noVNCURL(localPort)
	opened := openBrowser(consoleURL)
	if opened {
		out += "\nConsole URL: " + consoleURL + "\nBrowser: requested local console URL"
	} else {
		out += "\nConsole URL: " + consoleURL
	}
	if adjusted {
		out += fmt.Sprintf("\nLocal port %d was busy; using %d instead.", preferredLocalPort, localPort)
	}
	return strings.TrimSpace(out), nil
}

func noVNCURL(localPort int) string {
	values := url.Values{}
	values.Set("autoconnect", "1")
	values.Set("resize", "scale")
	values.Set("show_dot", "1")
	values.Set("quality", "9")
	values.Set("compression", "0")
	return fmt.Sprintf("http://127.0.0.1:%d/vnc.html?%s", localPort, values.Encode())
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

func firstFreePort(preferred, attempts int) (int, bool) {
	if preferred < 1 || preferred > 65535 || attempts < 1 {
		return 0, false
	}
	for i := 0; i < attempts && preferred+i <= 65535; i++ {
		port := preferred + i
		if portFree(port) {
			return port, i != 0
		}
	}
	return 0, false
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
