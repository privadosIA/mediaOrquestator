# Spec Delta

## MODIFIED Requirements

### Requirement: The command fills the store from the enabled sources
`veritmo prompts fetch` SHALL process every source of the sources file that is enabled (or only `--source id`; an unknown id is an error naming the ids), and write the resulting presets as bundled, read-only presets (`own: false`) into `<MEDIA_DIR>/prompts/<kind>/` (the media directory from `--media-dir`, `MEDIA_DIR`, or the path in `~/.config/veritmo/media-dir`; `prompts/` beside the projects directory when there is none). With `--out file` it SHALL write the `{format: "prompt-presets", version: 1, meta, items}` bundle instead and touch no store. With `--dry-run` it SHALL download and report but write nothing. Running it twice SHALL add nothing the second time (a preset with the same `kind`, `source` and `source_id` is already there).

#### Scenario: First and second run
- **WHEN** the command runs twice with the same sources and seed
- **THEN** the first run writes the presets and the second reports them as already there and writes none

#### Scenario: A bundle file instead of the store
- **WHEN** it runs with `--out bundle.json`
- **THEN** the file holds the presets in bundle format and the store folder is unchanged
