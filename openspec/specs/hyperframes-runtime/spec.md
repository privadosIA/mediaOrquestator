# hyperframes-runtime Specification

## Purpose
TBD - created by archiving change add-hyperframes-runtime-and-components. Update Purpose after archive.

## Requirements

### Requirement: The upstream runtime is built once and kept locally

The server SHALL be able to draw a page with the HyperFrames runtime built from the pinned upstream commit. The build SHALL run once, by a script, from sources pinned by commit and dependency versions, SHALL record the hash of its output, and the runtime SHALL be served to the page from the server, never from the network.

#### Scenario: Built and recorded
- **WHEN** the script runs on a machine with Node
- **THEN** the runtime file exists under `third_party/` with its hash recorded, and running the script again does not rebuild it

### Requirement: Components are blocks

Each component of the registry SHALL be imported whole into the catalog as a block of `kind: component`, drawn through its demo composition, listed with its family, profile, tags, variables and sync points, and checked, thumbnailed, edited and rendered like any block.

#### Scenario: A component is in the catalog
- **WHEN** the catalog is listed with `kind=component`
- **THEN** the 238 components appear with their thumbnails and the state of their checks

### Requirement: A page with sub-compositions is drawn

A block that mounts sub-compositions (`data-composition-src`) SHALL be drawn with the sub-compositions mounted, their variables from `data-variable-values`, and everything seeked by the same time, with nothing fetched from outside.

#### Scenario: The agent cursor
- **WHEN** the `agent-cursor` component is drawn at 2.2 s
- **THEN** the cursor and its label are over the window and the window has reacted to the first click

### Requirement: A component can be a layer over a verse

A layer item of a music piece SHALL accept a component by name with its variables, as it does a block.

#### Scenario: Component over a verse
- **WHEN** a script has a layer item that names a component from 12 s to 18 s
- **THEN** the frames of that range show it over the verse
