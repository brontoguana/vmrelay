# Changelog

## Unreleased

## 0.2.35 - 2026-05-06

- Added a VM detail Actions tab repair action: press `t` to add a USB tablet input device to an existing VM for better absolute-pointer behavior in graphical consoles.
- The USB tablet repair action is idempotent, updates persistent VM XML, and attempts a live attach for running VMs when libvirt supports it.

## 0.2.34 - 2026-05-06

- Changed VM state display so libvirt `shut off` is shown as `off`, and a locally requested graceful shutdown is shown as `shutdown...` until background refresh sees the VM stop.
- Added state-colored VM rows in the VM list, with powered-off VMs dimmed.
- Added a per-remote-user ownership fallback when `/var/lib/vmrelay/ownership.tsv` is unavailable, so hosts without writable system policy still show VMs as owned by the SSH user and can record VMRelay ownership under the remote user's data directory.

## 0.2.33 - 2026-05-06

- Added a VM detail Actions tab rename flow: press `e`, edit the VM name, and press `Enter` to rename the powered-off VM with libvirt `domrename`.
- The rename form has a scoped footer with no app-level quit action, and `q` is entered as normal text while editing the new name.
- Rename preserves VMRelay ownership because the VM UUID is unchanged.

## 0.2.32 - 2026-05-05

- Changed VM service mappings to avoid SSHD configuration changes: SSH reverse tunnels now bind only remote `127.0.0.1`, and a remote bridge-bound relay exposes the stable VM endpoint.
- VMRelay now prefers `systemd-socket-activate` plus `systemd-socket-proxyd` for the relay, falls back to `socat`, and installs `socat` during apt-based host setup.
- Host setup no longer writes the VMRelay SSHD drop-in and removes the legacy VMRelay-managed drop-in when present and sshd validation allows it.
- Verified on `iron` that `192.168.122.1:<vm-port>` can reach a local workstation HTTP service through the new loopback reverse tunnel plus systemd socket proxy relay.

## 0.2.31 - 2026-05-05

- Hardened VM service mapping setup so VMRelay-managed SSH reverse forwards are scoped to the remote libvirt bridge address using `PermitListen <bridge-ip>:*` when the host supports it.
- Changed host readiness to distinguish bridge-restricted VM service forwarding from older bridge-bound forwarding, while VMRelay mappings themselves continue to bind only to the libvirt bridge address and not the public interface.
- Added SSH config validation/restore handling so VMRelay does not leave a bad sshd drop-in behind if the host rejects the bridge-restricted forwarding config.

## 0.2.30 - 2026-05-05

- Added a quiet 10-second background refresh on the host VM list so VM state changes appear without pressing `r`.
- Changed graceful shutdown to send an explicit ACPI shutdown request, wait briefly for libvirt state to change, and report when the guest is still running and force-off may be needed.

## 0.2.29 - 2026-05-05

- Made the Mappings page show the concrete guest-facing endpoint clearly: VMs connect to the VM endpoint column, normally `192.168.122.1:<vm-port>`, and VMRelay forwards that traffic back to `127.0.0.1:<local-service-port>` on the local workstation.
- Mapping start now performs a minimal host-side bootstrap by ensuring the libvirt `default` NAT network is defined/running/autostarted when possible and ensuring SSH reverse forwards can bind the VM bridge, with setup-focused errors if sudo-capable host setup is still required.
- Mapping start records the discovered bridge IP locally so the Mappings page continues to show the exact endpoint VMs should use.
- Fixed new and duplicated VM definitions so guest-initiated restarts use `<on_reboot>restart</on_reboot>` instead of shutting the domain off.
- Repaired persistent reboot lifecycle XML on `iron` for `Win10-Orig`, `Win11-Orig`, `Win10-Aardvark`, and `Win10-Bison`; currently running VMs will use the repaired lifecycle after their next stop/start because libvirt keeps the old value in live memory.

## 0.2.28 - 2026-05-05

