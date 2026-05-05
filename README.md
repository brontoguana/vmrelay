# VMRelay

VMRelay is a terminal UI for managing VMs on a normal remote Linux host without turning that host into a dedicated virtualization appliance. It uses SSH, KVM/libvirt, noVNC, and websockify underneath, but the day-to-day workflow starts from one app:

```bash
vmrelay
```

On startup, VMRelay checks the latest GitHub release. If a newer version is available, it asks whether to update; accepting exits the TUI, restores the terminal, runs the installer against `/dev/tty` so sudo password prompts work, then restarts VMRelay.

VMs run system-wide under `qemu:///system` on the remote host. VMRelay stores local host preferences on the client machine, stores VM ownership metadata on the VM host, and exposes VM graphical consoles through browser-based noVNC tunnels bound to loopback and forwarded over SSH.

## Install

Install the latest VMRelay release:

```bash
curl -fsSL https://raw.githubusercontent.com/brontoguana/vmrelay/main/install.sh | bash
```

## Quick Start

```bash
vmrelay
```

Inside the TUI:

1. Press `a` to add a host such as `iron` with an SSH target such as `aem@iron`.
2. Press `m` from the bottom controls row to browse and select a saved theme.
3. Press `t` to test SSH, libvirt, KVM, noVNC, and websockify.
4. Press `s` to run apt-based setup on Ubuntu/Debian hosts.
5. Press `Enter` to open the selected host detail screen.
6. Use `left`/`right` to switch between VMs, Config, and Mappings.
7. In VMs, press `n` to create a new VM from a remote ISO path; use up/down to move fields, left/right to cycle preset values, and press `Enter` on the ISO field to browse remote directories and `.iso` files.
8. Select a VM and press `Enter` to open its VM detail screen.
9. In VM detail, use Summary, Disks, NICs, and Actions tabs.
10. In Disks, press `n` to create and attach a qcow2 disk, `i` to import/convert/attach an existing disk, `enter` to make the selected disk first in boot order, or `x` to detach the selected disk/eject selected ISO media.
11. In NICs, press `n` to attach a libvirt network NIC or `x` to detach the selected NIC.
12. In Actions, press `d` to duplicate a powered-off VM, type the new VM name, then press `Enter`.
13. In Mappings, press `n` to add a VM-accessible service mapping and `e` to start or stop it. VMs connect to the VM endpoint shown in that table.

## Capabilities

- Host manager opens by default; there is no separate day-to-day CLI workflow.
- Startup prompts for update-and-restart when a newer GitHub release is available, handing off to a restored `/dev/tty` terminal before running the installer so interactive sudo prompts work correctly.
- The TUI uses a full-screen terminal layout with a VMRelay title border, one outer line frame, and ten selectable themes.
- Hosts are reached over SSH and managed through system libvirt at `qemu:///system`.
- Host setup installs/checks `qemu-kvm`, libvirt clients/daemon, `virt-install`/`virt-clone`, `qemu-utils`, Python 3, OVMF/UEFI/Secure Boot firmware, `swtpm`, noVNC, and websockify on apt-based hosts, ensures the libvirt `default` NAT network is active/autostarted, enables SSH reverse forwards to the VM bridge, then initializes a VMRelay libvirt storage pool at `/var/lib/vmrelay/images`.
- Host detail screens include VM inventory, host config/readiness actions, and VM-accessible service mappings.
- VM creation from the host VMs or Config tab creates a qcow2 boot disk through the VMRelay storage pool when present, falls back to existing active libvirt pools for older hosts, stages user-home ISOs into libvirt-readable storage when needed, starts a VNC installer VM with selectable disk bus, BIOS/UEFI firmware, device-level CDROM-first installer boot order, NAT networking with a Windows-compatible `e1000e` NIC, USB tablet input, and UEFI Secure Boot plus TPM 2.0 when UEFI is selected, sets guest reboot behavior to restart instead of shutting off, and records VMRelay ownership for the remote SSH user when the ownership policy is writable. The creation wizard supports arrow-key field movement, preset cycling, Yes/No shared selection, VM names up to 80 characters, horizontally scrolling active text fields, and a read-only remote ISO picker rooted initially at the remote user's `~/Documents/`, with `~` paths accepted for ISO files.
- VM inventory shows state plus VMRelay ownership status.
- VM detail screens show summary, disks, NICs, and actions for the selected VM.
- VM actions include start, shutdown, force off, refresh, adopt ownership, share/private toggle, browser console open, console stop, and powered-off VM duplication with an editable new-name prompt.
- Disk management can create qcow2 disks, import existing remote disk images, auto-convert non-qcow2 sources through `qemu-img convert`, attach disks persistently, set the selected disk as the VM's first boot disk, detach disks without deleting their image files, and eject selected CDROM/ISO media.
- NIC management can attach an interface to a libvirt network such as `default`, defaulting to `e1000e` for stock Windows compatibility, and detach selected interfaces by MAC address.
- VM service mappings are saved per workstation/user and run as SSH reverse forwards. VMs connect to the VM endpoint shown on the Mappings page, normally `192.168.122.1:<vm-port>`, and VMRelay forwards that back to `127.0.0.1:<local-service-port>` on the local workstation. The bridge address is shared by every VM on the host's default NAT network, so it does not change per VM or per guest reboot. Starting a mapping performs the required default-network and bridge-bound SSH forwarding setup when the remote account can do so noninteractively; otherwise the screen reports that host setup is required.
- VM consoles use the libvirt VNC display on the remote host, noVNC/websockify bound to remote `127.0.0.1`, and an SSH local forward to a browser URL on local `127.0.0.1`; console open falls back to the live libvirt XML VNC port if `domdisplay` is unhelpful, restarts stale noVNC proxies when libvirt assigns a new VNC port, enables noVNC's local pointer dot and low-latency settings by default, and automatically picks the next available local port if the preferred local console port is busy.
- VMRelay imports legacy host definitions from `~/.config/vmrelay/hosts.d` when present.

## Ownership

VMRelay manages ownership from the start of the TUI model.

- VMs are system-wide libvirt resources on the remote host.
- VMRelay ownership metadata lives on the remote host at `/var/lib/vmrelay/ownership.tsv`.
- VMs can be adopted by the current remote SSH account.
- Private VMs are intended for owner/admin visibility.
- Shared VMs are visible to all VMRelay users for that host.
- Local console and VM service mapping choices remain local to each workstation/user.

The current TUI includes the host-side ownership state, creation-time owner assignment, and share/private flags. Full role grants are planned next.

## Security Model

VMRelay does not expose libvirt, noVNC, or websockify directly on the network. Management transport is SSH. Browser console services listen on loopback and are reached through SSH local forwards. VM service mappings bind only to the remote libvirt bridge address so guests can reach local workstation services through SSH reverse forwards without binding those services to the remote host's public network.

VMRelay ownership is product-level access control. If a remote account has unrestricted root, sudo, or libvirt access outside VMRelay, host Unix permissions and sudoers/libvirt policy must also match the intended segregation.

## Configuration

Local config:

```text
~/.config/vmrelay/config.json
```

This stores hosts, theme choice, and VM service mapping definitions for this workstation/user.

Local runtime state:

```text
~/.local/state/vmrelay
```

This stores SSH control sockets and transient console/mapping runtime state.

Remote VMRelay policy state:

```text
/var/lib/vmrelay/ownership.tsv
```

Set this to suppress automatic browser launch:

```bash
VMRELAY_OPEN_BROWSER=0 vmrelay
```

## Legacy Script

The older Bash implementation remains in `bin/vmrelay` as a migration reference for now. The release installer installs the Go TUI binary.
