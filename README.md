# VMRelay

VMRelay makes it easy to use a remote Linux box as a VM server without fully converting that machine into a dedicated virtualization appliance like Proxmox. It keeps the remote box as a normal Linux host, then uses Bash, OpenSSH, KVM/libvirt, and Cockpit to provide a private web UI and tunnelled access from your local Linux or macOS machine.

VMRelay sets up KVM/libvirt/Cockpit on a remote Ubuntu host, keeps Cockpit private on the remote loopback interface, creates local SSH forwards for the web UI and configured TCP mappings, and can expose VM graphical consoles through a local browser using noVNC.

## Install

Install the latest public VMRelay script:

```bash
curl -fsSL -H "Accept: application/vnd.github.raw" "https://api.github.com/repos/brontoguana/vmrelay/contents/bin/vmrelay?ref=main" | sudo tee /usr/local/bin/vmrelay >/dev/null && sudo chmod +x /usr/local/bin/vmrelay && vmrelay --version
```

## Quick Start

```bash
vmrelay addhost box1 user@remote-box
vmrelay setup box1
vmrelay status box1
```

For the first host, Cockpit is forwarded to:

```text
http://127.0.0.1:4400
```

Additional hosts get stable ports from the same base:

```text
box1 -> http://127.0.0.1:4400
box2 -> http://127.0.0.1:4401
box3 -> http://127.0.0.1:4402
```

Remote Cockpit is configured to listen only on `127.0.0.1:9090` on the remote host. VMRelay automatically forwards it locally during `setup`, `up`, and `status`.

## Commands

```bash
vmrelay addhost NAME SSH_TARGET
vmrelay removehost HOST
vmrelay hosts
vmrelay host HOST
vmrelay inbound HOST LOCAL_PORT REMOTE_PORT
vmrelay outbound HOST LOCAL_PORT REMOTE_PORT
vmrelay console HOST VM_NAME [LOCAL_PORT]
vmrelay console-down HOST VM_NAME
vmrelay setup HOST
vmrelay up HOST
vmrelay resume
vmrelay status HOST
vmrelay down HOST
vmrelay tail [LINES]
vmrelay update
```

## Port Mappings

Inbound mappings expose a local service to remote VMs through the remote libvirt bridge:

```bash
vmrelay inbound box1 8080 18080
```

Result:

```text
remote VM 192.168.122.1:18080 -> local 127.0.0.1:8080
```

Outbound mappings expose a remote host service locally:

```bash
vmrelay outbound box1 15432 5432
```

Result:

```text
local 127.0.0.1:15432 -> remote 127.0.0.1:5432
```

If a VMRelay-managed tunnel is already running, mapping commands update the host config and restart that host's managed tunnel so the change applies immediately.

## VM Consoles

VMRelay can expose a libvirt VM's graphical console in a local browser without requiring RDP, SSH, or any agent inside the guest:

```bash
vmrelay console box1 Draytek_VPN_virtualisation_server 4500
```

Result:

```text
Console URL: http://127.0.0.1:4500/vnc.html?autoconnect=1&resize=scale
Browser: requested local console URL
```

This uses the VM's libvirt VNC display on the remote host, starts noVNC/websockify on remote `127.0.0.1`, and forwards that browser console to the local machine over SSH. It works for Linux and Windows guests because the connection is to the virtual display, not to services inside the guest OS.

VMRelay opens the console URL automatically with the local OS default browser on macOS, Linux, WSL, and Windows shell environments when a suitable browser launcher is available. To suppress this for scripts or manual use:

```bash
VMRELAY_OPEN_BROWSER=0 vmrelay console box1 Draytek_VPN_virtualisation_server 4500
```

If `LOCAL_PORT` is omitted, VMRelay chooses a stable local port from the host and VM name.

If a VM was created with SPICE graphics, VMRelay can switch it to VNC automatically when the VM is shut down. For a running SPICE-only VM, shut the VM down and rerun the console command.

To stop the console tunnel and remote noVNC proxy:

```bash
vmrelay console-down box1 Draytek_VPN_virtualisation_server
```

## Tunnel Maintenance

```bash
vmrelay resume
```

`vmrelay resume` starts or reconciles tunnels for every configured host. It is intended as the command that an OS-level user service can run at login or on an interval.

## Configuration

Host configs live in:

```text
~/.config/vmrelay/hosts.d
```

Runtime state lives in:

```text
~/.local/state/vmrelay
```

Each host config stores the SSH target, stable Cockpit web port offset, VM bridge address, and configured inbound/outbound mappings.

Operational logs and the command lock live in:

```text
~/.vmrelay
```

Commands that change config or tunnel state take a per-user lock so concurrent VMRelay runs do not race each other. The default lock wait timeout is 300 seconds and can be changed with `VMRELAY_LOCK_TIMEOUT`.

To show the latest VMRelay log entries:

```bash
vmrelay tail
```

`vmrelay tail` shows the latest 200 log lines by default. Pass a number to choose a different count.

## Update

```bash
vmrelay update
```

`vmrelay update` downloads the latest `bin/vmrelay` script from the public GitHub contents API using `curl`. It validates the downloaded Bash script, keeps a backup of the previous installed script, and does not change host configs, tunnels, or remote machines.
