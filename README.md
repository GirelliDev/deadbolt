# Deadbolt

Create, configure and keep a **Project Zomboid** dedicated server up to date, from one small binary.

> Status: early development. Build 42 only.

```
dbolt -help
dbolt status
dbolt backup
dbolt update      # waits until the server is empty
```

## Planned for the MVP

- Install and update the server through SteamCMD (`stable` / `unstable` branch)
- start / stop / restart / status via systemd
- Edit `servertest.ini` and `SandboxVars.lua`
- Manage Workshop mods
- Backups and restore (`.tar.zst`, daily/weekly retention)
- Automatic game and mod updates, applied only when nobody is online, with rollback
- Minimal web panel (localhost by default) with admin actions: give items, kick, ban, teleport

Project Zomboid is a trademark of The Indie Stone. Deadbolt is an unofficial community tool.

## License

MIT
