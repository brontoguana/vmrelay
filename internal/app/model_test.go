package app

import (
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	if !strings.HasPrefix(stripANSI(lines[len(lines)-3]), "│ Ready.") {
		t.Fatalf("status should have one space after the left border: %q", lines[len(lines)-3])
	}
	if !strings.HasPrefix(stripANSI(lines[len(lines)-2]), "│ ?: help") {
		t.Fatalf("footer should have one space after the left border: %q", lines[len(lines)-2])
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
	if !strings.Contains(view, "enter/y: update in terminal") {
		t.Fatalf("update prompt missing footer help:\n%s", view)
	}
}

func TestUpdatePromptQuitsForTerminalInstaller(t *testing.T) {
	m := Model{
		version:    "0.2.15",
		config:     Config{Theme: "Classic"},
		mode:       modeUpdate,
		updateInfo: updateInfo{Latest: "0.2.16"},
	}
	updated, cmd := m.updateUpdateKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("accepting update should quit the TUI for terminal installer handoff")
	}
	next := updated.(Model)
	if !next.UpdateRequested() {
		t.Fatal("accepting update should mark update requested")
	}

	m.updateExit = false
	updated, _ = m.updateUpdateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	next = updated.(Model)
	if next.UpdateRequested() {
		t.Fatal("skipping update should not mark update requested")
	}
}

func TestVMListFailureNamesHost(t *testing.T) {
	text := failureText(resultMsg{op: "vms", err: errTest("exit status 1")}, Model{activeHost: Host{Name: "iron"}})
	if text != "Failed to open iron: exit status 1" {
		t.Fatalf("unexpected failure text: %q", text)
	}
}

func TestVMRefreshTickStartsSilentInventoryLoad(t *testing.T) {
	m := Model{
		mode:       modeVMs,
		hostTab:    hostTabVMs,
		activeHost: Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"},
	}
	updated, cmd := m.updateVMRefreshTick()
	next := updated.(Model)
	if !next.vmRefreshInFlight {
		t.Fatal("VM refresh tick should mark background refresh in flight")
	}
	if cmd == nil {
		t.Fatal("VM refresh tick should return a command")
	}

	m = Model{
		mode:       modeVMDetail,
		activeHost: Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"},
		vmDetail:   VMDetail{VM: VM{Name: "vm1"}},
	}
	updated, cmd = m.updateVMRefreshTick()
	next = updated.(Model)
	if !next.vmRefreshInFlight {
		t.Fatal("VM detail refresh tick should mark background refresh in flight")
	}
	if cmd == nil {
		t.Fatal("VM detail refresh tick should return a command")
	}

	m.mode = modeHosts
	m.vmRefreshInFlight = false
	updated, _ = m.updateVMRefreshTick()
	next = updated.(Model)
	if next.vmRefreshInFlight {
		t.Fatal("VM refresh tick should not start a load outside VM screens")
	}
}

func TestAutoVMRefreshUpdatesRowsWithoutChangingStatus(t *testing.T) {
	host := Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"}
	m := Model{
		mode:              modeVMs,
		hostTab:           hostTabVMs,
		activeHost:        host,
		status:            "User-facing status stays put.",
		vmRefreshInFlight: true,
		vms:               []VM{{Name: "old", State: "running"}},
	}
	updated, cmd := m.updateResult(resultMsg{
		op:   "vms-auto",
		host: host,
		vms:  []VM{{Name: "new", State: "shut off"}},
	})
	if cmd != nil {
		t.Fatal("auto VM refresh result should not trigger another command")
	}
	next := updated.(Model)
	if next.vmRefreshInFlight {
		t.Fatal("auto VM refresh result should clear in-flight state")
	}
	if next.status != m.status {
		t.Fatalf("auto VM refresh changed status: %q", next.status)
	}
	if len(next.vms) != 1 || next.vms[0].Name != "new" || next.vms[0].State != "shut off" {
		t.Fatalf("auto VM refresh did not replace VM rows: %#v", next.vms)
	}

	stale := resultMsg{op: "vms-auto", host: Host{Name: "other", Target: "other"}, vms: []VM{{Name: "wrong"}}}
	updated, _ = next.updateResult(stale)
	next = updated.(Model)
	if len(next.vms) != 1 || next.vms[0].Name != "new" {
		t.Fatalf("stale auto refresh should be ignored: %#v", next.vms)
	}
}

func TestAutoVMDetailRefreshUpdatesDetailWithoutChangingStatus(t *testing.T) {
	host := Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"}
	m := Model{
		mode:              modeVMDetail,
		activeHost:        host,
		status:            "User-facing status stays put.",
		vmRefreshInFlight: true,
		vmDetail:          VMDetail{VM: VM{Name: "old", State: "running"}},
	}
	updated, cmd := m.updateResult(resultMsg{
		op:     "vm-detail-auto",
		host:   host,
		detail: VMDetail{VM: VM{Name: "old", State: "shut off"}},
	})
	if cmd != nil {
		t.Fatal("auto VM detail refresh result should not trigger another command")
	}
	next := updated.(Model)
	if next.vmRefreshInFlight {
		t.Fatal("auto VM detail refresh result should clear in-flight state")
	}
	if next.status != m.status {
		t.Fatalf("auto VM detail refresh changed status: %q", next.status)
	}
	if next.vmDetail.VM.State != "shut off" {
		t.Fatalf("auto VM detail refresh did not replace detail: %#v", next.vmDetail)
	}
}

