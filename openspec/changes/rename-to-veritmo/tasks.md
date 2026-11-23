# Tasks: rename-to-veritmo

Commit messages in Spanish. Start from a clean tree with no other change half implemented. Every task names how to verify it. Group 5 is the owner's.

## 1. The mechanical rename (one commit; the tree never stops building)

- [ ] 1.1 `go.mod`: `module veritmo`. `sed -i 's#"mediaOrquestator/#"veritmo/#g'` over every `*.go`; `gofmt -l .` prints nothing; `go build ./...` passes.
- [ ] 1.2 `git mv cmd/orchestrator cmd/veritmo`; the binary name in `Dockerfile` (build output, `COPY`, the `veritmo-api` symlink), `scripts/serve.sh` (`BIN=bin/veritmo-api`, `go build … ./cmd/veritmo`, the `/proc` regex `veritmo(-api)? serve`), `scripts/install.sh`, `.gitea/workflows/migrar.yml`, `serve.sh`, `bin/.gitignore` if it names the binary. Verify: `bash scripts/serve.sh` builds `bin/veritmo-api` and starts; `veritmo --version` and `veritmo-api --version` print `veritmo`.
- [ ] 1.3 Settings paths: `settings.ConfigDir()` → `veritmo`; cache dir → `veritmo`; every literal `mediaorquestator` in `internal/` and `cmd/` (settings, references, media, motion tests, musicsvc Docker specs, ctxpress `client`, the `openapi.yaml` title) → `veritmo` / `veritmo/<image>` / `vt-<engine>`. The Docker test that compares server and `scripts/music-containers.sh` keeps passing after the script changes too. Verify: `go test ./...`; `grep -rIn "mediaorquestator\|mediaOrquestator" internal cmd` prints only the migration and fallback code of group 2 (none yet at this point).
- [ ] 1.4 Environment variables: every `MO_<X>` in `internal/`, `cmd/`, `compose.yaml`, `compose.services.yaml`, `deploy/compose/*`, `.gitea/workflows/migrar.yml`, `scripts/*.sh` and tests → `VERITMO_<X>`. Verify: `grep -rn "\bMO_[A-Z]" --exclude-dir=.git --exclude-dir=.orchestrator .` prints nothing (the fallback comes in 2.2).
- [ ] 1.5 Docker and compose: `Dockerfile` comments and image tags, `compose.yaml`/`compose.services.yaml` `name: veritmo` and `veritmo/<x>:${VERSION}`, `deploy/compose/*` (prometheus, grafana datasources/dashboards, mysql users), `scripts/compose-init.sh`, `scripts/music-containers.sh` (`vt-<engine>`). Verify: `docker compose -f compose.yaml config` resolves.
- [ ] 1.6 Metrics: the family prefix in `internal/server` (`veritmo_`), its tests, `deploy/compose/prometheus*.yml` rules if any name a family. Verify: metrics test asserts `veritmo_info` and no `mediaorquestator_`.
- [ ] 1.7 `sonar-project.properties` key `veritmo`; `NOTICE`, `THIRD_PARTY.md`, `README.md` (first line: the tagline *Veritmo — ver el ritmo* and one sentence), `ESTADO.md`, `CONTRIBUTING.md`, `SECURITY.md`, `.github/*`, `.claude/skills/**`, `dotnet/lion-clap/**` docs and `CLAUDE.md`, `docker/*/README.md` and `fetch.py` strings (text only; no Python logic). Verify: `grep -rIil "mediaorquestator" --exclude-dir=.git --exclude-dir=.orchestrator --exclude-dir=third_party --exclude-dir=node_modules . | grep -v '^docs/'` prints nothing.

Commit: `Veritmo: nuevo nombre del proyecto en módulo, binario, rutas, variables, imágenes, métricas y textos`

## 2. Migration and fallbacks (second commit)

- [ ] 2.1 `internal/platform/settings`: on the first `ConfigDir()`/`CacheDir()` call, rename `~/.config/mediaorquestator` → `~/.config/veritmo` (and the cache) when only the old exists, log one line; warn once when both exist; never copy or delete. Honour `XDG_*`. Verify: tests with a temp `XDG_CONFIG_HOME` for the three scenarios of `project-identity`; the settings, providers and motion tests that create the old directory by hand are switched to the new one.
- [ ] 2.2 Environment fallback: a helper `env("DB_HOST")` that reads `VERITMO_DB_HOST`, then `MO_DB_HOST` with a once-per-variable warning `MO_DB_HOST is obsolete, use VERITMO_DB_HOST`; used for every variable of 1.4; `TODO(veritmo-env)` where the fallback is to be removed. Verify: `env_test` with old only, new only, both.
- [ ] 2.3 `scripts/serve.sh` and `scripts/install.sh`: before reading token/media-dir/web-dir/pid/log, `mv` the old directory to the new when only the old exists, echo what moved. Verify: `scripts/test/` case with a fake `HOME` holding the old directory; the server started by the script finds the token.

Commit: `Veritmo: el directorio de configuración antiguo se mueve una vez y las variables MO_* siguen leyéndose con aviso`

## 3. Docs, Grafana and specs

- [ ] 3.1 `docs/**`: the name, the command, the directories, the variables, the images; `docs/grafana/mediaorquestator-*.json` → `veritmo-*.json` regenerated with `docs/grafana/build_dashboard.py` and the datasource/dashboard provisioning in `deploy/compose/grafana` pointing at them; `docs/INSTALL.md` gets the two lines on what moves on update (the directory, the variables). Verify: `scripts/check-docs.sh`; `grep -rIil "mediaorquestator" docs` prints nothing.
- [ ] 3.2 Specs textual pass (the archive step): `sed` over `openspec/specs/**/spec.md` for `` `orchestrator `` → `` `veritmo ``, `orchestrator-api` → `veritmo-api`, `mo-<engine>` → `vt-<engine>`; then `openspec archive rename-to-veritmo` applies the ten deltas and adds `project-identity`. Verify: `grep -rn "orchestrator \|mediaorquestator\|MO_DB" openspec/specs` prints nothing; `openspec validate` passes.

Commit: `Docs y especificaciones con el nombre Veritmo; paneles de Grafana regenerados`

## 4. Verification on the server

- [ ] 4.1 `go vet ./... && go test ./...`; `docker compose build` of `veritmo/server` and `veritmo/tools`; `docker compose up -d`; `curl -H "Authorization: Bearer $(cat ~/.config/veritmo/token)" :4666/api/metrics | head` shows `veritmo_info`; the music services start as `vt-heartmula` / `vt-diffrhythm2`; Grafana shows the renamed panels with data.
- [ ] 4.2 Remove the old `mo-*` containers and `mediaorquestator/*` images (`docker rm`, `docker rmi`) once the new ones run; note the commands in `docs/INSTALL.md` under the update note.

## 5. Owner's tasks (outside the repository)

- [ ] 5.1 Rename the repositories: Gitea `personales/mediaOrquestator` → `personales/veritmo`; GitHub `privadosIA/mediaOrquestator` → `privadosIA/veritmo`. Both redirect the old name. Set the description to the tagline: *Veritmo — ver el ritmo. A Go server that turns a song into typographic motion graphics.*
- [ ] 5.3 Any cron, script or shell alias outside the repo that reads `~/.config/mediaorquestator/token` or runs `orchestrator`: update to the new path and name (the migration log tells where things went).
- [ ] 5.4 Apply `rename-server-veritmo` in `dashboards-labs` after the server's `openapi.yaml` is published (its `contract:sync` reads the new title).
