# VMRelay

VMRelay is a local Bash/OpenSSH command for managing remote Ubuntu VM hosts from Linux or macOS. It sets up KVM/libvirt/Cockpit on a remote host, keeps Cockpit private on the remote loopback interface, and creates local SSH forwards for the web UI and configured TCP mappings.

## Install

Install the latest public VMRelay script:

```bash
curl -fsSL https://github.com/brontoguana/vmrelay/raw/refs/heads/main/bin/vmrelay | sudo tee /usr/local/bin/vmrelay >/dev/null && sudo chmod +x /usr/local/bin/vmrelay && vmrelay --version
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

`vmrelay update` downloads the latest `bin/vmrelay` script from the public GitHub raw URL using `curl`. It validates the downloaded Bash script, keeps a backup of the previous installed script, and does not change host configs, tunnels, or remote machines.