func TestManualVMListRefreshStaysOnTable(t *testing.T) {
	host := Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"}
	m := Model{
		mode:       modeVMs,
		hostTab:    hostTabVMs,
		activeHost: host,
		vms:        []VM{{Name: "old", State: "running"}},
	}
	updated, cmd := m.updateVMKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("manual VM refresh should start a background command")
	}
	next := updated.(Model)
	if next.mode != modeVMs {
		t.Fatalf("manual VM refresh should keep VM list visible, got mode %v", next.mode)
	}
	if !next.vmRefreshInFlight {
		t.Fatal("manual VM refresh should mark refresh in flight")
	}

	updated, cmd = next.updateResult(resultMsg{
		op:   "vms-refresh",
		host: host,
		vms:  []VM{{Name: "new", State: "shut off"}},
	})
	if cmd != nil {
		t.Fatal("manual VM refresh result should not trigger another command")
	}
	next = updated.(Model)
	if next.vmRefreshInFlight {
		t.Fatal("manual VM refresh result should clear in-flight state")
	}
	if next.mode != modeVMs {
		t.Fatalf("manual VM refresh result should keep VM list visible, got mode %v", next.mode)
	}
	if len(next.vms) != 1 || next.vms[0].Name != "new" {
		t.Fatalf("manual VM refresh did not update table rows: %#v", next.vms)
	}
	if !strings.Contains(next.status, "Refreshed 1 VMs") {
		t.Fatalf("manual VM refresh should report completion in place, got %q", next.status)
	}
}

func TestManualVMDetailRefreshStaysOnDetail(t *testing.T) {
	host := Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"}
	vm := VM{Name: "vm1", State: "running"}
	m := Model{
		mode:       modeVMDetail,
		activeHost: host,
		vmDetail:   VMDetail{VM: vm},
	}
	updated, cmd := m.updateVMDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("manual VM detail refresh should start a background command")
	}
	next := updated.(Model)
	if next.mode != modeVMDetail {
		t.Fatalf("manual VM detail refresh should keep detail visible, got mode %v", next.mode)
	}
	if !next.vmRefreshInFlight {
		t.Fatal("manual VM detail refresh should mark refresh in flight")
	}

	updated, cmd = next.updateResult(resultMsg{
		op:     "vm-detail-refresh",
		host:   host,
		detail: VMDetail{VM: VM{Name: "vm1", State: "shut off"}},
	})
	if cmd != nil {
		t.Fatal("manual VM detail refresh result should not trigger another command")
	}
	next = updated.(Model)
	if next.vmRefreshInFlight {
		t.Fatal("manual VM detail refresh result should clear in-flight state")
	}
	if next.mode != modeVMDetail {
		t.Fatalf("manual VM detail refresh result should keep detail visible, got mode %v", next.mode)
	}
	if next.vmDetail.VM.State != "shut off" {
		t.Fatalf("manual VM detail refresh did not update detail: %#v", next.vmDetail)
	}
	if !strings.Contains(next.status, "Refreshed vm1") {
		t.Fatalf("manual VM detail refresh should report completion in place, got %q", next.status)
	}
}

func TestVMRefreshCanPreserveActionStatus(t *testing.T) {
	m := Model{
		mode:       modeBusy,
		priorMode:  modeVMs,
		activeHost: Host{Name: "iron"},
		status:     "Loading VMs from iron...",
	}
	updated, _ := m.updateResult(resultMsg{
		op:     "vms",
		status: "Shutdown requested for vm1.",
		vms:    []VM{{Name: "vm1", State: "running"}},
	})
	next := updated.(Model)
	if !strings.Contains(next.status, "Shutdown requested") {
		t.Fatalf("VM refresh should preserve action status, got %q", next.status)
	}
}

func TestShutdownLifecycleScriptSendsACPIAndReturnsPromptly(t *testing.T) {
	script := lifecycleScript("vm1", "shutdown")
	for _, want := range []string{
		"shutdown \"$vm\" --mode acpi",
		"domstate \"$vm\"",
		"Shutdown requested for $vm.",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("shutdown script missing %q:\n%s", want, script)
		}
	}
	for _, old := range []string{"after 30s", "use force off", "deadline="} {
		if strings.Contains(script, old) {
			t.Fatalf("shutdown script should return after requesting shutdown, found %q:\n%s", old, script)
		}
	}
	startScript := lifecycleScript("vm1", "start")
	if strings.Contains(startScript, "--mode acpi") || !strings.Contains(startScript, "virsh -c qemu:///system start") {
		t.Fatalf("start script should stay a direct lifecycle action:\n%s", startScript)
	}
}

func TestPendingShutdownDisplaysTransitionState(t *testing.T) {
	host := Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"}
	vm := VM{Name: "vm1", UUID: "uuid-1", State: "running", Owner: "alice"}
	m := Model{
		config:     Config{Theme: "Classic"},
		activeHost: host,
		vms:        []VM{vm},
	}
	m.markShutdownRequested(vm)
	if got := m.vmStateLabel(vm); got != "shutdown..." {
		t.Fatalf("pending shutdown label = %q, want shutdown...", got)
	}
	m.reconcilePendingTransitions([]VM{{Name: "vm1", UUID: "uuid-1", State: "shut off", Owner: "alice"}})
	if got := m.vmStateLabel(VM{Name: "vm1", UUID: "uuid-1", State: "shut off", Owner: "alice"}); got != "off" {
		t.Fatalf("completed shutdown label = %q, want off", got)
	}
}

