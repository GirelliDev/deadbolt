# Deadbolt

Create, configure and keep a **Project Zomboid** dedicated server up to date, from one small binary (`dbolt`).
Build 42, Linux, systemd. One Go binary, no runtime dependencies.

```
dbolt -help
```

## What it does

| Command | |
|---|---|
| `dbolt status` / `start` / `stop` / `restart` | State, players (RCON) and mod count. Stop/restart save the world first. |
| `dbolt backup create\|list\|restore\|prune` | `.tar.zst` of Saves, Server settings and db. Only the latest is kept: a new backup replaces the previous one. Restore keeps the replaced folders as `*.pre-restore-*`. |
| `dbolt install` | Install the server with SteamCMD (`-branch public\|unstable`). |
| `dbolt update check` | Is there a new game build or a newer Workshop mod? |
| `dbolt update` / `-wait` / `-force` | Apply it. Refuses while players are online; `-wait` waits for an empty server. Always backs up first and rolls back if the server does not come back. |
| `dbolt update watch` | Keep checking and update **only when nobody is online** (2 empty checks in a row). |
| `dbolt config show\|get\|set` | Edit `<server>.ini`. Comments and line endings are kept, secrets are hidden. |
| `dbolt config sandbox get\|set` | Edit gameplay settings, e.g. `ZombieLore.Speed`. |
| `dbolt mods list\|add\|remove` | Manage Workshop items and the mod ids they provide. |
| `dbolt web` / `web passwd` | Admin panel: server control, players, give items, kick/ban/unban, teleport, broadcast, backups, updates. |

Edits are refused while the server runs (it would overwrite them) unless you pass `-force`.

## Admin panel

```
dbolt web passwd      # prints a generated admin password once
dbolt web             # http://127.0.0.1:8787
```

It listens on loopback only by default. Single admin, bcrypt password, session cookie (`HttpOnly`, `SameSite=Strict`),
login lockout, CSRF header check, strict CSP, and an audit log of every action in the state dir.
To reach it from outside, put an HTTPS reverse proxy in front.
Everything typed in the panel is validated against a strict allowlist before it becomes an RCON command.

Heal and kill are not offered: Project Zomboid's RCON has no commands for them.

## Install

```
git clone https://github.com/GirelliDev/deadbolt && cd deadbolt
make build && sudo install bin/dbolt /usr/local/bin/
sudo mkdir -p /etc/deadbolt && sudo cp deadbolt.example.toml /etc/deadbolt/deadbolt.toml
```

Defaults match a server installed in `/home/pz`. See `deadbolt.example.toml` and `contrib/` for systemd units.
RCON needs `RCONPort` and `RCONPassword` set in the server `.ini` (restart the server after setting them).

## Roadmap

First-run server generation and unit creation in `dbolt install`, multi-user panel roles,
off-site backups, multiple servers, Build 41.

Project Zomboid is a trademark of The Indie Stone. Deadbolt is an unofficial community tool.

## License

MIT
