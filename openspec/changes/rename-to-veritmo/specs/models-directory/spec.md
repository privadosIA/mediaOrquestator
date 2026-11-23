# Spec Delta

## MODIFIED Requirements

### Requirement: The models directory is set by a flag, a variable or a user file

The server SHALL take the models directory from `--models-dir`, else from `MODELS_DIR`, else from the first line of `~/.config/veritmo/models-dir` (in `$XDG_CONFIG_HOME` when it is set). With none of them nothing SHALL change: `models/` stays where it is. `MODELS_DIR=off` SHALL ignore the user file.

#### Scenario: Flag wins
- **WHEN** the flag, the variable and the file name different directories
- **THEN** the flag is used

#### Scenario: Nothing set
- **WHEN** no models directory is given
- **THEN** the server runs as before and reports no `models` in the storage answer
