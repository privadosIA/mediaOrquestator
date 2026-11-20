## 1. Decide

- [x] 1.0 Decided instead: the source of the ports is vendored in `dotnet/` (no tests fixtures, revisions in REVISIONS.txt); `install.sh` has the `dotnet` and `services` components

- [x] 1.1 Source of the binaries: built on the machine from the vendored source (decided 2026-11-15; see design)
- [x] 1.2 Sharing the native libraries between the three services: hard links after building, 12.3 of 18.8 GB saved (see design, "Decided"); `get-services.sh` does it, `--share-only` for an existing installation
- [x] 1.3 Installer tiers (none, recognition, generator, all) and what each builds: decided and implemented in `install.sh` (see design)
- [x] 1.4 MuQ: the audio half is ported (diffrhythm2 embed/classify); the app runs `classify` for "muq" in recognition

## 2. Measure on the laptop

- [ ] 2.1 HeartMuLa on one 12 GB card (isolation modes), DiffRhythm2 and CLAP alone and in pairs
- [ ] 2.2 Time and disk of the whole installation from nothing

## 3. Build

- [ ] 3.1 Whatever 1.x decides: `get-services.sh`/`install.sh`, the Models screen (a card for the services), INSTALL.md — done: `get-services.sh` (sharing, `--only` lists), `install.sh` (tiers), INSTALL.md; open: the card for the services on the Models screen (dashboards-lab, another repository)