func TestPendingLaunchDisplaysTransitionState(t *testing.T) {
	host := Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"}
	vm := VM{Name: "vm1", UUID: "uuid-1", State: "shut off", Owner: "alice"}
	m := Model{
		config:     Config{Theme: "Classic"},
		activeHost: host,
		vms:        []VM{vm},
	}
	m.markLaunchRequested(vm)
	if got := m.vmStateLabel(vm); got != "launch..." {
		t.Fatalf("pending launch label = %q, want launch...", got)
	}
	m.reconcilePendingTransitions([]VM{{Name: "vm1", UUID: "uuid-1", State: "running", Owner: "alice"}})
	if got := m.vmStateLabel(VM{Name: "vm1", UUID: "uuid-1", State: "running", Owner: "alice"}); got != "running" {
		t.Fatalf("completed launch label = %q, want running", got)
	}
}

func TestListPowerActionRunsInBackground(t *testing.T) {
	vm := VM{Name: "vm1", UUID: "uuid-1", State: "shut off", Owner: "alice"}
	m := Model{
		config:     Config{Theme: "Classic"},
		mode:       modeVMs,
		hostTab:    hostTabVMs,
		activeHost: Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"},
		vms:        []VM{vm},
	}
	updated, cmd := m.updateVMKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd == nil {
		t.Fatal("power action should start a background command")
	}
	next := updated.(Model)
	if next.mode != modeVMs {
		t.Fatalf("power action should keep VM list visible, got mode %v", next.mode)
	}
	if got := next.vmStateLabel(vm); got != "launch..." {
		t.Fatalf("power action label = %q, want launch...", got)
	}
}

func TestDetailPowerActionRunsInBackground(t *testing.T) {
	vm := VM{Name: "vm1", UUID: "uuid-1", State: "running", Owner: "alice"}
	m := Model{
		config:     Config{Theme: "Classic"},
		mode:       modeVMDetail,
		activeHost: Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"},
		vmDetail:   VMDetail{VM: vm},
	}
	updated, cmd := m.runVMAction(vmActionPower)
	if cmd == nil {
		t.Fatal("power action should start a background command")
	}
	next := updated.(Model)
	if next.mode != modeVMDetail {
		t.Fatalf("power action should keep VM detail visible, got mode %v", next.mode)
	}
	if got := next.vmStateLabel(vm); got != "shutdown..." {
		t.Fatalf("power action label = %q, want shutdown...", got)
	}
}

func TestBackgroundLifecycleFailureClearsTransition(t *testing.T) {
	vm := VM{Name: "vm1", UUID: "uuid-1", State: "shut off", Owner: "alice"}
	m := Model{
		config:     Config{Theme: "Classic"},
		mode:       modeVMs,
		activeHost: Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"},
		vms:        []VM{vm},
	}
	m.markLaunchRequested(vm)
	updated, cmd := m.updateResult(resultMsg{op: "start", host: m.activeHost, vm: vm, background: true, err: errTest("exit status 1")})
	if cmd != nil {
		t.Fatal("failed background lifecycle action should not start a refresh command")
	}
	next := updated.(Model)
	if next.mode != modeVMs {
		t.Fatalf("failed background action should keep current mode, got %v", next.mode)
	}
	if got := next.vmStateLabel(vm); got == "launch..." {
		t.Fatalf("failed background action should clear launch transition, got %q", got)
	}
	if !strings.Contains(next.errText, "start failed") {
		t.Fatalf("failed background action should report error, got %q", next.errText)
	}
}

