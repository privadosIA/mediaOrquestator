# Proposal

## Why

The music engines are now .NET services that `scripts/get-services.sh` publishes into `bin/services`. That works on this machine, but installing them on another one (the laptop: i7, 32 GB, one RTX 4080 of 12 GB, CachyOS) raises questions that were not asked while the engines were containers, and the song recognition added three methods (Whisper, LAION-CLAP, MuQ) whose requirements differ. The way to install them is not decided yet; this item collects what has to be decided and measured.

## What Changes (to be decided)

Nothing yet. This is a plan, not an implementation. The options to weigh are in `design.md`.

## Impact

`scripts/get-services.sh`, `scripts/install.sh`, `docs/INSTALL.md`, the Models and Environment screens, and whatever the chosen option adds.