- Changed host Mappings from local SSH forwards to VM-accessible service mappings: VMs connect to the host's stable libvirt bridge address, normally `192.168.122.1`, and SSH reverse-forwards that traffic back to `127.0.0.1` on the local workstation.
- Simplified the mapping form to name, local service port, and VM port; the remote bridge address is discovered from libvirt's `default` NAT network when the mapping starts.
- Changed host setup/readiness to cover VM service forwarding, including `GatewayPorts clientspecified` for SSH reverse forwards to the libvirt bridge.
- Verified on `iron` that a temporary reverse forward can bind `192.168.122.1` and carry HTTP back to a local workstation service, so every VM on the default NAT network can use the same stable endpoint.

## 0.2.27 - 2026-05-05

- Fixed console open after a VM's live VNC port changes: VMRelay now detects stale remote noVNC/websockify proxies and restarts them against the current libvirt VNC endpoint.
- Repaired the live `Win11-Orig` console proxy on `iron`, which was still forwarding to old VNC port `5904` while the running VM had moved to `5903`.

## 0.2.26 - 2026-05-05

- Fixed the VM duplicate name prompt so `q` is entered as normal text instead of quitting the app.
- Removed the misleading `q: quit` footer from the duplicate prompt and made the editable new-name field more explicit.

## 0.2.25 - 2026-05-05

- Changed new VM networking to default to an `e1000e` NIC on the selected libvirt NAT network instead of `virtio`, so stock Windows installers have a usable network adapter without virtio drivers.
- Changed manual NIC attach defaults and help text to use `e1000e` for the same Windows-compatible behavior.
- Changed UEFI VM creation to request Q35, Secure Boot with enrolled keys, and TPM 2.0 through `virt-install`; host setup/readiness now includes `swtpm` and Secure Boot-capable OVMF checks.
- Changed host setup to define/start/autostart the libvirt `default` NAT network when needed, and VM creation now starts an inactive selected network or reports a setup-focused error.
- Repaired `Win10-Orig` and `Win11-Orig` on `iron` by adding Windows-compatible NAT NICs; regenerated `Win11-Orig` as a Q35 Secure Boot/TPM VM and restarted it with reset NVRAM.

## 0.2.24 - 2026-05-05

- Fixed Windows installer VM creation more robustly by rewriting the created domain XML to use device-level CDROM boot order `1` and disk boot order `2`, instead of relying on generic `<os><boot dev='cdrom'/>` entries from `virt-install`.
- Restarted new UEFI installer VMs with reset NVRAM after applying boot order, then sent the initial boot key so Windows ISO prompts are caught reliably.
- Recovered `Win11-Orig` on `iron` by backing up its XML, applying device-level boot order, resetting NVRAM, restarting it, and verifying by screenshot that it reached the Windows 11 Setup product-key screen.

## 0.2.23 - 2026-05-05

- Made console opening more tolerant when `virsh domdisplay` does not return a clean VNC URL by falling back to the live libvirt XML `<graphics type='vnc'>` port.
- Improved console-open failures so stopped, missing, and running-without-live-VNC VMs report distinct actionable messages instead of the generic no-VNC error.
- Verified the VNC XML parser read the live VNC endpoint correctly on `iron` and ignored a shut-off VM's unallocated `port='-1'` graphics definition.

## 0.2.22 - 2026-05-05

- Added a VM detail Actions tab duplicate flow: press `d`, enter a new VM name, and VMRelay clones the powered-off VM with `virt-clone`.
- Normalized duplicated VM definitions so installer ISO media is ejected from the clone and disk boot order remains usable.
- Fixed CDROM/ISO detach from the Disks tab by ejecting media with `virsh change-media --eject` instead of treating the CDROM like a normal disk detach.
- Added host readiness reporting for `virt-clone`.

## 0.2.21 - 2026-05-05

- Added one-column left padding to the bottom status and key-help rows inside the TUI border.

## 0.2.20 - 2026-05-05

