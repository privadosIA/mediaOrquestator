# code-documentation Specification

## Purpose
Defines how the Go code is documented so that `go doc`, `pkgsite` and an editor show a reader what every package and exported name is for, in the form the Go tools expect, and defines the check that keeps every package and exported name documented in both editions.

## Requirements

### Requirement: Every package has one package comment

Every Go package under `cmd/` and `internal/` SHALL have exactly one package comment: a comment immediately before the `package` clause of one of its non-test files, starting with `Package <name>` (or `Command <name>` for a `main` package), that says what the package is for, what it owns (files, processes, state) and, when it is one of several that cooperate, where it sits among them. A package with more than one file SHALL keep the package comment in the file that gives the package its name or in a file called `doc.go`. Test-only packages and packages under `third_party/` are out of scope.

#### Scenario: a package without a comment
- **WHEN** the documentation test runs over a tree where one package has no package comment
- **THEN** the test fails naming the package directory

#### Scenario: a main package
- **WHEN** the package comment of `cmd/orchestrator` is read with `go doc ./cmd/orchestrator`
- **THEN** it starts with `Command orchestrator` and names the subcommands it dispatches

#### Scenario: two package comments
- **WHEN** two files of one package both carry a package comment
- **THEN** the test fails naming both files

### Requirement: Every exported name has a doc comment in the Go form

Every exported top-level declaration (function, method, type, variable, constant, including each name of an exported `var`/`const` group that lacks a group comment) in a non-test file under `cmd/` and `internal/` SHALL have a doc comment immediately before it. The comment SHALL be one or more complete sentences, SHALL begin with the declared name (an article such as `A`, `An` or `The` MAY precede the name of a type), and SHALL say what the name does or is, not how its body works. A comment of the form `Name: text` SHALL NOT be used: the name is followed by a space. A method that implements a well-known interface (`Error`, `String`, `Write`, `Read`, `Close`, `ServeHTTP`, `MarshalJSON`, `UnmarshalJSON`, `Is`, `Unwrap`) MAY have no comment when its behaviour is the interface's. Generated files (a first line matching `^// Code generated .* DO NOT EDIT\.$`) are out of scope.

#### Scenario: an exported function without a comment
- **WHEN** the documentation test runs over a tree where an exported function has no comment
- **THEN** the test fails naming the file, the line and the name

#### Scenario: a comment that does not start with the name
- **WHEN** a doc comment reads `// Returns the model.` on `func Model()`
- **THEN** the test fails saying the comment must start with `Model`

#### Scenario: a type with an article
- **WHEN** a doc comment reads `// A Theme is the look of a piece.` on `type Theme`
- **THEN** the test accepts it

#### Scenario: an interface method
- **WHEN** `func (e *NotAllowedError) Error() string` has no comment
- **THEN** the test accepts it

#### Scenario: both editions
- **WHEN** the test runs with and without the `private` build tag
- **THEN** it passes in both, so the twins of a tagged file are documented too

### Requirement: Doc comments use the Go doc-comment syntax and the rules of content

Doc comments SHALL be written in English and SHALL use only the Go doc-comment syntax: blank `//` lines between paragraphs, a `# Heading` line for a heading, lines indented one more space for a code block, `-` or `*` for list items, `[Name]`, `[pkg.Name]` or `[text]: URL` for links, and a paragraph starting with `Deprecated:` for a deprecated name. A doc comment SHALL state what the caller can rely on: the meaning of the arguments and the result, the errors returned (naming the sentinel or type a caller may test with `errors.Is`/`errors.As`), whether a value is safe for concurrent use, which files or processes it creates, owns or deletes, and the units of any number. A doc comment SHALL NOT narrate the history of the code (what it did before, which change introduced it), SHALL NOT repeat the body line by line, and SHALL NOT contain Markdown that the Go syntax does not define (backticks for code, `**bold**`, `##` headings). A doc comment that explains an HTTP handler SHALL name its route pattern (`GET /api/x/{id}`) and SHALL point at the OpenAPI description rather than repeat the fields.

#### Scenario: rendered by go doc
- **WHEN** a doc comment with a `# Heading`, a list and an indented code block is read with `go doc`
- **THEN** the heading, the list and the code block are rendered as such, with no stray `#`, `-` or backtick

#### Scenario: gofmt agrees
- **WHEN** `gofmt -l` runs over the tree
- **THEN** it lists no file, because `gofmt` reformats doc comments that break the syntax

#### Scenario: a deprecated name
- **WHEN** a doc comment has a paragraph `Deprecated: use [NewRunner] instead.`
- **THEN** `go doc` and the editors mark the name as deprecated

### Requirement: Behaviour that words leave unclear has an Example test

A package whose public behaviour is easy to misuse (an input format, an ordering, a hashing rule, an error body) SHALL carry at least one `Example` function in a `_test.go` file, with an `// Output:` comment, so that `go test` checks the example and `go doc` shows it next to the name. At least three packages SHALL have one when this change is archived, and the development guide SHALL tell when to add one.

#### Scenario: an example runs
- **WHEN** `go test` runs over a package with an `Example` function that has an `// Output:` comment
- **THEN** the example is executed and the test fails if its output differs

#### Scenario: an example is shown
- **WHEN** `go doc -all` is run on a package with an `ExampleName` function
- **THEN** the example appears under `Name`

### Requirement: The documentation can be read as a site and the rule is written down

`docs/DEVELOPMENT.md` SHALL have a section *Documenting the code* that states the rules of this capability in plain words with one good and one bad example, names the test that checks them, and gives the two commands to read the documentation: `go doc <package or name>` for one name, and a local `pkgsite` server for the whole module (the module has no host in its name, so no public site indexes it). The section SHALL be in the public part of the document.

#### Scenario: the local site
- **WHEN** the documented `pkgsite` command is run in the repository and the browser opens the address it prints
- **THEN** every package under `cmd/` and `internal/` is listed with its package comment and the comments of its exported names

#### Scenario: the section is public
- **WHEN** the public branch is produced
- **THEN** the section *Documenting the code* is in its `docs/DEVELOPMENT.md`

### Requirement: The CI reports the same as the test

The lint configuration of the private CI SHALL enable the style checks for package comments and for the form of doc comments (missing package comment, comment not starting with the name of the function, type or variable), so that the CI names the same problems the test names.

#### Scenario: a malformed comment in CI
- **WHEN** a file with a doc comment that does not start with the exported name is pushed
- **THEN** the lint step reports it, naming the file, the line and the expected form
