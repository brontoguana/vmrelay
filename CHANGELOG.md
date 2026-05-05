# Changelog

## Unreleased

## 0.2.4 - 2026-05-05

- Added a startup GitHub release check. If a newer VMRelay release is available, the TUI asks whether to update with the release installer and restart before continuing.
- Clarified the Hosts footer so `Enter`/`r` opens the selected host rather than implying VM console launch.
- Improved host-open failure status text so pressing `Enter` on a host reports a visible host-specific error if VM inventory cannot load.
- Added tests for update prompt rendering and semantic version comparison.

## 0.2.3 - 2026-05-05

- Changed the TUI layout so the key-help footer is anchored at the bottom of the screen inside the outer border.
- Changed the main panes, including the Hosts table, to fill the available screen area instead of rendering as compact content at the top.
- Added layout tests for bottom-anchored status/help rows and full-height host pane rendering.

## 0.2.2 - 2026-05-05

- Changed the Go TUI to render as a full-screen framed interface with a rounded outer border and centered `VMRelay` version title.
- Added ten selectable TUI themes, stored in local config and available from the main Hosts page with `m`.
- Fixed VM list column rendering so long VM names are clipped and owner/visibility columns stay vertically aligned.
- Added focused tests for full-screen frame sizing and VM row alignment.

## 0.2.1 - 2026-05-05

- Fixed remote SSH script execution in the Go TUI by sending scripts to `bash -s` over stdin instead of passing them through `bash -lc`, preventing remote shell diagnostics from being emitted during VM inventory.
- Hardened VM inventory parsing with explicit `VMRELAY_VM` record prefixes so command diagnostics cannot appear as fake VM rows.
- Added a focused parser test for ignoring remote diagnostics such as `bash: -c: option requires an argument`.

## 0.2.0 - 2026-05-05

