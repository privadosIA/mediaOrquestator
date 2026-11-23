# Spec Delta

## MODIFIED Requirements

### Requirement: The command line uses the database of the server

A command of `veritmo` that reads or writes state kept in the database SHALL open the same database the server uses: the one saved in Settings, or the SQLite file given with `--db` (by default the one `serve` uses). A command SHALL be able to name another MySQL or Postgres database with `--driver`, `--host`, `--port`, `--database` and `--user`, and SHALL read its password from the environment variable `VERITMO_DB_PASSWORD` (or `MO_DB_PASSWORD` as the obsolete fallback) and never from a flag (a command line is visible to the other users of the machine); those flags without `--driver`, `--driver sqlite` and an engine that does not exist SHALL be a usage error. It SHALL work while the server is running on the same SQLite file. A command that cannot open the database SHALL exit with a non-zero code and an error that names the database, before doing any work.

#### Scenario: While the server runs

- **WHEN** a command that reads state runs while the server is up on the same SQLite file
- **THEN** it reads the same state the server answers and neither fails

#### Scenario: No database

- **WHEN** the database cannot be opened
- **THEN** the command exits with a non-zero code and names the database
