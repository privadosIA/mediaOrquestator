# model-availability Specification

## Purpose
Say, per task that needs a model (prompt assistant, describing a picture or a clip, quality check), whether the provider routed to it can run it now, so the dashboard shows only the AI controls that can work and, when the provider answers but lacks the model, says what to install.

## Requirements

### Requirement: The server says which model tasks can run now

`GET /api/models/availability` SHALL answer `{tasks: {assistant, describe_image, describe_video, quality, motion_graphics}}`, each `{available, provider: {id, name, type}, reason?}`, after resolving the provider routed to each task and checking it can be used now. For an Ollama (the built-in or an extra one) it SHALL ask for its models within 3 seconds and say `unreachable` when it does not answer, `no_models` when the assistant or the motion-graphics task has no chat model (embedding and reranker models do not count) and `no_vision_model` when a describe or quality task has no model that reads pictures. For an OpenAI-compatible provider it SHALL say `no_model` when the model the task needs is not declared, `no_consent` when the provider is remote and the user has not accepted sending data to its current address (checked without contacting it), `key_rejected` when its model list answers 401 or 403, and `unreachable` when it does not answer within 3 seconds. A task that can run SHALL be `{available: true, provider}`. The check SHALL send nothing of the user's (no picture, clip or text), and an Ollama check SHALL NOT load any model.

#### Scenario: a working Ollama
- **WHEN** Ollama answers with a chat model and a model that reads pictures
- **THEN** every task routed to it is available

#### Scenario: Ollama is stopped
- **WHEN** nothing answers at the Ollama address
- **THEN** every task routed to it is `{available: false, reason: "unreachable"}` and names the provider

#### Scenario: no model that reads pictures
- **WHEN** Ollama has only text models
- **THEN** `assistant` is available and the describe and quality tasks are `no_vision_model`

#### Scenario: only embedding models
- **WHEN** Ollama has only embedding and reranker models
- **THEN** `assistant` is `no_models`

#### Scenario: a cloud provider without its key accepted
- **WHEN** a task is routed to an OpenAI-compatible provider whose model list answers 401
- **THEN** the task is `key_rejected`

#### Scenario: a remote provider without consent
- **WHEN** the provider is remote and was never accepted, or its address changed since
- **THEN** the task is `no_consent` and the provider was not contacted

#### Scenario: a model not declared
- **WHEN** a task is routed to an OpenAI-compatible provider that has no model for it
- **THEN** the task is `no_model`

### Requirement: The answer is cached briefly and refreshed when anything changes

The server SHALL keep the answer for 30 seconds, so the dashboard asking often does not contact Ollama or a provider each time, and SHALL drop it whenever the Ollama address, the providers, a provider's key, the routing or an acknowledgement change.

#### Scenario: asked twice in a row
- **WHEN** the endpoint is called twice within 30 seconds with no change
- **THEN** the models of Ollama are asked for once

#### Scenario: a change drops the cache
- **WHEN** the Ollama address, a provider or the routing is changed between two calls
- **THEN** the second call checks again

### Requirement: Controls that depend on a model are hidden when it is not available

The dashboard SHALL hide, not disable, the controls of a task whose availability is not `true`, and SHALL say so with a notice when the cause is a missing model (see the next requirement). The controls are: Improve with AI and Suggest motion (assistant), Write with AI for songs (assistant), the videoclip plan's "write the scenes with the AI" (assistant), "Let the assistant propose it" in the clip review (assistant), Describe a reference (describe), and the controls that ask the model to write a motion graphic (motion_graphics): the "Animated graphic" option of the scene's picture selector (unless the scene already is one), adding an overlay to a scene (unless it already has one), the "Motion-graphics project" button of the projects list, and the generate-page button of the page editor. While the availability is not known (no answer yet, or the server does not answer) those controls SHALL stay hidden. The dashboard SHALL read the availability when it starts, after any change in Settings (the Ollama address, providers, tasks) and when the window regains focus. A videoclip plan whose AI is not available SHALL be made from the song's style and lyrics without an error. The preview and the manual editing of a page already stored, and the render of scenes that already have their page, SHALL NOT depend on the model and SHALL stay. The quality check's Analyze and its model selector SHALL NOT be hidden. Settings SHALL keep showing the state and the connection test.

