package app

import (
	"net"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func TestParseVMListOutputIgnoresRemoteDiagnostics(t *testing.T) {
	out := "bash: -c: option requires an argument\n" +
		"VMRELAY_VM\tDraytek_VPN_virtualisation_server\tabc-123\trunning\talice\t1\n" +
		"warning: ignored diagnostic\n"

	vms := parseVMListOutput(out)
	if len(vms) != 1 {
		t.Fatalf("expected 1 VM, got %d: %#v", len(vms), vms)
	}
	if vms[0].Name != "Draytek_VPN_virtualisation_server" {
		t.Fatalf("unexpected VM name: %q", vms[0].Name)
	}
	if vms[0].UUID != "abc-123" {
		t.Fatalf("unexpected VM UUID: %q", vms[0].UUID)
	}
	if vms[0].State != "running" {
		t.Fatalf("unexpected VM state: %q", vms[0].State)
	}
	if vms[0].Owner != "alice" {
		t.Fatalf("unexpected VM owner: %q", vms[0].Owner)
	}
	if !vms[0].Shared {
		t.Fatal("expected VM to be shared")
	}
}

func TestViewFrameFillsWindow(t *testing.T) {
	m := Model{version: "0.2.3", config: Config{Theme: "Classic"}, width: 80, height: 20, mode: modeHosts, status: "Ready."}
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 20 {
		t.Fatalf("expected 20 rendered lines, got %d", len(lines))
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w != 80 {
			t.Fatalf("line %d width = %d, want 80: %q", i, w, line)
		}
	}
	if !strings.Contains(stripANSI(lines[0]), "VMRelay 0.2.3") {
		t.Fatalf("top border missing title/version: %q", lines[0])
	}
	if !strings.Contains(stripANSI(lines[len(lines)-2]), "?: help  m: themes") {
		t.Fatalf("footer was not anchored above the bottom border: %q", lines[len(lines)-2])
	}
	if !strings.Contains(stripANSI(lines[len(lines)-3]), "Ready.") {
		t.Fatalf("status was not anchored above the footer: %q", lines[len(lines)-3])
	}
	plain := stripANSI(view)
	if strings.Count(plain, "╭") != 1 || strings.Count(plain, "╰") != 1 {
		t.Fatalf("expected only the outer rounded border, got:\n%s", plain)
	}
	if strings.Contains(plain, "Theme:") {
		t.Fatalf("theme control should live in the footer, not the hosts table:\n%s", plain)
	}
}

func TestThemeCatalogHasTenThemes(t *testing.T) {
	if len(themes) != 10 {
		t.Fatalf("expected 10 themes, got %d", len(themes))
	}
}

func TestVersionGreater(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{latest: "0.2.4", current: "0.2.3", want: true},
		{latest: "v0.2.4", current: "0.2.3", want: true},
		{latest: "0.2.3", current: "0.2.3", want: false},
		{latest: "0.2.3", current: "0.2.4", want: false},
		{latest: "0.10.0", current: "0.9.9", want: true},
	}
	for _, test := range tests {
		if got := versionGreater(test.latest, test.current); got != test.want {
			t.Fatalf("versionGreater(%q, %q) = %v, want %v", test.latest, test.current, got, test.want)
		}
	}
}

func TestUpdatePromptRendersAvailableVersion(t *testing.T) {
	m := Model{
		version:    "0.2.3",
		config:     Config{Theme: "Classic"},
		width:      80,
		height:     20,
		mode:       modeUpdate,
		updateInfo: updateInfo{Latest: "0.2.4", URL: "https://github.com/brontoguana/vmrelay/releases/tag/v0.2.4"},
		status:     "Update available: 0.2.4.",
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Update Available") {
		t.Fatalf("update prompt missing title:\n%s", view)
	}
	if !strings.Contains(view, "Installed: 0.2.3") || !strings.Contains(view, "Available: 0.2.4") {
		t.Fatalf("update prompt missing version details:\n%s", view)
	}
	if !strings.Contains(view, "enter/y: update and restart") {
		t.Fatalf("update prompt missing footer help:\n%s", view)
	}
}

func TestVMListFailureNamesHost(t *testing.T) {
	text := failureText(resultMsg{op: "vms", err: errTest("exit status 1")}, Model{activeHost: Host{Name: "iron"}})
	if text != "Failed to open iron: exit status 1" {
		t.Fatalf("unexpected failure text: %q", text)
	}
}

