# Proposal: rename-to-veritmo

## Why

The project is called `mediaOrquestator`. "Media Orchestrator" is a generic term of the broadcast industry (Quickplay and Xytech sell products with that name, MPEG has a "Media Orchestration" standard), so the name is impossible to find and sits next to other people's trademarks in the same sector. Now that the repositories are public and nobody depends on the name yet is the cheapest moment to change it.

The new name is **Veritmo**: seven letters, no accent, pronounced the same in Spanish and English (ve-RIT-mo), and it reads as *ver + ritmo*, "see the rhythm", which is what the server does: it puts the rhythm of a song on the screen as type. Checked on 2026-11-20: no product, company, repository or community uses it; the nearest neighbours are *Veritimo* (an IT consultancy) and *verismo* (the art movement).

## What changes

Everything that carries the old name, in one change, so that no half-renamed state is ever committed:

- **Go module** `mediaOrquestator` → `veritmo`, and every import path with it.
- **Binary** `orchestrator` (and the `orchestrator-api` alias) → `veritmo` (alias `veritmo-api`); `cmd/orchestrator/` → `cmd/veritmo/`.
- **User directories** `~/.config/mediaorquestator/` → `~/.config/veritmo/` and `~/.cache/mediaorquestator/` → `~/.cache/veritmo/`, with an automatic one-time migration so that nobody loses the token, the settings, the keys or the models-dir file.
- **Environment variables** `MO_*` → `VERITMO_*`; the old names are still read for one release with a warning naming the new one.
- **Docker**: images `mediaorquestator/<x>` → `veritmo/<x>`, compose project `mediaorquestator` → `veritmo`, music containers `mo-<engine>` → `vt-<engine>`.
- **Prometheus metrics** `mediaorquestator_*` → `veritmo_*`, and the Grafana dashboards in `docs/grafana/` with them.
- **Texts and identifiers**: the OpenAPI title, the `client` the server sends to ctxpress, the Sonar project key, `README`, `ESTADO`, `NOTICE`, `THIRD_PARTY`, `docs/`, the `.gitea` workflow, the skills under `.claude/skills/`, `dotnet/lion-clap` docs, and `scripts/` (`serve.sh`, `install.sh`, `sync-public.sh`, `publish-github.sh`).
- **Specifications**: the ten requirements that name the directory, the variables, the images or the metrics are modified here; the command name `orchestrator` in the other specs is a pure spelling change and is applied as a textual pass when this change is archived (see design).
- **Repositories** (owner's task): Gitea `personales/mediaOrquestator` → `personales/veritmo`, GitHub `privadosIA/mediaOrquestator` → `privadosIA/veritmo`, the public worktree folder, and the remotes on the Mac and the server. Both hosts redirect the old name.

The dashboards workspace has its own small change (`rename-server-veritmo` in `dashboards-labs`) for the texts, the contract source, the PWA manifest and the Tauri identifier; the workspace itself keeps its name.

## What does not change

- The licence and the copyright line (`Copyright (c) 2026 Jesús Suero`).
- The API: no route, field or behaviour changes; only the OpenAPI `title`.
- Database schema, settings keys, file formats, the parts catalog, the pieces.
- The `.orchestrator/` folder at the repository root, which belongs to the coding tool, not to the project.

## Capabilities

- `project-identity` (ADDED): the name, the module, the binary, the directories, the variables, the images, the metrics, and the migration.
- `database-choice`, `cli`, `installer`, `ollama-settings`, `models-directory`, `prompt-fetch`, `prometheus-metrics`, `music-docker-option`, `model-providers` (MODIFIED: the name inside the requirement).

## Dependencies

None on other changes; apply when the working tree is clean and no other change is half implemented, because it touches every Go file.