func TestVMRowStyleMutesOffVMsWithoutFaint(t *testing.T) {
	m := Model{config: Config{Theme: "Classic"}}
	off := m.vmRowStyle(VM{State: "shut off"}, false)
	if off.GetFaint() {
		t.Fatal("off VM row should use a brighter muted color instead of terminal faint")
	}
	running := m.vmRowStyle(VM{State: "running"}, false)
	if running.GetFaint() {
		t.Fatal("running VM row should not be dimmed")
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
		if strings.Contains(line, "running") || strings.Contains(line, " off ") {
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
	if !strings.Contains(view, "VMs connect to the VM endpoint") {
		t.Fatalf("mapping tab should explain the guest-facing endpoint:\n%s", view)
	}
	if !strings.Contains(view, "127.0.0.1:8080") || !strings.Contains(view, "192.168.122.1:8081") {
		t.Fatalf("mapping endpoints missing:\n%s", view)
	}
	writeMappingEndpointHost(m.stateDir, "iron", "map1", "192.168.130.1")
	view = stripANSI(m.viewHostDetail(100, 20))
	if !strings.Contains(view, "192.168.130.1:8081") {
		t.Fatalf("mapping tab should render recorded bridge endpoint:\n%s", view)
	}
}

func TestMappingStatusStyleUsesStateColors(t *testing.T) {
	m := Model{config: Config{Theme: "Classic"}}
	active := m.mappingStatusStyle("active")
	if active.GetForeground() != m.currentTheme().OK {
		t.Fatalf("active mapping status foreground = %q, want %q", active.GetForeground(), m.currentTheme().OK)
	}
	stopped := m.mappingStatusStyle("stopped")
	if stopped.GetForeground() != m.currentTheme().Error {
		t.Fatalf("stopped mapping status foreground = %q, want %q", stopped.GetForeground(), m.currentTheme().Error)
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

func TestVMActionsExposeDuplicateFlow(t *testing.T) {
	m := Model{
		config:     Config{Theme: "Classic"},
		mode:       modeVMDetail,
		activeHost: Host{Name: "iron"},
		vmTab:      vmTabActions,
		vmDetail: VMDetail{
			VM: VM{Name: "source-vm", State: "shut off"},
		},
	}
	view := stripANSI(m.viewVMDetail(100, 28))
	if !strings.Contains(view, "Duplicate") || !strings.Contains(view, "Duplicate to new VM name") {
		t.Fatalf("actions tab should expose duplicate action:\n%s", view)
	}
	if !strings.Contains(m.helpText(), "up/down: choose") || strings.Contains(m.helpText(), "rename/duplicate") {
		t.Fatalf("actions help should advertise duplicate: %q", m.helpText())
	}
	m.vmActionCursor = vmActionDuplicate
	updated, cmd := m.updateVMDetailKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("opening duplicate form should not start a command")
	}
	next := updated.(Model)
	if next.mode != modeDuplicateVM || next.duplicateVMName != "source-vm-copy" {
		t.Fatalf("duplicate action did not open form with suggested name: %#v", next)
	}
	form := stripANSI(next.viewDuplicateVM(80, 16))
	if !strings.Contains(form, "Source:     source-vm") || !strings.Contains(form, "> New name: source-vm-copy") {
		t.Fatalf("duplicate form missing source/new name:\n%s", form)
	}
	if help := next.helpText(); strings.Contains(help, "q: quit") || !strings.Contains(help, "type name") {
		t.Fatalf("duplicate footer should be a scoped form footer, got %q", help)
	}
	next.duplicateVMName = "dupli"
	updated, cmd = next.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		t.Fatal("typing q in duplicate form should not quit")
	}
	next = updated.(Model)
	if next.duplicateVMName != "dupliq" {
		t.Fatalf("typing q should edit the duplicate VM name, got %q", next.duplicateVMName)
	}
}

func TestVMActionsExposeRenameFlow(t *testing.T) {
	m := Model{
		config:     Config{Theme: "Classic"},
		mode:       modeVMDetail,
		activeHost: Host{Name: "iron"},
		vmTab:      vmTabActions,
		vmDetail: VMDetail{
			VM: VM{Name: "source-vm", State: "shut off"},
		},
	}
	view := stripANSI(m.viewVMDetail(100, 24))
	if !strings.Contains(view, "Rename") || !strings.Contains(view, "Rename VM") {
		t.Fatalf("actions tab should expose rename action:\n%s", view)
	}
	if !strings.Contains(m.helpText(), "enter: run") || strings.Contains(m.helpText(), "e/d") {
		t.Fatalf("actions help should advertise rename: %q", m.helpText())
	}
	m.vmActionCursor = vmActionRename
	updated, cmd := m.updateVMDetailKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("opening rename form should not start a command")
	}
	next := updated.(Model)
	if next.mode != modeRenameVM || next.renameVMName != "source-vm" {
		t.Fatalf("rename action did not open form with current name: %#v", next)
	}
	form := stripANSI(next.viewRenameVM(80, 16))
	if !strings.Contains(form, "Current:    source-vm") || !strings.Contains(form, "> New name: source-vm") {
		t.Fatalf("rename form missing current/new name:\n%s", form)
	}
	if help := next.helpText(); strings.Contains(help, "q: quit") || !strings.Contains(help, "type name") {
		t.Fatalf("rename footer should be a scoped form footer, got %q", help)
	}
	next.renameVMName = "renamed"
	updated, cmd = next.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		t.Fatal("typing q in rename form should not quit")
	}
	next = updated.(Model)
	if next.renameVMName != "renamedq" {
		t.Fatalf("typing q should edit the rename VM name, got %q", next.renameVMName)
	}
}

func TestVMActionsExposeUSBTabletRepair(t *testing.T) {
	m := Model{
		config:     Config{Theme: "Classic"},
		mode:       modeVMDetail,
		activeHost: Host{Name: "iron"},
		vmTab:      vmTabActions,
		vmDetail: VMDetail{
			VM: VM{Name: "source-vm", State: "running"},
		},
	}
	view := stripANSI(m.viewVMDetail(100, 26))
	if !strings.Contains(view, "Repair") || !strings.Contains(view, "Add USB tablet input") {
		t.Fatalf("actions tab should expose USB tablet repair:\n%s", view)
	}
	if !strings.Contains(m.helpText(), "enter: run") || strings.Contains(m.helpText(), "t: tablet") {
		t.Fatalf("actions help should advertise tablet repair: %q", m.helpText())
	}
	m.vmActionCursor = vmActionRepairTablet
	updated, cmd := m.updateVMDetailKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("tablet repair should start a command")
	}
	next := updated.(Model)
	if next.mode != modeBusy || next.priorMode != modeVMDetail || !strings.Contains(next.status, "Repairing USB tablet input") {
		t.Fatalf("tablet repair did not enter busy mode: %#v", next)
	}
}

