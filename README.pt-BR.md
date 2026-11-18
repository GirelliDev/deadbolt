# Deadbolt

Crie, configure e mantenha atualizado um servidor dedicado de **Project Zomboid**, com um único binário (`dbolt`).
Build 42, Linux, systemd. Um binário em Go, sem dependências em runtime.

```
dbolt -help
```

## O que faz

| Comando | |
|---|---|
| `dbolt status` / `start` / `stop` / `restart` | Estado, jogadores (RCON) e nº de mods. Stop/restart salvam o mundo antes. |
| `dbolt backup create\|list\|restore\|prune` | `.tar.zst` de Saves, configs do servidor e db. Retenção diária/semanal. O restore guarda as pastas substituídas como `*.pre-restore-*`. |
| `dbolt install` | Instala o servidor pelo SteamCMD (`-branch public\|unstable`). |
| `dbolt update check` | Há build nova do jogo ou mod do Workshop mais novo? |
| `dbolt update` / `-wait` / `-force` | Aplica. Recusa com jogadores online; `-wait` espera o servidor esvaziar. Sempre faz backup antes e volta atrás se o servidor não subir. |
| `dbolt update watch` | Fica verificando e atualiza **só quando não há ninguém online** (2 checagens vazias seguidas). |
| `dbolt config show\|get\|set` | Edita o `<server>.ini`. Mantém comentários e quebras de linha, esconde segredos. |
| `dbolt config sandbox get\|set` | Edita as regras de jogo, ex.: `ZombieLore.Speed`. |
| `dbolt mods list\|add\|remove` | Gerencia itens do Workshop e os mods que eles trazem. |
| `dbolt web` / `web passwd` | Painel admin: controle do servidor, jogadores, dar itens, kick/ban/unban, teleporte, aviso geral, backups e updates. |

Edições são recusadas com o servidor rodando (ele sobrescreveria), a menos que você use `-force`.

## Painel admin

```
dbolt web passwd      # imprime uma senha de admin gerada, uma única vez
dbolt web             # http://127.0.0.1:8787
```

Escuta só em loopback por padrão. Admin único, senha com bcrypt, cookie de sessão (`HttpOnly`, `SameSite=Strict`),
bloqueio após tentativas erradas, checagem de header contra CSRF, CSP estrita e log de auditoria de toda ação.
Para acessar de fora, ponha um proxy reverso com HTTPS na frente.
Tudo que é digitado no painel passa por uma lista de permissão estrita antes de virar comando RCON.

Ainda fora do painel: curar e matar. A RCON do Project Zomboid não tem esses comandos; precisam de um mod Lua de servidor (roadmap).

## Instalação

```
git clone https://github.com/GirelliDev/deadbolt && cd deadbolt
make build && sudo install bin/dbolt /usr/local/bin/
sudo mkdir -p /etc/deadbolt && sudo cp deadbolt.example.toml /etc/deadbolt/deadbolt.toml
```

Os padrões assumem um servidor instalado em `/home/pz`. Veja `deadbolt.example.toml` e `contrib/` para as units do systemd.
A RCON precisa de `RCONPort` e `RCONPassword` no `.ini` do servidor (reinicie o servidor depois de definir).

## Roadmap

Geração do primeiro boot e da unit no `dbolt install`, mod de curar/matar, papéis no painel,
backup externo, vários servidores, Build 41.

Project Zomboid é marca da The Indie Stone. O Deadbolt é uma ferramenta comunitária não oficial.

## Licença

MIT