- Built the first Go/Bubble Tea VMRelay TUI MVP as `vmrelay 0.2.0`, with host management, SSH host checks/setup, system-libvirt VM inventory, lifecycle actions, VM ownership/share state, and loopback noVNC console launch.
- Added a release installer (`install.sh`) and release build script for Linux/macOS amd64/arm64 binaries.
- Updated the README so the one-line install uses the Go binary release installer and the documented workflow starts with `vmrelay` opening the TUI.
- Changed the VMRelay TUI ownership design so VMs created/imported through VMRelay are owned by the creating remote host account by default, with explicit shared/granted/admin visibility rules.
- Clarified VMRelay TUI security assumptions: SSH remains the trust boundary, noVNC/websockify and local console URLs stay loopback-bound, port mappings travel over SSH, and stronger VM segregation requires host permissions to match VMRelay ownership policy.
- Added a VMRelay ownership model to the TUI design: VMs remain system-wide libvirt resources, but VMRelay records shared per-host VM ownership/operator policy while port mappings stay local to each user/workstation.
- Clarified the TUI design so remote VM management uses system-wide libvirt (`qemu:///system`) by default, while port mappings, console tunnels, local ports, and conflict-resolution choices are stored per local VMRelay user/workstation.
- Added a Lore design document for a future Go/Bubble Tea VMRelay TUI where `vmrelay` opens a host manager by default, Cockpit Machines is removed from the design, and host setup, VM management, browser consoles, and port mappings live in the TUI rather than a duplicated CLI workflow.
- Changed `vmrelay console` to best-effort open the generated noVNC URL in the local default browser on macOS, Linux, WSL, and Windows shell environments; set `VMRELAY_OPEN_BROWSER=0` to suppress auto-open.
- Bumped VMRelay to `0.1.11`.
- Fixed `vmrelay console` for Ubuntu's packaged `websockify`, which does not support the `--pid` option; VMRelay now backgrounds websockify with `nohup`, writes its own pid file, and reports the startup log if websockify exits immediately.
- Bumped VMRelay to `0.1.10`.
- Added `vmrelay console HOST VM_NAME [LOCAL_PORT]` to expose a libvirt VM's VNC graphics console through noVNC/websockify and an SSH local forward, so Linux and Windows guests can be accessed without guest-side RDP/SSH setup.
- Added `vmrelay console-down HOST VM_NAME` to stop the console-specific SSH tunnel and remote noVNC proxy.
- Changed `setup HOST` to install `novnc` and `websockify` on Ubuntu/Debian hosts.
- Bumped VMRelay to `0.1.9`.
- Added a clearer README description explaining that VMRelay uses a normal remote Linux box as a VM server without fully converting it into a Proxmox-style appliance.
- Changed the GitHub repository visibility from private to public.
- Changed the README installer to use the public GitHub contents API with `curl`, so install no longer requires authenticated `gh`.
- Changed `vmrelay update` to download from the public GitHub contents API with `curl` instead of requiring authenticated GitHub API access.
- Bumped VMRelay to `0.1.8`.
- Added a portable per-user command lock under `~/.vmrelay/lock` for config and tunnel operations, with a default 300-second wait timeout and dead-owner stale lock cleanup.
- Added operational logging to `~/.vmrelay/vmrelay.log`.
- Added `vmrelay tail [LINES]`, defaulting to the latest 200 log lines.
- Added `vmrelay resume` to start or reconcile tunnels for all configured hosts.
- Bumped VMRelay to `0.1.5`.
- Changed `status HOST` to print a complete disconnected report when the managed tunnel cannot start, including SSH target, WebGUI URL, Cockpit tunnel, configured mappings, and remote check failure.
- Changed empty inbound/outbound mapping sections to print `none`.
- Bumped VMRelay to `0.1.4`.
- Added a configured-host summary to the help output shown by bare `vmrelay`, `vmrelay --help`, and `vmrelay help`.
- Bumped VMRelay to `0.1.3`.
- Replaced GNU `find -maxdepth` usage with a portable shell glob so host listing works on macOS without GNU coreutils.
- Bumped VMRelay to `0.1.2`.
- Reviewed the initial CLI implementation for shell/runtime bugs.
- Fixed tunnel argument parsing so invalid host mappings fail before SSH starts instead of being masked by subshell/command-substitution behavior.
- Reused noninteractive SSH options for managed tunnels and remote setup, and made `setup HOST` fail clearly on SSH reachability errors before prompting for sudo.
- Bumped VMRelay to `0.1.1`.
- Verified the pushed GitHub download serves `0.1.1`; the exact README installer reaches `sudo` but cannot complete in this session because passwordless sudo is unavailable.
- Added the initial Bash implementation at `bin/vmrelay`.
- Added `README.md` with a one-line private GitHub install command.
- Added `.gitignore` so Lore metadata and `TRACKING.md` stay out of Git.
- Implemented local host config commands, inbound/outbound mapping commands, managed SSH tunnel start/stop/status, remote Ubuntu setup, and self-update support.
- Updated the Lore design doc with MVP tunnel ownership and config reconciliation decisions.
- Pushed the initial implementation to the private GitHub repository and verified the README install download path.
- Added proposed `vmrelay update` command for updating the local VMRelay script from the GitHub repository.
- Created the private GitHub repository `brontoguana/vmrelay`.
- Clarified that `setup HOST` should also ensure the built-in Cockpit/web UI local forward is active and print the assigned URL.
- Clarified that the Cockpit/web UI forward is a built-in system mapping, separate from user-managed inbound/outbound mappings.
- Added live reconciliation behavior for `inbound` and `outbound` mapping edits when a VMRelay-managed tunnel is already running.
- Changed the default Cockpit/web UI local base port from `19090` to `4400`; hosts now use `4400 + WEB_PORT_OFFSET`.
- Changed Cockpit/web UI forwarding from a single shared local port to stable per-host ports based on a common base port plus each host's assigned offset.
- Changed Cockpit/web UI forwarding to be automatically included by `up` and `status`.
- Clarified that remote Cockpit must listen only on `127.0.0.1` and must not be publicly exposed.
- Changed `setup HOST` to use sensible defaults without requiring `--ask-sudo` or a separate GatewayPorts option.
- Added proposed `inbound HOST LOCAL_PORT REMOTE_PORT` and `outbound HOST LOCAL_PORT REMOTE_PORT` commands for host config mapping edits.
- Split host config mappings into `INBOUND_MAPPINGS` and `OUTBOUND_MAPPINGS`.
- Removed the standalone proposed `verify` command; `status HOST` now covers both tunnel state and remote readiness checks.
- Removed proposed browser-opening commands and flags from the VMRelay command surface.
- Added proposed `removehost` and `host` commands for host removal and host-detail inspection.
- Clarified the safety boundary for VM bridge-bound reverse forwards during `setup HOST`.
- Renamed proposed CLI commands: `init`/`init-host` to `addhost`, `check`/`verify` to `status`, and `bootstrap` to `setup`.
- Changed reverse port handling so mappings live in host config instead of being passed as `vmrelay up --port` flags.
- Created the initial VMRelay design documentation in Lore.
- Added the initial Lore project file map entries for local bookkeeping files.
