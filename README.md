# VMRelay

VMRelay is a terminal UI for managing VMs on a normal remote Linux host without turning that host into a dedicated virtualization appliance. It uses SSH, KVM/libvirt, noVNC, and websockify underneath, but the day-to-day workflow starts from one app:

```bash
vmrelay
```

On startup, VMRelay checks the latest GitHub release. If a newer version is available, it asks whether to update and restart before continuing.

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
7. In VMs, press `n` to create a new VM from a remote ISO path.
8. Select a VM and press `Enter` to open its VM detail screen.
9. In VM detail, use Summary, Disks, NICs, and Actions tabs.
10. In Disks, press `n` to create and attach a qcow2 disk, `i` to import/convert/attach an existing disk, `enter` to make the selected disk first in boot order, or `x` to detach the selected disk.
11. In NICs, press `n` to attach a libvirt network NIC or `x` to detach the selected NIC.
12. In Mappings, press `n` to add a local SSH port mapping and `e` to start or stop it.

## Capabilities

- Host manager opens by default; there is no separate day-to-day CLI workflow.
- Startup prompts for update-and-restart when a newer GitHub release is available.
- The TUI uses a full-screen terminal layout with a VMRelay title border, one outer line frame, and ten selectable themes.
- Hosts are reached over SSH and managed through system libvirt at `qemu:///system`.
- Host setup installs/checks `qemu-kvm`, libvirt clients/daemon, `virt-install`, `qemu-utils`, OVMF/UEFI firmware, noVNC, and websockify on apt-based hosts.
- Host detail screens include VM inventory, host config/readiness actions, and local port mappings.
- VM creation from the host VMs or Config tab creates a qcow2 boot disk, starts a VNC installer VM from a remote ISO path with selectable disk bus, BIOS/UEFI firmware, libvirt networking, and USB tablet input, and records VMRelay ownership for the remote SSH user.
- VM inventory shows state plus VMRelay ownership status.
- VM detail screens show summary, disks, NICs, and actions for the selected VM.
- VM actions include start, shutdown, force off, refresh, adopt ownership, share/private toggle, browser console open, and console stop.
- Disk management can create qcow2 disks, import existing remote disk images, auto-convert non-qcow2 sources through `qemu-img convert`, attach disks persistently, set the selected disk as the VM's first boot disk, and detach disks without deleting their image files.
- NIC management can attach a virtio interface to a libvirt network such as `default` and detach selected interfaces by MAC address.
- Local port mappings are saved per workstation/user and run as SSH local forwards such as `127.0.0.1:8080 -> 127.0.0.1:8081` on the selected host.
- VM consoles use the libvirt VNC display on the remote host, noVNC/websockify bound to remote `127.0.0.1`, and an SSH local forward to a browser URL on local `127.0.0.1`; console URLs enable noVNC's local pointer dot and low-latency settings by default, and if the preferred local console port is busy, VMRelay automatically picks the next available local port.
- VMRelay imports legacy host definitions from `~/.config/vmrelay/hosts.d` when present.

## Ownership

VMRelay manages ownership from the start of the TUI model.

- VMs are system-wide libvirt resources on the remote host.
- VMRelay ownership metadata lives on the remote host at `/var/lib/vmrelay/ownership.tsv`.
- VMs can be adopted by the current remote SSH account.
- Private VMs are intended for owner/admin visibility.
- Shared VMs are visible to all VMRelay users for that host.
- Local console and port choices remain local to each workstation/user.

The current TUI includes the host-side ownership state, creation-time owner assignment, and share/private flags. Full role grants are planned next.

## Security Model

VMRelay does not expose libvirt, noVNC, or websockify directly on the network. Management transport is SSH. Browser console services listen on loopback and are reached through SSH local forwards.

VMRelay ownership is product-level access control. If a remote account has unrestricted root, sudo, or libvirt access outside VMRelay, host Unix permissions and sudoers/libvirt policy must also match the intended segregation.

## Configuration

Local config:

```text
~/.config/vmrelay/config.json
```

This stores hosts, theme choice, and local port mapping definitions for this workstation/user.

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
