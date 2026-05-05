package app

import (
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
	m := Model{version: "0.2.2", config: Config{Theme: "Classic"}, width: 80, height: 20, mode: modeHosts, status: "Ready."}
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
	if !strings.Contains(stripANSI(lines[0]), "VMRelay 0.2.2") {
		t.Fatalf("top border missing title/version: %q", lines[0])
	}
}

func TestThemeCatalogHasTenThemes(t *testing.T) {
	if len(themes) != 10 {
		t.Fatalf("expected 10 themes, got %d", len(themes))
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
	view := stripANSI(m.viewVMs(98))
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

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}
