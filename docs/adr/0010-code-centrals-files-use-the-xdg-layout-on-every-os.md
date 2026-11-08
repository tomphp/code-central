# Code Central's files use the XDG layout on every OS

Code Central's files have always used the XDG layout: the store in `~/.local/share/code-central`, the settings in `~/.config/code-central/settings.yaml` and the Factory's state in `~/.local/state/code-central-factory/<project>.json`. macOS already uses this layout rather than `~/Library/Application Support`, so the same layout covers macOS and Linux. Windows usually has no `HOME`, so porting Code Central there meant choosing where its files go. We decided they use the same XDG layout under the account's profile folder: `%USERPROFILE%\.local\share\code-central` and so on. Each default path is the matching `XDG_*` variable, else under `HOME`, else under the profile folder from `os.UserHomeDir`. One lookup in the `home` package resolves every default path, for the store, the settings, the Factory's state file and the service's files. `CODE_CENTRAL_STORE` and `--store` still win for the store. The service's unit goes where each service manager looks for it (`~/Library/LaunchAgents`, with its log in `~/Library/Logs`, on macOS, and the systemd user folder on Linux). The Windows service (added by code-central-151) keeps its log in `~\.local\state\code-central\code-central.log`, found through the same lookup.

## Considered Options

- **Windows-native folders, `%LOCALAPPDATA%` for the store and state and `%APPDATA%` for the settings.** Rejected in favour of one rule everywhere: the docs, the error messages and people's habits carry across machines, and the overrides (`XDG_DATA_HOME`, `XDG_CONFIG_HOME`, `HOME`) mean the same thing on every OS.
- **`~/Library` on macOS.** Never used: macOS has been on the XDG layout from the start, and changing it now would move people's stores.

## Consequences

A Windows user finds their files somewhere Windows apps don't usually put them, but where the docs say they are. Moving a store later, to Windows-native folders or anywhere else, would mean migrating people's data, so this layout is meant to stay. On macOS and Linux nothing moves when `HOME` is set. `XDG_STATE_HOME` isn't read, so the Factory's state file stays where it always was.
