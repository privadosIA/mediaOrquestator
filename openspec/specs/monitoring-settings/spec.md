# monitoring-settings Specification

## Purpose
TBD - created by archiving change add-monitoring-settings. Update Purpose after archive.

## Requirements

### Requirement: The addresses of the monitoring tools are kept in Settings

The server SHALL keep, for each of `grafana`, `prometheus`, `loki` and `redis`, an address, a user and a key. `GET /api/settings/monitoring` SHALL list the four, in that order, with the address and the user and `secret_set`, and SHALL never return a key. `PUT /api/settings/monitoring/{tool}` SHALL validate and save: the address is `host[:port]` or an http(s) URL (Redis: `host[:port]` only) with no user, query or fragment, the usual port of the tool is filled in (3000, 9090, 3100, 6379), and anything else is `400`; an empty address removes the tool and its key; a key left out keeps the saved one only when the address and the user are the same. Another `tool` is `404`. The key SHALL be kept in the settings file with mode 0600.

#### Scenario: The port

- **WHEN** `192.168.1.20` is saved for Grafana
- **THEN** the address kept is `http://192.168.1.20:3000`

#### Scenario: The key

- **WHEN** the settings are read, or the address is saved again with no key
- **THEN** no answer has the key, and the saved one is kept; with another address it is not

### Requirement: Each tool can be tested

`POST /api/settings/monitoring/{tool}/test` SHALL ask the tool within 6 seconds, with the form's values or, with no body, what is saved, and answer `{ok, version?, authenticated?, error?}`: Grafana `/api/health` (version) and, with a key, `/api/org`; Prometheus `/-/healthy` and its build info; Loki `/ready` and its build info; Redis `AUTH` (with a key), `PING` and the version from `INFO`. With a user the HTTP calls SHALL use Basic authentication and with none a Bearer token. A tool that does not answer, or does not accept the key, SHALL be `ok: false` with the reason, and `authenticated` false when it refused the key. It SHALL write and save nothing. `400` when there is no address to test.

#### Scenario: A token

- **WHEN** Grafana is tested with a key and no user
- **THEN** `/api/org` is asked with `Authorization: Bearer <key>` and `authenticated` is true when it answers 200

#### Scenario: A tool that is off

- **WHEN** nothing listens on the address
- **THEN** the answer is `200` with `ok: false` and the error

### Requirement: The settings card

Settings SHALL have a **Monitoring** card with a row per tool (address, user, key, a test button, a save button and, for Grafana, Prometheus and Loki with a saved address, a link to its page), showing the version and whether the key was accepted, or why it failed, and never the key. The server SHALL NOT depend on any of the tools being up.

#### Scenario: Saved

- **WHEN** Grafana is saved
- **THEN** its row links to it and the key field shows only that one is kept
