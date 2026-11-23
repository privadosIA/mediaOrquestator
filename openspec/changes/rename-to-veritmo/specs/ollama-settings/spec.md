# Spec Delta

## MODIFIED Requirements

### Requirement: The setting is stored privately and validated

The saved address SHALL live in `settings.json` next to the server's token (`~/.config/veritmo/`, honouring `XDG_CONFIG_HOME`), in a directory of mode 0700 and a file of mode 0600, written atomically. An address SHALL be accepted only when its scheme is `http` or `https` (or absent, meaning `http`), it has a host, no user or password, and no path (other than `/`), query or fragment, and it is at most 200 characters; anything else SHALL be rejected with a message that says which rule failed, and nothing SHALL be saved. A saved value SHALL be stored normalized (with scheme). Removing the setting SHALL delete the key so the resolver falls back to `OLLAMA_HOST`.

#### Scenario: a valid address is saved
- **WHEN** the address `192.168.1.50:11434` is saved
- **THEN** `settings.json` holds `http://192.168.1.50:11434`, its mode is 0600, and the resolver uses it

#### Scenario: credentials are refused
- **WHEN** the address is `http://user:pass@host:11434`
- **THEN** it is rejected, nothing is saved, and the message says credentials are not allowed

#### Scenario: a path is refused
- **WHEN** the address is `http://host:11434/api`
- **THEN** it is rejected with a message that says the address must have no path

#### Scenario: another scheme is refused
- **WHEN** the address is `ftp://host`
- **THEN** it is rejected with a message that says only http and https are accepted

#### Scenario: removing the setting
- **WHEN** the saved setting is removed while `OLLAMA_HOST` is set
- **THEN** the resolver returns the environment's address with source `env`
