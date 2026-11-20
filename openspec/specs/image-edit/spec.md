# image-edit Specification

## Purpose
Edit a picture by instruction with a reference-image model such as FLUX.1 Kontext, so that "put a red cap on him" changes the cap and keeps the rest.

## Requirements

### Requirement: A model set can be an editing model

A project's `defaults.model_set` SHALL accept `edit: true` to say that the model edits a reference picture by instruction. An editing model set SHALL be one of two families, told apart by the files it names: the **FLUX family** (`diffusion_model`, `vae`, `clip_l` and `t5xxl`, without `llm`) and the **Qwen family** (`diffusion_model`, `vae`, `llm` and `llm_vision`, without `clip_l` and `t5xxl`). A set that mixes the files of both families, or lacks one of its family's, SHALL be refused saying which is missing. A model set that is not an editing one SHALL refuse `clip_l`, `t5xxl` and `llm_vision`. Every file SHALL exist, as for any model set.

#### Scenario: a valid editing set

- **WHEN** a project has `model_set` with `edit: true`, `diffusion_model`, `vae`, `clip_l` and `t5xxl` that exist
- **THEN** the configuration is valid without an `llm`

#### Scenario: a valid Qwen editing set

- **WHEN** a project has `model_set` with `edit: true`, `diffusion_model`, `vae`, `llm` and `llm_vision` that exist
- **THEN** the configuration is valid without `clip_l` and `t5xxl`

#### Scenario: a missing piece

- **WHEN** a Qwen editing set has no `llm_vision`
- **THEN** the configuration is refused saying `llm_vision` is required

#### Scenario: families mixed

- **WHEN** an editing set names `t5xxl` and `llm_vision`
- **THEN** the configuration is refused

#### Scenario: pieces of an editing model in an ordinary set

- **WHEN** a model set without `edit` names a `clip_l` or an `llm_vision`
- **THEN** the configuration is refused

### Requirement: A scene of an editing project edits its picture

In a project whose model set is an editing one, a scene with an `init_image` SHALL be rendered by giving that picture to the model as the reference, fitted to the scene's resolution as for any starting picture, and the scene's prompt SHALL be the instruction. The scene's strength SHALL have no effect. The result SHALL keep the reference's content except for what the instruction changes. A scene of such a project without an `init_image` SHALL produce a warning saying an editing model needs the picture to edit.

#### Scenario: an instruction

- **WHEN** a scene has `init_image: foto.png` and the prompt "put a red cap on the man"
- **THEN** the picture is made from `foto.png` as the reference and shows the same scene with the cap

#### Scenario: no picture

- **WHEN** a scene of an editing project has no `init_image`
- **THEN** the project validates with a warning

#### Scenario: the cache

- **WHEN** the same scene is rendered again with the same picture and instruction
- **THEN** the result is reused, and changing the instruction or the picture renders it again

### Requirement: An edit keeps the shape of its picture

A scene of an editing project SHALL be edited at the shape of its picture, in about the pixels of the resolution the project (or the scene) asks for, in multiples of 16: a portrait photo SHALL NOT be cropped to a landscape resolution, nor a landscape one to a portrait. A picture whose size cannot be read SHALL be edited at the resolution asked for, as before.

#### Scenario: a portrait photo in a landscape project

- **WHEN** a 960x1367 photo is edited in a project whose resolution is 1024x576
- **THEN** the result is 640x912 and shows the whole picture

#### Scenario: a picture that cannot be read

- **WHEN** the size of the picture cannot be read
- **THEN** the edit is made at the project's resolution

### Requirement: The editing model is a catalog entry

The model catalog SHALL offer FLUX.1 Kontext dev as an entry made of the model, the CLIP-L and T5-XXL text encoders and the VAE, with the measured time and memory, installable from Models like any other. The entry SHALL be flagged as an editing model and SHALL NOT be chosen as the generation model for a request that has no picture to edit, nor offered as a look (photo, anime, general).

#### Scenario: installing

- **WHEN** the user installs the entry from Models
- **THEN** the four files are downloaded and the entry shows as installed

#### Scenario: no picture to edit