func TestVMActionsUseArrowSelection(t *testing.T) {
	m := Model{
		config:     Config{Theme: "Classic"},
		mode:       modeVMDetail,
		activeHost: Host{Name: "iron"},
		vmTab:      vmTabActions,
		vmDetail: VMDetail{
			VM: VM{Name: "source-vm", State: "shut off"},
		},
	}
	view := stripANSI(m.viewVMActions(80, 24))
	if !strings.Contains(view, "> Start VM") {
		t.Fatalf("first action should be selected by default:\n%s", view)
	}
	updated, cmd := m.updateVMDetailKey(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatal("moving the action selection should not start a command")
	}
	next := updated.(Model)
	if next.vmActionCursor != vmActionForceOff {
		t.Fatalf("down should select force off, got cursor %d", next.vmActionCursor)
	}
	updated, cmd = next.updateVMDetailKey(tea.KeyMsg{Type: tea.KeyUp})
	if cmd != nil {
		t.Fatal("moving the action selection should not start a command")
	}
	next = updated.(Model)
	if next.vmActionCursor != vmActionPower {
		t.Fatalf("up should return to power action, got cursor %d", next.vmActionCursor)
	}
	updated, cmd = next.updateVMDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatal("letter keys should not run Actions-tab actions")
	}
	next = updated.(Model)
	if next.mode != modeVMDetail {
		t.Fatalf("letter key should leave actions tab in detail mode, got %#v", next.mode)
	}
}

func TestDuplicateVMNameValidation(t *testing.T) {
	m := Model{
		vmDetail:        VMDetail{VM: VM{Name: "source-vm"}},
		duplicateVMName: "source-vm-copy",
	}
	name, err := m.pendingDuplicateVMName()
	if err != nil {
		t.Fatalf("expected duplicate name to validate: %v", err)
	}
	if name != "source-vm-copy" {
		t.Fatalf("unexpected duplicate name: %q", name)
	}
	m.duplicateVMName = "source-vm"
	if _, err := m.pendingDuplicateVMName(); err == nil {
		t.Fatal("expected duplicate name matching source to fail")
	}
	m.duplicateVMName = "bad name"
	if _, err := m.pendingDuplicateVMName(); err == nil {
		t.Fatal("expected duplicate name with spaces to fail")
	}
	long := strings.Repeat("a", maxVMNameRunes)
	if got := suggestedDuplicateName(long); len([]rune(got)) != maxVMNameRunes || !strings.HasSuffix(got, "-copy") {
		t.Fatalf("suggested duplicate name should be capped with suffix, got %q (%d runes)", got, len([]rune(got)))
	}
}

func TestRenameVMNameValidation(t *testing.T) {
	m := Model{
		vmDetail:     VMDetail{VM: VM{Name: "source-vm"}},
		renameVMName: "renamed-vm",
	}
	name, err := m.pendingRenameVMName()
	if err != nil {
		t.Fatalf("expected rename name to validate: %v", err)
	}
	if name != "renamed-vm" {
		t.Fatalf("unexpected rename name: %q", name)
	}
	m.renameVMName = "source-vm"
	if _, err := m.pendingRenameVMName(); err == nil {
		t.Fatal("expected rename name matching current VM to fail")
	}
	m.renameVMName = "bad name"
	if _, err := m.pendingRenameVMName(); err == nil {
		t.Fatal("expected rename name with spaces to fail")
	}
}

func TestRenameVMScriptUsesInactiveDomrename(t *testing.T) {
	script := renameVMScript("source-vm", "renamed-vm")
	for _, want := range []string{
		"domstate \"$source\"",
		"Power off ${source} before renaming it",
		"domrename \"$source\" \"$name\"",
		"VM UUID is unchanged",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("rename script missing %q:\n%s", want, script)
		}
	}
}

func TestRepairUSBTabletScriptAttachesConfigAndLive(t *testing.T) {
	script := repairUSBTabletScript("source-vm")
	for _, want := range []string{
		"<input type='tablet' bus='usb'/>",
		"dumpxml --inactive \"$vm\"",
		"attach-device \"$vm\" \"$device_xml\" --config",
		"attach-device \"$vm\" \"$device_xml\" --live",
		"USB tablet input is already present",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("tablet repair script missing %q:\n%s", want, script)
		}
	}
}

