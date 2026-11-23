# project-documentation Specification

## Purpose
Defines how the project's documentation is organized so a person who did not write it can install the project and find any topic: a short README that introduces and links, one document per topic, and an installation guide that covers every requirement.

## Requirements

### Requirement: The README introduces the project and links to the documents

`README.md` SHALL contain only what the project is, a quick start (the installer and the server), a table of the documents with one line each, and the license, and SHALL NOT exceed about 100 lines. It SHALL link to a document for each of: installation, the command line, the `scenes.yaml` reference, the management server, the prompt assistant, scene import, development, the tutorial, image mode, music mode, the HTTP API and the events.

#### Scenario: length
- **WHEN** the README is read
- **THEN** it has at most about 100 lines and no section holding a manual

#### Scenario: every topic is reachable
- **WHEN** the README's table of documents is followed
- **THEN** each of the topics above leads to its document

### Requirement: Each topic has its own document and the moved text is kept

The content that was in the README SHALL live in the document of its topic, unchanged except for headings and links: requirements and setup in `docs/INSTALL.md`; usage, flags, GPU use and the example in `docs/CLI.md`; the `scenes.yaml` reference and its sub-sections in `docs/SCENES_YAML.md`; the management server, its models, remote use and the web frontend in `docs/SERVER.md`; the prompt assistant in `docs/ASSISTANT.md`; dataset import and background music in `docs/IMPORT.md`; development in `docs/DEVELOPMENT.md`. References elsewhere (the tutorial, the HTTP API document, the image mode document, code comments, the documentation skill) SHALL point at the new documents.

#### Scenario: nothing lost
- **WHEN** the text of the old README sections is searched for in the new documents
- **THEN** each paragraph is found in the document of its topic

#### Scenario: no stale reference
- **WHEN** the repository is searched for links to removed README sections
- **THEN** none is found

### Requirement: The installation guide covers every requirement of every component

`docs/INSTALL.md` SHALL list, for each component, whether it is required or optional, the versions needed, the commands to install its dependencies on Arch, Debian/Ubuntu and Fedora, the NVIDIA driver and CUDA requirements, the size of each download and model, how to run the installer (`--check`, `--dry-run`, `--yes`, `--only`, `--no-models`), what to do on an unsupported distribution, a manual path with the same steps as the installer, and a troubleshooting section (the compiler/CUDA mismatch, a missing `nvcc`, memory limits, Ollama). The installer's report SHALL point at its sections.

#### Scenario: a clean machine
- **WHEN** a person follows the guide on a clean supported distribution
- **THEN** every tool and library needed to build and run the project is named with the command to install it

#### Scenario: the installer and the guide agree
- **WHEN** the installer's component list and the guide's are compared
- **THEN** every component of the installer has a section in the guide

### Requirement: The documentation's relative links resolve

`scripts/check-docs.sh` SHALL check every relative Markdown link and anchor in `README.md` and `docs/*.md` and exit non-zero, naming the file and the link, when a target file or heading does not exist.

#### Scenario: a broken link
- **WHEN** a document links to `docs/NOPE.md` or to a heading that does not exist
- **THEN** the checker exits non-zero and names the file and the link

#### Scenario: a clean tree
- **WHEN** every link resolves
- **THEN** the checker exits zero

### Requirement: The repository carries a skill for the agents that make a music video

`.claude/skills/music-video/SKILL.md` SHALL describe, for an agent driving this server, how to make an animated music video end to end with the HTTP API: the prerequisites and the features to check in the catalog, the steps in order with their routes and bodies (the piece with its song, the words heard, the reference, the proposal of design and script, the modes and the contrast to check, the scenes written and reviewed, the retry of the failed ones only, the approval, the piece review, the render, the contact sheet, the review as a stranger, the fixes), the points where it stops and shows the person what to decide, the craft rules in short with a link to `references/craft.md`, the evidence to leave (a `LEEME.txt` naming every file and id, nothing of the owner's deleted) and what never to do (delete pieces, run `npx` on the server, score without describing). `docs/MUSIC_MODE_MOTION.md` SHALL have a Craft section with the full rules and their origin, and `THIRD_PARTY.md` SHALL credit the HyperFrames references (Apache-2.0, commit) as the source the rules distil. `scripts/check-docs.sh` SHALL verify that every route the skill names exists in `openapi.yaml`.

#### Scenario: A route renamed
- **WHEN** a route the skill names is removed from `openapi.yaml`
- **THEN** `check-docs` fails naming the skill and the route

#### Scenario: The craft is one text
- **WHEN** the Craft section of the docs and `references/craft.md` are compared
- **THEN** they list the same rules
