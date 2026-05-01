# Changelog

## Unreleased

- Added the initial Bash implementation at `bin/vmrelay`.
- Added `README.md` with a one-line private GitHub install command.
- Added `.gitignore` so Lore metadata and `TRACKING.md` stay out of Git.
- Implemented local host config commands, inbound/outbound mapping commands, managed SSH tunnel start/stop/status, remote Ubuntu setup, and self-update support.
- Updated the Lore design doc with MVP tunnel ownership and config reconciliation decisions.
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
