# generation-metadata Specification

## Purpose
Reads the generation settings embedded in a picture or clip (prompt, negative prompt, seed, steps, cfg, model) and normalizes them, so an imported file can offer to reuse them. Untrusted input: every read is bounded and a file with nothing readable yields no metadata, never an error.

## Requirements

### Requirement: Generation settings are read from the formats that carry them

The server SHALL read, from a picture or clip, the generation settings it embeds, from: PNG text chunks (`tEXt`, `iTXt` and compressed `zTXt`) holding the A1111/stable-diffusion.cpp `parameters` text or ComfyUI's `prompt` graph; the `comment` and `description` tags of video containers (read with ffprobe); the comment of a GIF; and the EXIF and XMP of a WebP. The result SHALL be one set of optional fields: prompt, negative prompt, seed, steps, cfg scale, sampler, model, width, height and strength. A file with no readable settings, an unknown format or malformed metadata SHALL yield no metadata and no error.

#### Scenario: a PNG made by stable-diffusion.cpp
- **WHEN** a PNG whose `parameters` text is "a red apple\nNegative prompt: blurry\nSteps: 25, CFG scale: 7.0, Seed: 42, Size: 512x512, Model: sd15.safetensors" is read
- **THEN** the prompt is "a red apple", the negative prompt "blurry", steps 25, cfg 7, seed 42, size 512x512 and the model `sd15.safetensors`

#### Scenario: a clip made by stable-diffusion.cpp
- **WHEN** an MP4 whose `comment` tag holds that same text (with its `SDCPP:` JSON tail) is read
- **THEN** the same fields are found, with the exact negative prompt and seed of the JSON tail

#### Scenario: a ComfyUI PNG
- **WHEN** a PNG carries ComfyUI's `prompt` graph with a sampler whose positive and negative inputs lead to text-encode nodes
- **THEN** the prompt and negative prompt are those nodes' texts and the seed, steps and cfg come from the sampler

#### Scenario: nothing to read
- **WHEN** a PNG, GIF or video has no generation metadata, or its metadata is not in a known shape
- **THEN** no metadata is returned and there is no error

### Requirement: Reading is bounded and the values are plain data

The reader SHALL inspect at most the first 8 MiB of a file, keep each text value to at most 64 KiB, decompress at most 1 MiB per compressed chunk, accept a ComfyUI graph of at most 2000 nodes, clip the prompt and negative prompt to 20 000 characters, and accept only numbers in range (a seed that fits a signed 64-bit integer, steps from 1 to 1000, cfg from 0 to 100, sides from 1 to 16384). A value outside its range SHALL be left out, not clamped. Values SHALL be returned as data and never interpreted.

#### Scenario: oversized or truncated input
- **WHEN** a chunk claims more than the limits, or the file ends in the middle of a chunk
- **THEN** reading stops without error and returns what was valid before it, possibly nothing

#### Scenario: a number out of range
- **WHEN** the text says `Steps: 5000`
- **THEN** steps are left out and the other fields are kept