- **WHEN** a request has no picture and the editing model is installed
- **THEN** the usual image model is chosen, never the editing one

### Requirement: Retouch is a look of a project

The style choices of a project (in the editor's Style card and in the new-project dialog) SHALL include **Retouch (edit pictures)** next to photorealism, illustration and anime, and general. It SHALL be available only when an editing model is installed, SHALL say what it is in one line, and the style card SHALL highlight the look the project has now. Picking Retouch in the editor SHALL switch the project's image model to the editing model and picking any other style SHALL give the project back the image model and settings it had; picking it in the new-project dialog SHALL create the project with the editing model. A project SHALL never become an editing one by only choosing a starting picture.

The scene editor's "starting picture" section SHALL offer the same choice for the project ("Start from this picture" or "Edit it with an instruction"), kept in step with the style card, with a note that it changes the model of the whole project.

#### Scenario: Retouch in the style card

- **WHEN** the user picks "Retouch" in the editor's Style card
- **THEN** the project's model becomes the editing model, the button is highlighted and a note says each scene needs its starting picture

#### Scenario: going back by picking a style

- **WHEN** the user then picks "Illustration and anime"
- **THEN** the project has an anime model again, with its settings

#### Scenario: a new project

- **WHEN** the user picks "Retouch (edit pictures)" in the new-project dialog
- **THEN** the project is created with the editing model

#### Scenario: choosing a starting picture only

- **WHEN** a scene gets a starting picture
- **THEN** the project keeps its image model: it starts a generation from the picture until the user picks Retouch

#### Scenario: no editing model

- **WHEN** no editing model is installed
- **THEN** Retouch is disabled and a note says where to install it

### Requirement: Qwen-Image-Edit is a second editing model

The model catalog SHALL offer Qwen-Image-Edit-2509 as an entry made of the model, the Qwen2.5-VL text model with its vision projector and the Qwen-Image VAE, with the measured time and memory and its licence, installable from Models like any other and flagged as an editing model with the same rules as FLUX.1 Kontext (never chosen to generate from nothing, never offered as a look). A scene of a project that uses it SHALL be edited as for any editing set: the picture as the reference, the prompt as the instruction, no strength, at the shape of the picture. The engine SHALL run with the files read from disk, the weights offloaded to the CPU and the VAE in tiles, so that it fits the memory limit the server already runs under, and it SHALL NOT be offered on a machine with less RAM than its entry declares.

#### Scenario: installing

- **WHEN** the user installs the entry from Models
- **THEN** the four files are downloaded and the entry shows as installed

#### Scenario: an instruction

- **WHEN** a scene of a Qwen editing project has `init_image: foto.png` and the prompt "make the sky sunset orange"
- **THEN** the picture shows the same scene and people with the sunset sky

#### Scenario: not enough RAM

- **WHEN** the machine has less RAM than the entry declares
- **THEN** the entry is shown as not available for this machine and Retouch does not offer Precise

#### Scenario: the cache

- **WHEN** the same scene is rendered again with the same picture and instruction
- **THEN** the result is reused; changing the engine renders it again

### Requirement: Retouch has an engine

When more than one editing model is installed, Retouch (in the style card, the new-project dialog and the editor's starting-picture section) SHALL ask which engine to use and say in one line what sets them apart: **Fast** (FLUX.1 Kontext: the quickest, with a non-commercial licence) and **Precise** (Qwen-Image-Edit: slower, with a licence that allows commercial use). Fast SHALL be the default. When only one is installed Retouch SHALL use it without asking, and SHALL say which engine it is. Changing the engine of a project SHALL switch its image model to the other editing model and leave its scenes and their instructions unchanged.

#### Scenario: both installed

- **WHEN** both editing models are installed and the user picks Retouch
- **THEN** the engine choice is shown, Fast is selected and each option says in a line what it is

#### Scenario: one installed

- **WHEN** only the Qwen model is installed and the user picks Retouch
- **THEN** the project uses it, the card says "Precise (Qwen-Image-Edit)" and no choice is asked

#### Scenario: changing the engine

- **WHEN** the user changes the engine of a Retouch project from Fast to Precise
- **THEN** the project's model is the Qwen editing set and its scenes are untouched
