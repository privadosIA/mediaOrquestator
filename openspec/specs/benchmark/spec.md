# benchmark Specification

## Purpose
Compare machines on equal terms by running the same real jobs and recording how long they take and how much memory they need, including what a machine cannot do.

## Requirements

### Requirement: The same real jobs are run and measured on every machine

The benchmark SHALL run a fixed set of real projects through the orchestrator (an SDXL image set, Krea 2 and IntoRealism renders, a Wan 5B clip, a MiniMax H3 clip, an LTX 2.3 clip, a 30 s ACE-Step song and a song from each of the two generators that run as services, HeartMuLa and DiffRhythm2) with fixed seeds and no cache, and SHALL write one result file per machine and day with the seconds per phase, the peak VRAM over what was already in use and the lowest free RAM of each test, together with the machine description (CPU, RAM, GPUs) and the `sd` version.

#### Scenario: a test whose models are missing
- **WHEN** the models of a test are not installed
- **THEN** the test is skipped and says which files are missing, and the others run

#### Scenario: a test that fails
- **WHEN** a test fails, for example out of memory on a modest card
- **THEN** it is recorded as failed with the reason, which is a result too

#### Scenario: comparing two machines
- **WHEN** `--compare` is given two result files
- **THEN** it prints the tests side by side with the ratio of their times where both succeeded

### Requirement: Song generators that run as services are measured with their model load

For the generators that run as services, the benchmark SHALL stop the service before the test, start it through the running server inside the measured window and stop it afterwards, so the memory its model takes is part of the test's peak VRAM and the seconds it took to be ready are recorded as their own phase. A service that does not become ready SHALL make the test fail with that reason, and without the server running the test SHALL be skipped saying so.

#### Scenario: a service test
- **WHEN** the HeartMuLa test runs with the server up
- **THEN** the service is started and stopped by the test, the result has a `service_start` phase and the peak VRAM includes the loaded model

#### Scenario: no server
- **WHEN** the server is not running
- **THEN** the service tests are skipped naming the server, and the other tests run

#### Scenario: a service that does not start
- **WHEN** a service stops before it is ready
- **THEN** the test is recorded as failed with "the <engine> service did not start"

### Requirement: The generated files can be kept

The benchmark SHALL delete what its tests generate unless asked to keep it, and with `--keep` SHALL move the pictures, clips and songs of each test to a folder named after the machine and the day, one subfolder per test, inside a directory that version control ignores.

#### Scenario: default run
- **WHEN** the benchmark runs without `--keep`
- **THEN** no `out/bench-*` file is left behind and the projects of the person are untouched

#### Scenario: keeping the results
- **WHEN** the benchmark runs with `--keep`
- **THEN** the generated files of each test are under `out/bench-results/<host>-<date>/<test>/`

### Requirement: A successful run does not vouch for what was drawn

The documentation SHALL state that a test marked `ok` only means the run ended, and SHALL give a way to check that a clip is not blank, because a clip can be unusable while the run reports success.

#### Scenario: a blank clip
- **WHEN** a clip has a mean luma of 255 in its frames, or weighs a few KB for two seconds of video
- **THEN** the documentation tells the reader to treat the test as failed whatever the result file says

### Requirement: The benchmark records how much of each test is model load
For every test the result file SHALL include the seconds `sd` spent loading models, taken from the `load_ms` of the phase events, next to the seconds per phase. A test with no readable load time SHALL record it as absent, not as zero. `--compare` SHALL show the load seconds of both machines where both have them.

#### Scenario: a test with load times
- **WHEN** a render test finishes and its phase events carry `load_ms`
- **THEN** the result has `load_seconds` for that test, and it is no larger than the seconds of the test

#### Scenario: an older result file
- **WHEN** `--compare` reads a result file written before this field existed
- **THEN** it compares the tests as before and shows no load column for that machine

### Requirement: Results name a label, not the machine

A result file SHALL record a label given with `--label` (default `machine`) in place of the host name, and its file name SHALL use the label. The operating system and driver versions stay.

#### Scenario: default label
- **WHEN** the benchmark runs without `--label`
- **THEN** the result's `host` field is `machine` and the file is `benchmarks/machine-<date>.json`