- Fixed VM creation boot behavior for installer media: new VMs now set CDROM before hard disk in the libvirt boot order, preserving UEFI when selected.
- Sent an initial boot key after `virt-install` starts the guest so Windows installer ISOs do not time out at the short “press any key to boot from CD/DVD” prompt before the noVNC console is opened.
- Recovered the newly created `Win10-Orig` VM on `iron` by backing up its XML, setting CDROM boot order `1` and disk boot order `2`, restarting it, and sending the initial boot key; screenshots showed the Windows installer media loading instead of the UEFI shell.

## 0.2.19 - 2026-05-05

- Improved the create-VM name field so normal VM names can be up to 80 characters instead of feeling visually capped by the narrow form row.
- Changed active create-wizard text fields to show the right-hand side of long values, so typing continues visibly on narrow terminals.
- Bounded internal storage volume names with a short hash so long VM names do not create oversized libvirt volume filenames.

## 0.2.18 - 2026-05-05

- Hardened startup self-update when sudo prompts for a password: VMRelay now repairs terminal mode with `stty sane` and runs the installer with stdin/stdout/stderr attached directly to `/dev/tty`.
- Investigated the reported stuck sudo prompt; no live local or onward-SSH `sudo`/`vmrelay` process was visible by the time of inspection, so the fix targets the remaining likely terminal-state failure mode.

## 0.2.17 - 2026-05-05

- Fixed VM creation when staging a user-home ISO into libvirt storage: `virsh vol-upload` progress output is now suppressed so VMRelay captures only the final staged ISO path for `virt-install`.
- Removed the redundant `virt-install --wait 0` flag that produced a harmless warning before real failures.
- Improved visible error summaries so warnings after `exit status 1` are skipped in favor of the first real failure line.
- Read-only inspection on `iron` found the failed create attempt handed `virt-install` an ISO path with a leading embedded newline, causing validation under `/home/simplehelp/.../var/lib/libvirt/images`.

## 0.2.16 - 2026-05-05

- Fixed startup self-update when the installer needs a sudo password: accepting an update now exits the TUI first, runs the installer in the normal terminal with stdin/stdout/stderr attached, then restarts VMRelay.
- Updated the update prompt/help text to make the terminal handoff explicit.
- Added a regression test that accepting the update prompt quits for the terminal installer handoff instead of running the installer inside the TUI.

## 0.2.15 - 2026-05-05

- Changed host setup to initialize a VMRelay-managed libvirt directory storage pool named `vmrelay` at `/var/lib/vmrelay/images`, start it, and mark it for autostart.
- Changed host readiness checks to report whether the VMRelay storage pool is running, falling back to reporting the first active libvirt pool on older hosts.
- Changed VM creation to prefer the `vmrelay` storage pool before existing `images` or `default` pools, and to tell the user to run host setup when no running storage pool exists.

## 0.2.14 - 2026-05-05

- Fixed VM creation on hosts where the SSH user can manage libvirt but does not have passwordless sudo: VMRelay now creates the boot disk through a running libvirt storage pool instead of `sudo qemu-img` in `/var/lib/libvirt/images`.
- Fixed VM creation from user-home ISO paths such as `~/Documents/...iso` by staging the ISO into libvirt-readable storage when the original path is outside the selected storage pool.
- Changed creation-time ownership recording to be non-fatal when `/var/lib/vmrelay/ownership.tsv` is not writable; the VM can still be created and VMRelay reports the ownership warning.
- Improved visible error summaries so a remote stderr line, such as `sudo: a password is required`, appears with `exit status 1` instead of being hidden behind the generic status.
- Verified on `iron` that the selected libvirt storage pool is `images` at `/var/lib/libvirt/images`, `simplehelp` has no passwordless sudo, and the selected Windows ISO lives under a home directory that qemu cannot traverse directly.

## 0.2.13 - 2026-05-05

- Changed the create-VM ISO default from `/var/lib/libvirt/boot/` to the remote user's `~/Documents/`, falling back to the remote home directory if `Documents` is missing.
- Allowed ISO paths beginning with `~/` in the create workflow and expanded them on the remote host before validating/executing `virt-install`.
- Verified read-only on `iron` that `/home/simplehelp/Documents` exists and contains an ISO visible to the picker.

