# Deadbolt

Crie, configure e mantenha atualizado um servidor dedicado de **Project Zomboid**, com um único binário.

> Status: em desenvolvimento inicial. Somente Build 42.

```
dbolt -help
dbolt status
dbolt backup
dbolt update      # espera o servidor ficar vazio
```

## Previsto para o MVP

- Instalar e atualizar o servidor pelo SteamCMD (branch `stable` / `unstable`)
- start / stop / restart / status pelo systemd
- Editar `servertest.ini` e `SandboxVars.lua`
- Gerenciar mods do Workshop
- Backup e restore (`.tar.zst`, retenção diária/semanal)
- Atualização automática de jogo e mods, aplicada só com o servidor vazio, com rollback
- Painel web mínimo (localhost por padrão) com ações de admin: dar item, kick, ban, teleporte

Project Zomboid é marca da The Indie Stone. O Deadbolt é uma ferramenta comunitária não oficial.

## Licença

MIT
