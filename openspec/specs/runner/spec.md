# runner Specification

## Purpose
Defines the single seam every native subprocess invocation (Piper, sd, ffmpeg, ffprobe, nvidia-smi) goes through, and the guarantees callers can rely on regardless of which binary is invoked.

## Requirements

### Requirement: Run propagates captured stdout on success
On a zero exit, `Run` SHALL return the subprocess's captured stdout and a nil error.

#### Scenario: success
- **WHEN** the subprocess exits 0
- **THEN** `Run` returns its captured stdout bytes and a nil error

### Requirement: Run always surfaces captured stderr on failure
On a non-zero exit, `Run` SHALL return a nil stdout and a non-nil error whose text includes the subprocess's captured stderr; stderr SHALL never be silently discarded.

#### Scenario: non-zero exit includes stderr
- **WHEN** the subprocess exits non-zero and writes text to stderr
- **THEN** `Run` returns a non-nil error whose message includes that stderr text

### Requirement: Run honors context cancellation
`Run` SHALL stop waiting on the subprocess once `ctx` is cancelled, and SHALL return an error wrapping `ctx.Err()`.

#### Scenario: context cancelled before exit
- **WHEN** `ctx` is cancelled before the subprocess would otherwise finish
- **THEN** `Run` returns an error wrapping `ctx.Err()`

### Requirement: Run supports stdin piping and per-call environment overrides
`Run` SHALL deliver data supplied via `WithStdin` to the subprocess's stdin, and SHALL apply `WithEnv` overrides on top of the inherited environment, with later overrides taking precedence.

#### Scenario: stdin delivered
- **WHEN** `WithStdin(data)` is passed to `Run`
- **THEN** the subprocess receives `data` on its stdin

#### Scenario: env override applied
- **WHEN** `WithEnv("KEY=VALUE")` is passed to `Run`
- **THEN** the subprocess's environment includes `KEY=VALUE`, taking precedence over any inherited `KEY`

### Requirement: Run kills the whole process group and bounds what it keeps

When its context is cancelled, `Run` SHALL terminate the child and every process it spawned (the process group), and SHALL return within a fixed delay even if a grandchild keeps the output pipes open. `Run` SHALL keep the end of stderr only (at least the last 1 MiB), SHALL stop a program whose stdout exceeds a fixed limit (at least 256 MiB) and fail the call with a clear error instead of returning truncated data, and SHALL truncate the arguments it includes in an error message.

#### Scenario: a grandchild outlives the child
- **WHEN** the child starts a grandchild that inherits stdout and the context is cancelled
- **THEN** both are gone and `Run` returns within the delay

#### Scenario: a chatty program
- **WHEN** a program writes 100 MiB to stderr
- **THEN** the process's memory does not grow by 100 MiB and the error carries the tail

#### Scenario: a program that writes without end to stdout
- **WHEN** a program writes more than the stdout limit
- **THEN** it is stopped and the call fails saying the output was too large

#### Scenario: a long prompt in the arguments
- **WHEN** a program fails and one argument is a 20 KB prompt
- **THEN** the error message carries at most a fixed prefix of each argument