func TestVMRowsKeepOwnerAndVisibilityAligned(t *testing.T) {
	m := Model{
		config:     Config{Theme: "Classic"},
		mode:       modeVMs,
		activeHost: Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"},
		vms: []VM{
			{Name: "Draytek_VPN_virtualisation_server", State: "running", Owner: "", Shared: false},
			{Name: "A-very-long-virtual-machine-name-that-should-not-shift-columns", State: "shut off", Owner: "alice", Shared: true},
		},
	}
	view := stripANSI(m.viewVMs(98, 18))
	lines := strings.Split(view, "\n")
	var header string
	var rows []string
	for _, line := range lines {
		if strings.Contains(line, "VM ") && strings.Contains(line, "Visibility") {
			header = line
		}
		if strings.Contains(line, "running") || strings.Contains(line, "shut off") {
			rows = append(rows, line)
		}
	}
	if header == "" || len(rows) != 2 {
		t.Fatalf("failed to find VM header/rows in:\n%s", view)
	}
	ownerCol := strings.Index(header, "Owner")
	visibilityCol := strings.Index(header, "Visibility")
	if ownerCol < 0 || visibilityCol < 0 {
		t.Fatalf("missing columns in header: %q", header)
	}
	for _, row := range rows {
		if ownerCol >= len(row) || visibilityCol >= len(row) {
			t.Fatalf("row shorter than expected columns: %q", row)
		}
		ownerText := strings.TrimSpace(row[ownerCol:visibilityCol])
		if ownerText != "unmanaged" && ownerText != "alice" {
			t.Fatalf("owner column shifted in row %q; got owner field %q", row, ownerText)
		}
		visibilityText := strings.TrimSpace(row[visibilityCol:])
		if !strings.HasPrefix(visibilityText, "private") && !strings.HasPrefix(visibilityText, "shared") {
			t.Fatalf("visibility column shifted in row %q; got visibility field %q", row, visibilityText)
		}
	}
}

func TestHostDetailRendersMappings(t *testing.T) {
	m := Model{
		config: Config{
			Theme: "Classic",
			Mappings: []PortMapping{
				{ID: "map1", Host: "iron", Name: "Web", LocalPort: 8080, RemoteHost: "127.0.0.1", RemotePort: 8081},
			},
		},
		mode:       modeVMs,
		hostTab:    hostTabMappings,
		activeHost: Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"},
		stateDir:   t.TempDir(),
	}
	view := stripANSI(m.viewHostDetail(100, 20))
	if !strings.Contains(view, "Mappings") || !strings.Contains(view, "Web") {
		t.Fatalf("mapping tab did not render configured mapping:\n%s", view)
	}
	if !strings.Contains(view, "127.0.0.1:8080") || !strings.Contains(view, "127.0.0.1:8081") {
		t.Fatalf("mapping endpoints missing:\n%s", view)
	}
}

func TestParseVMDetailOutput(t *testing.T) {
	out := strings.Join([]string{
		"VMRELAY_DETAIL\tvm1\tuuid-1\trunning\talice\t1\tenable\t4\t8388608 KiB\tvnc://127.0.0.1:2",
		"VMRELAY_DISK\tfile\tdisk\tvda\t/var/lib/libvirt/images/vm1.qcow2",
		"VMRELAY_NIC\tvnet0\tnetwork\tdefault\tvirtio\t52:54:00:12:34:56",
	}, "\n")
	detail := parseVMDetailOutput(out)
	if detail.VM.Name != "vm1" || detail.VM.UUID != "uuid-1" || !detail.VM.Shared {
		t.Fatalf("unexpected detail VM: %#v", detail.VM)
	}
	if detail.Autostart != "enable" || detail.CPUs != "4" || detail.Graphics == "" {
		t.Fatalf("missing detail fields: %#v", detail)
	}
	if len(detail.Disks) != 1 || detail.Disks[0].Target != "vda" || detail.Disks[0].Source != "/var/lib/libvirt/images/vm1.qcow2" {
		t.Fatalf("unexpected disks: %#v", detail.Disks)
	}
	if len(detail.NICs) != 1 || detail.NICs[0].MAC != "52:54:00:12:34:56" || detail.NICs[0].Source != "default" {
		t.Fatalf("unexpected NICs: %#v", detail.NICs)
	}
}

func TestVMDetailRendersDiskAndNICTabs(t *testing.T) {
	m := Model{
		config:     Config{Theme: "Classic"},
		mode:       modeVMDetail,
		activeHost: Host{Name: "iron"},
		vmDetail: VMDetail{
			VM: VM{Name: "vm1", State: "running", Owner: "alice"},
			Disks: []VMDisk{
				{Type: "file", Device: "disk", Target: "vda", Source: "/var/lib/libvirt/images/vm1.qcow2"},
			},
			NICs: []VMNIC{
				{Interface: "vnet0", Type: "network", Source: "default", Model: "virtio", MAC: "52:54:00:12:34:56"},
			},
		},
	}
	m.vmTab = vmTabDisks
	disks := stripANSI(m.viewVMDetail(100, 20))
	if !strings.Contains(disks, "Disks") || !strings.Contains(disks, "vda") || !strings.Contains(disks, "vm1.qcow2") {
		t.Fatalf("disk tab did not render disk detail:\n%s", disks)
	}
	if help := m.helpText(); !strings.Contains(help, "enter: boot disk") {
		t.Fatalf("disk tab help should expose boot disk action: %q", help)
	}
	m.vmTab = vmTabNICs
	nics := stripANSI(m.viewVMDetail(100, 20))
	if !strings.Contains(nics, "NICs") || !strings.Contains(nics, "52:54:00:12:34:56") || !strings.Contains(nics, "default") {
		t.Fatalf("NIC tab did not render NIC detail:\n%s", nics)
	}
}