## 0.2.12 - 2026-05-05

- Improved the create-VM wizard with up/down field navigation, left/right preset cycling for memory, CPUs, disk size, disk bus, firmware, and shared/private choices.
- Changed the shared field from free text to a Yes/No selection, rendering as `Yes - shared` or `No - private`.
- Added a remote ISO picker opened from the ISO path field, starting at `/var/lib/libvirt/boot/` and listing directories plus `.iso` files from the selected SSH host.
- Verified the ISO picker listing behavior against `iron` and fixed the empty-directory case so `/var/lib/libvirt/boot/` can open even before any ISO files are present.
- Added focused tests for create wizard keyboard behavior, constrained fields, ISO picker parsing/rendering, and ISO selection.

## 0.2.11 - 2026-05-05

- Made VM creation discoverable from the host VMs tab: `n` now opens the create-VM form from either VMs or Config, the VMs footer advertises the shortcut, and the empty VM list shows a create hint.
- Added a focused test for opening the create-VM form from the host VMs tab.

## 0.2.10 - 2026-05-05

- Changed generated noVNC console URLs to enable noVNC's local pointer dot and lower-latency browser settings by default, so mouse control has immediate local visual feedback even if the guest cursor repaint is slow.
- Changed VM creation to add a USB tablet input device to new graphical guests for better absolute-pointer behavior.
- Operational note: fixed the imported `Draytek_VPN_virtualisation_server` on `iron` by backing up its libvirt XML, switching the VM definition from BIOS to OVMF/UEFI, preserving the SATA boot disk order, and restarting it; screenshots confirmed Windows progressed to `Getting devices ready`.

## 0.2.9 - 2026-05-05

- Added host-level VM creation from the Config tab using a remote ISO path, `virt-install`, a newly created qcow2 boot disk, VNC graphics, libvirt networking, selectable disk bus, and BIOS/UEFI firmware selection.
- Added creation-time VMRelay ownership recording so newly created VMs are owned by the current remote SSH account and can optionally be marked shared.
- Updated apt-based host setup/readiness checks to include OVMF/UEFI firmware support.
- Added tests for VM creation form rendering and request validation.

## 0.2.8 - 2026-05-05

- Added a VM detail Disks action to set the selected disk as the VM's first boot disk in persistent libvirt configuration.
- Updated the Disks footer/help and README so imported boot disks can be made bootable from the TUI after import/conversion.

## 0.2.7 - 2026-05-05

- Fixed browser console launch so an occupied preferred local console port no longer fails the operation; VMRelay now scans forward to the next available local port and reports the adjusted URL.
- Added a focused test for local console port fallback behavior.

## 0.2.6 - 2026-05-05

- Added a VM detail screen opened from the host VM list with Summary, Disks, NICs, and Actions tabs.
- Added VM detail inventory for UUID, state, owner/shared status, CPU count, memory, autostart, graphics display, attached disks, and network interfaces.
- Added disk creation from the VM detail Disks tab using `qemu-img create` plus persistent libvirt disk attachment.
- Added disk import from remote host paths with `qemu-img info` format detection, automatic conversion of non-qcow2 sources to qcow2, persistent attachment, and safe detach without deleting image files.
- Added NIC attach/detach actions for libvirt network interfaces from the VM detail NICs tab.
- Added tests for VM detail parsing, disk/NIC tab rendering, and disk import validation.

## 0.2.5 - 2026-05-05

- Removed the in-pane Hosts theme button so theme selection is only exposed through the bottom controls row.
- Removed inner pane line borders so the TUI uses one outer rounded frame around the full screen.
- Added a host detail screen with VMs, Config, and Mappings tabs.
- Added local per-host port mapping configuration in the host detail screen, including add/remove plus SSH local-forward start/stop actions.
- Added tests for the single-frame layout, mapping rendering, and mapping validation.

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
