# script-block-layers Specification

## Purpose
TBD - created by archiving change add-block-layers-to-script-writer. Update Purpose after archive.

## Requirements

### Requirement: The writer knows the usable blocks

The script writer SHALL be given a compact list of the blocks and components that can be drawn now (state `ok`, libraries installed, the runtime installed for those that need it), and SHALL be given none when nothing is usable or when the piece's `blocks` switch is `off`.

#### Scenario: Nothing installed
- **WHEN** no block is in the catalog or the libraries are missing
- **THEN** the writer's prompt has no list and a proposal cannot name a block

### Requirement: The writer may add block layers

A proposal MAY carry layer items that name a block or a component with variables and a range of the song. An item that does not pass the validation of a script (unknown or unusable block, bad range, overlap, too many parts in a page) SHALL be dropped and named in the proposal's warnings; the rest of the script SHALL be kept.

#### Scenario: A usable block
- **WHEN** the model names a block that is `ok` over a section
- **THEN** the proposed script has a layer item with that block over that range

#### Scenario: An unusable block
- **WHEN** the model names a block that failed its checks
- **THEN** the item is dropped, the warning names the block and the script is otherwise as proposed

### Requirement: The reviewer does not score a block as an error

The reviewer SHALL be told that a layer block may be on a frame, so that its look (full-frame shaders, UI components) is not scored as a defect of the piece.

#### Scenario: A frame with a component
- **WHEN** a piece with a block layer is reviewed
- **THEN** the reviewer's instructions mention block layers