func TestPendingMappingValidation(t *testing.T) {
	m := Model{
		activeHost:       Host{Name: "iron"},
		addMapName:       "HTTP",
		addMapLocalPort:  "8080",
		addMapRemoteHost: "127.0.0.1",
		addMapRemotePort: "8081",
	}
	mapping, err := m.pendingMapping()
	if err != nil {
		t.Fatalf("pendingMapping returned error: %v", err)
	}
	if mapping.Host != "iron" || mapping.LocalPort != 8080 || mapping.RemotePort != 8081 {
		t.Fatalf("unexpected mapping: %#v", mapping)
	}
}

func TestCreateVMFormAndValidation(t *testing.T) {
	m := Model{
		config:           Config{Theme: "Classic"},
		mode:             modeCreateVM,
		activeHost:       Host{Name: "iron"},
		createVMName:     "win10",
		createVMMemory:   "4",
		createVMCPUs:     "2",
		createVMDiskSize: "64",
		createVMDiskBus:  "sata",
		createVMISO:      "/var/lib/libvirt/boot/windows.iso",
		createVMNetwork:  "default",
		createVMFirmware: "uefi",
		createVMShared:   "n",
	}
	view := stripANSI(m.viewCreateVM(100, 20))
	if !strings.Contains(view, "Create VM on iron") || !strings.Contains(view, "windows.iso") || !strings.Contains(view, "Disk bus") || !strings.Contains(view, "Firmware") {
		t.Fatalf("create VM form missing expected content:\n%s", view)
	}
	req, err := m.pendingVMCreate()
	if err != nil {
		t.Fatalf("pendingVMCreate returned error: %v", err)
	}
	if req.Name != "win10" || req.MemoryMiB != 4096 || req.CPUs != 2 || req.DiskGiB != 64 || req.DiskBus != "sata" || req.Firmware != "uefi" || req.Shared {
		t.Fatalf("unexpected create request: %#v", req)
	}

	m.createVMISO = "relative.iso"
	if _, err := m.pendingVMCreate(); err == nil {
		t.Fatal("expected relative ISO path to fail")
	}
	m.createVMISO = "/var/lib/libvirt/boot/windows.iso"
	m.createVMFirmware = "coreboot"
	if _, err := m.pendingVMCreate(); err == nil {
		t.Fatal("expected unsupported firmware to fail")
	}
}

func TestNoVNCURLUsesLowLatencyCursorSettings(t *testing.T) {
	rawURL := noVNCURL(4523)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("failed to parse noVNC URL: %v", err)
	}
	if parsed.Scheme != "http" || parsed.Host != "127.0.0.1:4523" || parsed.Path != "/vnc.html" {
		t.Fatalf("unexpected noVNC URL: %s", rawURL)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"autoconnect": "1",
		"resize":      "scale",
		"show_dot":    "1",
		"quality":     "9",
		"compression": "0",
	} {
		if got := query.Get(key); got != want {
			t.Fatalf("query %s = %q, want %q in %s", key, got, want, rawURL)
		}
	}
}

func TestPendingDiskImportValidation(t *testing.T) {
	m := Model{
		importDiskSource: "/home/alice/source.vmdk",
		importDiskDest:   "/var/lib/libvirt/images/imported.qcow2",
		importDiskTarget: "vdb",
	}
	req, err := m.pendingDiskImport()
	if err != nil {
		t.Fatalf("pendingDiskImport returned error: %v", err)
	}
	if req.Source != "/home/alice/source.vmdk" || req.Dest != "/var/lib/libvirt/images/imported.qcow2" || req.Target != "vdb" {
		t.Fatalf("unexpected import request: %#v", req)
	}

	m.importDiskSource = "relative.vmdk"
	if _, err := m.pendingDiskImport(); err == nil {
		t.Fatal("expected relative source path to fail")
	}
}

func TestFirstFreePortSkipsBusyPreferredPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a local port: %v", err)
	}
	defer ln.Close()

	preferred := ln.Addr().(*net.TCPAddr).Port
	got, adjusted := firstFreePort(preferred, 100)
	if got == 0 {
		t.Fatalf("expected fallback port near %d", preferred)
	}
	if got == preferred {
		t.Fatalf("expected busy preferred port %d to be skipped", preferred)
	}
	if !adjusted {
		t.Fatalf("expected adjusted=true when preferred port is busy")
	}
	if !portFree(got) {
		t.Fatalf("fallback port %d should be free", got)
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}