func TestCDROMDetachUsesMediaEject(t *testing.T) {
	if !isCDROMDisk(VMDisk{Type: "file", Device: "cdrom", Target: "hda"}) {
		t.Fatal("cdrom device should be treated as CDROM media")
	}
	if !isCDROMDisk(VMDisk{Type: "cdrom", Target: "hda"}) {
		t.Fatal("cdrom type should be treated as CDROM media")
	}
	if isCDROMDisk(VMDisk{Type: "file", Device: "disk", Target: "vda"}) {
		t.Fatal("normal disk should not be treated as CDROM media")
	}
	script, err := detachDiskScript("vm1", VMDisk{Type: "file", Device: "cdrom", Target: "hda"})
	if err != nil {
		t.Fatalf("detachDiskScript returned error: %v", err)
	}
	if !strings.Contains(script, "device='cdrom'") || !strings.Contains(script, "change-media") || !strings.Contains(script, "--eject") {
		t.Fatalf("cdrom detach script should eject media with virsh change-media:\n%s", script)
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
	if mapping.Host != "iron" || mapping.LocalPort != 8080 || mapping.RemotePort != 8081 || mapping.RemoteHost != defaultVMBridgeHost {
		t.Fatalf("unexpected mapping: %#v", mapping)
	}
}

func TestAddMappingTreatsQAsText(t *testing.T) {
	m := Model{
		config:     Config{Theme: "Classic"},
		mode:       modeAddMapping,
		activeHost: Host{Name: "iron"},
		addMapName: "s",
	}
	updated, cmd := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		t.Fatal("q in add mapping form should edit text, not quit")
	}
	next := updated.(Model)
	if next.addMapName != "sq" {
		t.Fatalf("q was not entered into mapping name: %#v", next)
	}
	if strings.Contains(next.helpText(), "q: quit") {
		t.Fatalf("add mapping footer should not advertise quit: %q", next.helpText())
	}
}

