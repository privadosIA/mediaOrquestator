# adult-label Specification

## Purpose
A project can say whether what it shows is for adults, so that a server can keep it out of sight. This capability is the label itself: its values, where it is written, how it is set and how it travels with the project.

## Requirements

### Requirement: A project carries an adult label with three states

`scenes.yaml` SHALL accept a top-level `adult` that is `true` (the project shows pictures, videos or songs for people of 18 or older), `false` (the project was looked at and is fine for everyone) or absent (it was not looked at). Validation SHALL refuse any other value. Reading and saving a project SHALL keep the field as it is: saving never adds, removes or flips it by itself, and a project saved without the field stays without it.

#### Scenario: the three states
- **WHEN** projects are saved with `adult: true`, `adult: false` and no `adult`
- **THEN** each is read back with the same state

#### Scenario: a wrong value
- **WHEN** a project has `adult: maybe`
- **THEN** validation fails naming the field

### Requirement: The label shows in the project list and is set from the editor

The project summary SHALL carry `adult` (`true`, `false` or absent) and the project list SHALL show a "+18" badge on a project with `adult: true` and nothing on the others. The editor and the new-project dialog SHALL offer a control with the three states (for adults, reviewed and fine for everyone, not reviewed). New projects SHALL start not reviewed.

#### Scenario: the badge
- **WHEN** the list holds a project with `adult: true`
- **THEN** its row shows the "+18" badge

#### Scenario: a new project
- **WHEN** a project is created without touching the control
- **THEN** its `scenes.yaml` has no `adult`

### Requirement: The label travels with the project

Duplicating a project, exporting and importing it as a package, and importing a dataset that carries the line `Adult: yes` or `Adult: no` SHALL keep the label: a duplicate has the same state as its source, a package keeps it in `scenes.yaml`, and the dataset format writes `Adult: yes` for `true` and `Adult: no` for `false` and nothing for absent. A project that is imported without the label stays not reviewed.

#### Scenario: duplicate
- **WHEN** a project with `adult: true` is duplicated
- **THEN** the copy has `adult: true`

#### Scenario: dataset round trip
- **WHEN** a project with `adult: false` is exported as a dataset and imported again
- **THEN** the new project has `adult: false`
