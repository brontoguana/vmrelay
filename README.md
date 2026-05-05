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
2. Press `m` to browse and select a saved theme.
3. Press `t` to test SSH, libvirt, KVM, noVNC, and websockify.
4. Press `s` to run apt-based setup on Ubuntu/Debian hosts.
5. Press `Enter` to open the VM list for the selected host.
6. Select a VM and press `o` to open its browser console.

## Capabilities

- Host manager opens by default; there is no separate day-to-day CLI workflow.
- Startup prompts for update-and-restart when a newer GitHub release is available.
- The TUI uses a full-screen terminal layout with a VMRelay title border and ten selectable themes.
- Hosts are reached over SSH and managed through system libvirt at `qemu:///system`.
- Host setup installs/checks `qemu-kvm`, libvirt clients/daemon, `virt-install`, `qemu-utils`, noVNC, and websockify on apt-based hosts.
- VM inventory shows state plus VMRelay ownership status.
- VM actions currently include start, shutdown, force off, refresh, adopt ownership, share/private toggle, browser console open, and console stop.
- VM consoles use the libvirt VNC display on the remote host, noVNC/websockify bound to remote `127.0.0.1`, and an SSH local forward to a browser URL on local `127.0.0.1`.
- VMRelay imports legacy host definitions from `~/.config/vmrelay/hosts.d` when present.

## Ownership

VMRelay manages ownership from the start of the TUI model.

- VMs are system-wide libvirt resources on the remote host.
- VMRelay ownership metadata lives on the remote host at `/var/lib/vmrelay/ownership.tsv`.
- VMs can be adopted by the current remote SSH account.
- Private VMs are intended for owner/admin visibility.
- Shared VMs are visible to all VMRelay users for that host.
- Local console and port choices remain local to each workstation/user.

The current TUI includes the host-side ownership state and share/private flags. VM creation/import and full role grants are planned next.

## Security Model

VMRelay does not expose libvirt, noVNC, or websockify directly on the network. Management transport is SSH. Browser console services listen on loopback and are reached through SSH local forwards.

VMRelay ownership is product-level access control. If a remote account has unrestricted root, sudo, or libvirt access outside VMRelay, host Unix permissions and sudoers/libvirt policy must also match the intended segregation.

## Configuration

Local config:

```text
~/.config/vmrelay/config.json
```

Local runtime state:

```text
~/.local/state/vmrelay
```

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