func TestParseVMBridgeAddress(t *testing.T) {
	out := "noise\nVMRELAY_BRIDGE\t192.168.122.1\n"
	if got := parseVMBridgeAddress(out); got != "192.168.122.1" {
		t.Fatalf("parseVMBridgeAddress = %q", got)
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
		createVMISO:      "~/Documents/windows.iso",
		createVMNetwork:  "default",
		createVMFirmware: "uefi",
		createVMShared:   "n",
	}
	view := stripANSI(m.viewCreateVM(100, 20))
	if !strings.Contains(view, "Create VM on iron") || !strings.Contains(view, "windows.iso") || !strings.Contains(view, "Disk bus") || !strings.Contains(view, "Firmware") || !strings.Contains(view, "No - private") {
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
	m.createVMISO = "~/Documents/windows.iso"
	m.createVMFirmware = "coreboot"
	if _, err := m.pendingVMCreate(); err == nil {
		t.Fatal("expected unsupported firmware to fail")
	}
}

func TestImportVBoxFormAndValidation(t *testing.T) {
	m := Model{
		config:            Config{Theme: "Classic"},
		mode:              modeImportVBox,
		activeHost:        Host{Name: "iron"},
		importVBoxPath:    "~/Documents/Win10/Win10.vbox",
		importVBoxName:    "Win10-Imported",
		importVBoxDiskBus: "sata",
		importVBoxNetwork: "default",
		importVBoxShared:  "yes",
	}
	view := stripANSI(m.viewImportVBox(110, 22))
	for _, want := range []string{"Import VirtualBox VM on iron", "Win10.vbox", "Disk bus", "VMRelay NAT", "converted to qcow2"} {
		if !strings.Contains(view, want) {
			t.Fatalf("VirtualBox import form missing %q:\n%s", want, view)
		}
	}
	req, err := m.pendingVBoxImport()
	if err != nil {
		t.Fatalf("pendingVBoxImport returned error: %v", err)
	}
	if req.VBoxPath != "~/Documents/Win10/Win10.vbox" || req.Name != "Win10-Imported" || req.DiskBus != "sata" || req.Network != "default" || !req.Shared {
		t.Fatalf("unexpected VirtualBox import request: %#v", req)
	}

	m.importVBoxName = ""
	req, err = m.pendingVBoxImport()
	if err != nil {
		t.Fatalf("blank override name should use .vbox name: %v", err)
	}
	if req.Name != "" {
		t.Fatalf("blank override should stay blank for remote parser, got %q", req.Name)
	}

	m.importVBoxPath = "relative.vbox"
	if _, err := m.pendingVBoxImport(); err == nil {
		t.Fatal("expected relative .vbox path to fail")
	}
	m.importVBoxPath = "~/Documents/Win10/Win10.xml"
	if _, err := m.pendingVBoxImport(); err == nil {
		t.Fatal("expected non-.vbox path to fail")
	}
}

func TestHostImportVBoxShortcut(t *testing.T) {
	m := Model{
		config:     Config{Theme: "Classic"},
		mode:       modeVMs,
		hostTab:    hostTabVMs,
		activeHost: Host{Name: "iron"},
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if cmd != nil {
		t.Fatal("opening VirtualBox import form should not start a command")
	}
	next := updated.(Model)
	if next.mode != modeImportVBox || next.importVBoxPath != "~/Documents/" || next.importVBoxDiskBus != "sata" || next.importVBoxNetwork != "default" {
		t.Fatalf("unexpected VirtualBox import defaults: %#v", next)
	}
}

func TestImportVirtualBoxScriptConvertsAndOverridesNetworking(t *testing.T) {
	script := importVirtualBoxVMScript(vboxImportRequest{
		VBoxPath: "~/Documents/Win10/Win10.vbox",
		Name:     "Win10-Imported",
		DiskBus:  "sata",
		Network:  "default",
		Shared:   true,
	})
	for _, want := range []string{
		`python3 - "$vbox" "$name_override"`,
		`VMRELAY_VBOX_VM`,
		`VMRELAY_VBOX_DISK`,
		`qemu-img convert -p -f "$format" -O qcow2 "$source" "$dest"`,
		`--network "network=${network},model=e1000e"`,
		`--graphics vnc,listen=127.0.0.1`,
		`--input type=tablet,bus=usb`,
		`virsh -c qemu:///system define "$xml"`,
		`printf '%s\t%s\t%s\t%s\n' "$uuid" "$(whoami)" "$shared" ''`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("VirtualBox import script missing %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "attach-disk") {
		t.Fatalf("VirtualBox import should define a new VM, not attach to an existing VM:\n%s", script)
	}

	path := t.TempDir() + "/import-vbox.sh"
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if out, err := exec.Command("bash", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("VirtualBox import script failed bash -n: %v\n%s\n%s", err, out, script)
	}
}

func TestErrorSummaryIncludesRemoteOutputAfterExitStatus(t *testing.T) {
	got := errorSummary("VM creation failed: exit status 1\nsudo: a password is required\n")
	want := "VM creation failed: exit status 1: sudo: a password is required"
	if got != want {
		t.Fatalf("errorSummary() = %q, want %q", got, want)
	}
}

func TestErrorSummarySkipsWarningsAfterExitStatus(t *testing.T) {
	got := errorSummary("VM creation failed: exit status 1\nWARNING  Treating --wait 0 as --noautoconsole\nValidating install media failed\n")
	want := "VM creation failed: exit status 1: Validating install media failed"
	if got != want {
		t.Fatalf("errorSummary() = %q, want %q", got, want)
	}
}

func TestCreateVMBootOptionPreservesFirmwareMode(t *testing.T) {
	if got := createVMBootOption("uefi"); !strings.Contains(got, "uefi") || !strings.Contains(got, "secure-boot") || !strings.Contains(got, "enrolled-keys") {
		t.Fatalf("UEFI boot option = %q", got)
	}
	if got := createVMBootOption("bios"); got != "cdrom,hd" {
		t.Fatalf("BIOS boot option = %q", got)
	}
}

func TestNICAttachDefaultsToWindowsCompatibleModel(t *testing.T) {
	m := Model{
		config: Config{Theme: "Classic"},
		mode:   modeVMDetail,
		vmTab:  vmTabNICs,
		vmDetail: VMDetail{
			VM: VM{Name: "win11"},
		},
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got := next.(Model)
	if got.mode != modeAddNIC || got.addNICSource != "default" || got.addNICModel != "e1000e" {
		t.Fatalf("NIC add defaults = mode %v source %q model %q", got.mode, got.addNICSource, got.addNICModel)
	}
}

func TestCreateVMWizardArrowKeysAndPresetFields(t *testing.T) {
	m := Model{
		config:           Config{Theme: "Classic"},
		mode:             modeCreateVM,
		activeHost:       Host{Name: "iron"},
		createVMName:     "win10",
		createVMMemory:   "4",
		createVMCPUs:     "2",
		createVMDiskSize: "64",
		createVMDiskBus:  "sata",
		createVMISO:      "~/Documents/windows.iso",
		createVMNetwork:  "default",
		createVMFirmware: "uefi",
		createVMShared:   "no",
	}
	updated, _ := m.updateCreateVMKey(tea.KeyMsg{Type: tea.KeyDown})
	next := updated.(Model)
	if next.createVMField != createVMFieldMemory {
		t.Fatalf("down should move to memory field, got %d", next.createVMField)
	}
	updated, _ = next.updateCreateVMKey(tea.KeyMsg{Type: tea.KeyRight})
	next = updated.(Model)
	if next.createVMMemory != "8" {
		t.Fatalf("right should cycle memory preset to 8, got %q", next.createVMMemory)
	}
	next.createVMField = createVMFieldDiskBus
	updated, _ = next.updateCreateVMKey(tea.KeyMsg{Type: tea.KeyLeft})
	next = updated.(Model)
	if next.createVMDiskBus != "ide" {
		t.Fatalf("left should cycle disk bus to previous option, got %q", next.createVMDiskBus)
	}
	next.createVMField = createVMFieldShared
	updated, _ = next.updateCreateVMKey(tea.KeyMsg{Type: tea.KeyRight})
	next = updated.(Model)
	if next.createVMShared != "yes" || sharedChoiceLabel(next.createVMShared) != "Yes - shared" {
		t.Fatalf("right should cycle shared to yes, got %q", next.createVMShared)
	}
	updated, _ = next.updateCreateVMKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	next = updated.(Model)
	if next.createVMShared != "yes" {
		t.Fatalf("shared field should not accept typed text, got %q", next.createVMShared)
	}
	next.createVMField = createVMFieldName
	updated, _ = next.updateCreateVMKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	next = updated.(Model)
	if !strings.HasSuffix(next.createVMName, "h") {
		t.Fatalf("text fields should still accept h/l characters, got name %q", next.createVMName)
	}
}

func TestCreateVMNameAllowsLongSafeNamesAndScrollsDisplay(t *testing.T) {
	longName := strings.Repeat("a", maxVMNameRunes)
	m := Model{
		config:           Config{Theme: "Classic"},
		mode:             modeCreateVM,
		activeHost:       Host{Name: "iron"},
		createVMName:     longName,
		createVMMemory:   "4",
		createVMCPUs:     "2",
		createVMDiskSize: "64",
		createVMDiskBus:  "sata",
		createVMISO:      "~/Documents/windows.iso",
		createVMNetwork:  "default",
		createVMFirmware: "uefi",
		createVMShared:   "no",
		createVMField:    createVMFieldName,
	}
	if _, err := m.pendingVMCreate(); err != nil {
		t.Fatalf("expected %d-character VM name to validate: %v", maxVMNameRunes, err)
	}
	m.createVMName = longName + "b"
	if _, err := m.pendingVMCreate(); err == nil {
		t.Fatalf("expected VM name longer than %d characters to fail", maxVMNameRunes)
	}
	m.createVMName = "windows_10_enterprise_draytek_vpn_virtualisation_server"
	view := stripANSI(m.viewCreateVM(46, 20))
	if !strings.Contains(view, "...n_virtualisation_server") {
		t.Fatalf("active long VM name should show the typed suffix in narrow view:\n%s", view)
	}

	m.createVMName = strings.Repeat("x", maxVMNameRunes-1)
	updated, _ := m.updateCreateVMKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y', 'z'}})
	next := updated.(Model)
	if got := len([]rune(next.createVMName)); got != maxVMNameRunes {
		t.Fatalf("typed VM name should stop at %d runes, got %d", maxVMNameRunes, got)
	}
	if !strings.HasSuffix(next.createVMName, "y") {
		t.Fatalf("expected first typed rune to fit and extra rune to be ignored, got %q", next.createVMName)
	}
}

func TestISOEntryParsingAndPickerRendering(t *testing.T) {
	out := strings.Join([]string{
		"VMRELAY_ISO_DIR\t/home/simplehelp/Documents",
		"VMRELAY_ISO_ENTRY\tubuntu.iso\tfile\t/home/simplehelp/Documents/ubuntu.iso",
		"VMRELAY_ISO_ENTRY\twindows.iso\tfile\t/home/simplehelp/Documents/windows.iso",
		"VMRELAY_ISO_ENTRY\told\tdir\t/home/simplehelp/Documents/old",
		"ignored diagnostic",
	}, "\n")
	entries := parseRemoteISOEntries(out)
	if len(entries) != 4 {
		t.Fatalf("expected parent, directory, and two ISO entries, got %#v", entries)
	}
	if entries[0].Name != ".." || !entries[0].Dir || entries[0].Path != "/home/simplehelp" {
		t.Fatalf("unexpected parent entry: %#v", entries[0])
	}
	if entries[1].Name != "old" || !entries[1].Dir {
		t.Fatalf("directories should sort before ISO files: %#v", entries)
	}
	m := Model{
		config:     Config{Theme: "Classic"},
		mode:       modeISOPicker,
		activeHost: Host{Name: "iron"},
		isoDir:     "/home/simplehelp/Documents",
		isoEntries: entries,
		isoCursor:  2,
	}
	view := stripANSI(m.viewISOPicker(100, 20))
	if !strings.Contains(view, "Select ISO on iron") || !strings.Contains(view, "ubuntu.iso") || !strings.Contains(view, "old/") {
		t.Fatalf("ISO picker missing expected entries:\n%s", view)
	}
	updated, cmd := m.updateISOPickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("selecting an ISO should not start a command")
	}
	next := updated.(Model)
	if next.mode != modeCreateVM || next.createVMISO != "/home/simplehelp/Documents/ubuntu.iso" {
		t.Fatalf("ISO selection did not return to create form with selected ISO: %#v", next)
	}
}

func TestEnterOnISOFieldStartsRemotePicker(t *testing.T) {
	m := Model{
		config:      Config{Theme: "Classic"},
		mode:        modeCreateVM,
		activeHost:  Host{Name: "iron", Target: "simplehelp@iron.simplehelp.io"},
		createVMISO: "~/Documents/",
	}
	m.createVMField = createVMFieldISO
	updated, cmd := m.updateCreateVMKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on ISO field should start remote directory loading")
	}
	next := updated.(Model)
	if next.mode != modeBusy || next.priorMode != modeISOPicker || next.isoDir != "~/Documents" {
		t.Fatalf("unexpected state after opening ISO picker: %#v", next)
	}
}

func TestVMTabCanOpenCreateVM(t *testing.T) {
	m := Model{
		config:     Config{Theme: "Classic"},
		mode:       modeVMs,
		hostTab:    hostTabVMs,
		activeHost: Host{Name: "iron"},
	}
	updated, cmd := m.updateVMKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd != nil {
		t.Fatal("expected no command when opening create form")
	}
	next := updated.(Model)
	if next.mode != modeCreateVM || next.createVMFirmware != "uefi" || next.createVMDiskBus != "sata" || next.createVMISO != "~/Documents/" {
		t.Fatalf("n on VM tab did not open create form with defaults: %#v", next)
	}
	if !strings.Contains(stripANSI(m.helpText()), "n: create VM") {
		t.Fatalf("VM tab help text should advertise creation: %q", m.helpText())
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

func TestConsoleRemoteScriptRestartsStaleNoVNCProxy(t *testing.T) {
	script := consoleRemoteScript("Win11-Orig", 6316)
	for _, want := range []string{
		`target="${host}:${port}"`,
		`existing_args="$(ps -p "$pid" -o args= 2>/dev/null || true)"`,
		`Restarting stale noVNC`,
		`pkill -P "$pid"`,
		`start_novnc`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("console script missing %q:\n%s", want, script)
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
