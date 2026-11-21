## ADDED Requirements

### Requirement: The motion of a stored page can be revised without writing the page again

When a stored motion page declares a timeline, the program SHALL be able to change its motion from a request in words (for example slower entrances or a bounce on the title) without asking the model for a new page. It SHALL ask the language model of the routed `motion` task for the new content of the timeline block only: the request SHALL carry the page, or a reduced view of it that keeps the timeline block, the size and duration of the clip and the request; the instructions SHALL state the properties with their ranges and the curves that are accepted, SHALL require every selector to match an element of the page shown and SHALL ask for the JSON of the block and nothing else. The answer SHALL be checked as a timeline (version 1; tracks with a unique id, a target, a start of 0 or more, a duration above 0, a stagger of 0 or more when given, a known curve or four numbers, and `from` and `to` objects whose properties exist and are inside their ranges) and then written into the page, which SHALL keep every other byte as it was; the resulting page SHALL pass the same check as a new page. An answer that fails either check SHALL be requested again up to 3 times with the problems and the previous timeline, and when still failing the revision SHALL fail with an error that includes the last problem. A page without a timeline block SHALL be refused with an error that says so, without calling the model. A compression service (ctxpress) is optional: when `CTXPRESS_URL` is set the page MAY be shown to the model through it, and when it is not set, does not answer, does not compress or drops the timeline block, the model SHALL be shown the page itself.

#### Scenario: Slower entrances
- **WHEN** a stored page has a timeline, the user asks for slower entrances and the model answers with a valid timeline
- **THEN** the page has the new timeline, its text, styles and scripts are byte for byte what they were, and it passes the check of a new page

#### Scenario: Invalid timeline
- **WHEN** the model names a property or a curve that does not exist, a value out of range, a repeated id or a selector that matches no element
- **THEN** it is asked again with the problem and its previous timeline, and after 3 failures the revision fails with an error that includes the last problem

#### Scenario: A page without a timeline
- **WHEN** the page has no timeline block
- **THEN** the revision is refused with an error that says so and the model is not called

#### Scenario: Compression service not available
- **WHEN** `CTXPRESS_URL` is set and the service does not answer, or the view it returns has no timeline block
- **THEN** the model is shown the page as it is and the revision goes on
