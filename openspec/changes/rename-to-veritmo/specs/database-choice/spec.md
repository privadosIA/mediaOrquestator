# Spec Delta

## MODIFIED Requirements

### Requirement: The database can be given by the environment

The server and the commands that open the database SHALL take the engine, host, port, database name, user and password from the environment (`VERITMO_DB_DRIVER`, `VERITMO_DB_HOST`, `VERITMO_DB_PORT`, `VERITMO_DB_NAME`, `VERITMO_DB_USER`, `VERITMO_DB_PASSWORD`; the former `MO_*` names are read as a fallback with a warning, as `project-identity` says) when nothing is saved in the settings file, so that a container needs no hand-written file. What is saved in the settings file SHALL win over the environment. The password SHALL be read only from the environment variable or the file and SHALL NOT appear in any answer of the API, in a log line or in the status of the data layer. `GET /api/settings/database` SHALL say whether what is active comes from the settings file or from the environment.

#### Scenario: A container with no settings file

- **WHEN** the server starts with the variables of a MySQL in its environment and no saved database
- **THEN** it keeps all its state in that MySQL and `GET /api/settings/database` says the source is the environment

#### Scenario: Saved settings win

- **WHEN** a database is saved in the settings file and the environment names another
- **THEN** the server uses the saved one

#### Scenario: The password is never shown

- **WHEN** the settings and the status of the data layer are read
- **THEN** the password of the environment is in none of them