#### Scenario: no Ollama
- **WHEN** Ollama is unreachable and nothing else is routed
- **THEN** none of the controls above is on screen

#### Scenario: Ollama is back
- **WHEN** Ollama starts and the window regains focus
- **THEN** the controls appear without reloading the page

#### Scenario: only the vision tasks are missing
- **WHEN** Ollama has text models but none that reads pictures
- **THEN** the assistant's controls are shown and Describe a reference is not

#### Scenario: unknown means hidden
- **WHEN** the availability has not arrived yet or the request failed
- **THEN** the controls are hidden

#### Scenario: a videoclip without AI
- **WHEN** the assistant is not available and a videoclip plan is made
- **THEN** the checkbox is not shown and the plan is made from the style and the lyrics with no error

#### Scenario: the quality check stays
- **WHEN** no vision model is available
- **THEN** Analyze is still shown and measures colour and sharpness

#### Scenario: a routed provider
- **WHEN** a task is routed to a reachable cloud provider with its key accepted and Ollama is down
- **THEN** that task's controls are shown

#### Scenario: no model for motion graphics
- **WHEN** the `motion_graphics` task is not available
- **THEN** the "Animated graphic" option, the add-overlay control, the "Motion-graphics project" button and the generate-page button are not on screen

#### Scenario: a scene that already is a motion graphic
- **WHEN** the task is not available and the scene already has a motion graphic or an overlay
- **THEN** its description, preview and manual editing of the stored page stay, and only the generate-page button is hidden

#### Scenario: a run without the model
- **WHEN** a project has motion scenes with stored pages and the model is not available
- **THEN** the run renders them from their pages and does not ask for the model

### Requirement: A missing model is explained with a notice, a missing connection is not

When the provider of a task answers but has not the model the task needs (`no_models`, `no_vision_model` or `no_model`), the dashboard SHALL show, in place of the hidden controls, a short notice that says what is missing and how to fix it: for an Ollama the kind of model and a command to install one (`ollama pull qwen3:8b` for text, `ollama pull gemma4:26b` for pictures), for another provider that its text or vision model must be set in Settings, with a link to Settings. The notice SHALL appear in the scene editor (once, under the prompt, not once per field), in the song assistant, in the videoclip plan, where Describe a reference would be and where the motion-graphics controls would be (with `ollama pull gemma4:26b` for the model that writes the pages). It SHALL NOT appear when the provider does not answer (`unreachable`) or when the problem is its key or the consent: those controls are simply hidden. The review's "let the assistant propose it" SHALL be hidden without a notice. While the availability is not known no notice SHALL be shown.

#### Scenario: Ollama answers but has no model that reads pictures
- **WHEN** Ollama answers with only text models
- **THEN** where Describe a reference would be the notice says a model that reads pictures is missing and gives `ollama pull gemma4:26b`, with a link to Settings

#### Scenario: Ollama answers but has no chat model
- **WHEN** Ollama answers with no chat model
- **THEN** the scene editor shows one notice under the prompt saying a text model is missing, with `ollama pull qwen3:8b`

#### Scenario: another provider without its model
- **WHEN** a task is routed to an OpenAI-compatible provider that declares no model for it
- **THEN** the notice says to set that provider's model in Settings

#### Scenario: nothing answers
- **WHEN** the provider does not answer
- **THEN** no notice is shown and the controls are hidden

#### Scenario: the notice goes away with the model
- **WHEN** a model is pulled and the window regains focus
- **THEN** the notice is replaced by the controls

#### Scenario: no notice in the review
- **WHEN** the assistant has no model
- **THEN** "let the assistant propose it" is hidden and nothing replaces it
