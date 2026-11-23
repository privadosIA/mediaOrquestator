# Spec Delta

## MODIFIED Requirements

### Requirement: Keys live in their own private file and are never shown

A provider's API key SHALL be stored only in `keys.json`, next to the settings (`~/.config/veritmo/`), in a file of mode 0600 inside a directory of mode 0700, written atomically, keyed by provider id. A key SHALL be accepted only up to 500 characters with no spaces or control characters. A key SHALL NEVER appear in `settings.json`, `scenes.yaml`, exported packages, the environment report, log output, error messages, or any API answer: the API SHALL only say whether one is set (`key_set`). Deleting a provider SHALL delete its key, and removing a key SHALL leave the provider. A key SHALL NOT be sent over plain `http` to an address that is not this machine: creating or testing such a provider with a key SHALL be refused with a message saying to use https.

#### Scenario: where the key goes
- **WHEN** a provider is saved with a key
- **THEN** `keys.json` holds it with mode 0600 and `settings.json` does not contain it

#### Scenario: never in an answer
- **WHEN** providers are listed, created, edited or tested
- **THEN** no response body contains the key and each provider has only `key_set`

#### Scenario: never in an error
- **WHEN** a request to a provider fails and its body echoes the key
- **THEN** the message returned has the key replaced by `***`

#### Scenario: deleting a provider deletes its key
- **WHEN** a provider with a key is deleted
- **THEN** its entry is gone from `keys.json`

#### Scenario: no key over plain http
- **WHEN** a provider at `http://192.168.1.60:8000/v1` is saved or tested with a key
- **THEN** the request is refused with a message that says to use https, and the key is not sent

#### Scenario: a loopback server may use http
- **WHEN** a provider at `http://127.0.0.1:8000/v1` is saved with a key
- **THEN** it is accepted
