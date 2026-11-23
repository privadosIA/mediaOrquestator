# Spec Delta

## MODIFIED Requirements

### Requirement: Runs are idempotent, logged and confined

Every check SHALL look at the real thing (the binary runs, the file exists), so re-running after an interruption continues from what is still missing and installs nothing twice. Every run SHALL append the commands it ran and their output to `~/.cache/veritmo/install.log`. The installer SHALL NOT write outside the project directory, that log directory and the package manager's own locations, and SHALL NOT remove anything outside the repository.

#### Scenario: re-run
- **WHEN** the installer is run again after a complete install
- **THEN** it reports everything `ok` and runs no installation step

#### Scenario: interrupted build
- **WHEN** a previous run stopped before `bin/sd` was built
- **THEN** the next run reports it `missing` and offers it again

#### Scenario: log
- **WHEN** a step runs
- **THEN** its command and output are appended to the log file
