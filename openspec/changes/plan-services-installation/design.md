# Open questions

## What is known

- Each published service is about 5.8 GB, almost all of it the CUDA build of TorchSharp (libtorch, cuBLAS, cuDNN...), and the three are identical in that part: 17 GB on disk for what is one set of libraries. Weights are separate (HeartMuLa about 9 GB, DiffRhythm2 about 7.7 GB converted, CLAP about 600 MB) and live in `models/music/`.
- Building needs the .NET SDK and the sibling repositories (`heartmula`, `diffrhythm2`, `lion-clap`, `musicgen-core`); running needs only the .NET runtime (8 or newer; `RollForward=Major`).
- DiffRhythm2 needs the espeak-ng library (1.51 is the reference, `bin/libespeak-ng.so.1` is the one in use) and its weights are converted in C# by `diffrhythm2 install --accept-licenses`; MuQ-MuLan's weights are CC-BY-NC.
- HeartMuLa's model and codec did not work together on one 12 GB card in the service (they do in the CLI, one after the other). On this machine the codec goes to the other card.
- MuQ-MuLan's audio half is in the .NET port now (`diffrhythm2 embed`/`classify`, task 1.4), so the third recognition method works once the audio weights are converted (1.3 GB, CC-BY-NC).

## Decided (2026-11-15)

**2. Sharing the libraries: hard links, made by `get-services.sh` after building.** Measured on the development machine (btrfs): 18.8 GB of files of 1 MB or more in `bin/services`, 12.3 GB of them identical across heartmula, diffrhythm2 and lionclap (the seven biggest are `libtorch_cuda`, `libtorch_cuda_linalg`, `libcublasLt`, `libcudnn_engines_precompiled`, `libtorch_cpu`, `libcusparseLt` and `libcudnn_adv`, 5 GB for the three); with them linked the three take 6.5 GB. A file is linked only when it is byte-identical (`cmp`) to the first copy found, so a service built with another version of TorchSharp keeps its own. The question of whether a hard link survives `dotnet publish` re-runs has the answer that it does not need to: the script publishes into `<name>.new` and then replaces the folder (`rm -rf` removes one link of each file and leaves the others), so a link is never written through and a rebuilt service is linked again after the build. A shared folder with three entry points was not needed. `--share-only` links an installation that was made before; `--no-share` skips it. Test: `scripts/test/get_services_test.sh`.

**1. Where the binaries come from: built on the machine (decided 2026-11-15).** What `get-services.sh` does already, from the source vendored in `dotnet/`; no prebuilt packages to host, no clone step (the sibling repositories are not needed). The idea noted below (cloning the four repositories) is superseded by the vendored source.

**4. What the installer asks: four tiers (decided 2026-11-15, implemented).** `none`, `recognition` (LAION-CLAP + video-vision), `generator` (+ DiffRhythm2) and `all` (+ HeartMuLa); `install.sh` asks, Enter takes the recommended one (`all` with a card of 15 GB or more, `generator` with a smaller NVIDIA card, `recognition` without one), `--services TIER` skips the question and `--yes` takes the recommended one. The recommendation for a 12 GB card is a rule of thumb until task 2.1 measures it on the laptop. The Models screen card for the services lives in the dashboards (another repository).

## Idea noted (2026-10-22)

Since the four repositories (`heartmula`, `diffrhythm2`, `lion-clap`, `musicgen-core`) are in a git server, the installer can **clone them and build them on the machine**: `get-services.sh` would `git clone`/`git pull` into a work folder when the sibling folders are not there, then publish into `bin/services` as it does now. To be looked at together with the later installation script (`scripts/install.sh`), not before. It needs: the repository addresses (and whether they are public), the pinned revision of each (as `build-acestep.sh` pins its commit), the .NET SDK, and the time and disk of a first build.

## Options to decide

1. **Where the binaries come from.** (a) Build on the machine from the sibling repositories (what the script does now; needs the SDK, minutes and the repositories). (b) Download prebuilt packages published somewhere (needs a place to host them and a version per backend: cuda-linux, cpu). (c) Both: download, build as the fallback.
2. **Sharing the libraries.** Hard links or symlinks to a single copy of the native libraries (saves about 12 GB), or one shared service folder with the three entry points.
3. **Which backend.** CUDA for NVIDIA, CPU for the rest. What to tell a user without an NVIDIA card: CLAP on the CPU is usable (about a second per clip), the generators are not (about 17 times slower than real time for HeartMuLa).
4. **What the installer asks.** Tiers that fit the machine: nothing, only recognition (CLAP, 600 MB), one generator, all. The automatic planner already chooses models by memory; services could follow it.
5. **The laptop (one 12 GB card).** Measure HeartMuLa with `Service:Isolation=process` and a single card, and DiffRhythm2 and CLAP together with it; decide which pairs can be running at once and say so in Environment.
6. **MuQ.** Either port the audio half to the .NET service (and then add the method to the installer), or keep it out; its licence is non-commercial, so it must be an explicit choice.
7. **dotnet itself.** Detect it, say how to install it per distribution, or ship a self-contained build (larger, no runtime to install).

## Not decided here

How the Docker images are retired: they stay in the repository until the services have run on a second machine.
