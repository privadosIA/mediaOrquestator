# Spec Delta

## MODIFIED Requirements

### Requirement: The server runs the container as the script does

With the `docker` backend the server SHALL run `veritmo/heartmula:1` and `veritmo/diffrhythm2:1` with the name `vt-<engine>`, `--restart no`, the card chosen, the port of the engine published on `127.0.0.1` only and the folder of the weights mounted read-only, and SHALL report its state as the state of the .NET service (`stopped`, `starting`, `running`, `gpu`, `port`, `since`). A container that does not say ready in time SHALL be removed and the reason kept in `container.error`. `scripts/music-containers.sh` SHALL run the same images, names, ports and flags, and a test SHALL fail when the two differ. Nothing SHALL start by itself when the machine starts.

#### Scenario: A container that does not get ready
- **WHEN** a container is started and never answers ready
- **THEN** it is removed and `container.error` says why

#### Scenario: Script and server agree
- **WHEN** the image, the port, the name or a flag changes in the script or in the server but not in the other
- **THEN** the test that compares them fails
