# Spec Delta

## MODIFIED Requirements

### Requirement: A script is proposed from the song and can be edited

From the song map, the design, the level and the catalog, the server SHALL propose a script: a list of pieces, each with its start and end, the section, the part and its parameters, the lines it covers with their keywords and, optionally, a word hit and a mode (the proposer SHALL NOT write a mode on its own: the mode comes from the design unless the assistant has a reason to set one, and the fixed rule writes none). The proposal's input SHALL carry, per section, its mean loudness and its energy rank from the map, and the device and hook of the plan when the piece has them, and its instruction SHALL ask that the cut density follow the energy (pieces of 8–15 s in the quiet sections, one piece per two to four bars in the loud ones, one held piece on a breakdown), that the chorus return to one home part named in the answer and escalate on each return, that the ending rhyme with the opening, that a piece longer than 8 s carry a change inside it (an animated parameter, an extra that enters mid-piece or a layer item) and that at most a third of the seams be hard cuts (the rest with `transition_seconds` of 0.3–0.6 s). The proposal SHALL be made by the assistant and checked against the catalog and the limits; a proposal that fails SHALL be asked again up to 3 times with the error, and without an assistant the script SHALL be made by a fixed rule: pieces whose length follows the section's energy, the same part for every chorus piece, parts chosen in turn from those the level supports for the rest. The script SHALL be stored in the project as a file the user can read and edit, a later request SHALL NOT replace it unless asked, and an edited script SHALL be validated with the same checks before it is used.

#### Scenario: Proposal
- **WHEN** a script is requested for a song with three sections
- **THEN** the answer has pieces that cover the song with no gap and no overlap and every part exists in the catalog

#### Scenario: Energy in the input
- **WHEN** the map's loudness is highest in the chorus
- **THEN** the proposal's input lists the chorus with energy rank 1 and the instruction asks for the shortest pieces there

#### Scenario: The fixed rule follows the energy
- **WHEN** the fixed rule makes a script for a song whose verse is quiet and whose chorus is loud
- **THEN** the verse pieces are longer than the chorus pieces and both chorus pieces share one part

#### Scenario: No mode written
- **WHEN** the fixed rule makes a script for a design with `default_mode: dark`
- **THEN** no piece carries a `mode` and every page is drawn dark

### Requirement: The text has two layers

A scene made for a piece SHALL show the verse that is being sung in the lower layer, with the words appearing at the time they are sung and the keyword of the line in the accent colour, and MAY show a word hit in the upper layer: one word at full screen, with the time it enters and its length. A line of more than six words SHALL be shown in groups, one group at a time: two to three words in a section of the top third of energy, three to five in the middle third, four to six in the bottom third, breaking at punctuation or at a pause of 150 ms or more between words, the keyword always inside a group; a group opens 0.2 s before its first word (never before the line starts) and is removed 0.12 s after its last word ends or when the next group opens; the group that holds the keyword carries the accent. A design MAY turn the grouping off (`verse_groups: false`). The keyword of a line and the word hit SHALL be chosen by the script and SHALL be a word of that line. The words SHALL stay inside the frame and apart from each other, as for every page: the word hit SHALL be sized to the smaller of 60 % of the frame's height and the size at which its width is 90 % of the frame's width, measured on the page with its display font loaded (the page requests the font before the first measure and measures again on every seek until the fonts report loaded, and once after), in every look of the hit and on the house page and the scene page alike, and a page whose hit leaves the frame SHALL fail the page checks as the server's fault, never the scene's. A line with no timed words SHALL appear whole when it starts.

#### Scenario: Words at their time
- **WHEN** a line is sung from 12.0 s to 14.5 s with its words timed
- **THEN** each word is shown from its own start time and none before

#### Scenario: A long line in a loud section
- **WHEN** a nine-word line is sung in the section of energy rank 1
- **THEN** it is shown in groups of two or three words, one at a time, each opening 0.2 s before its first word

#### Scenario: A short line
- **WHEN** a line has five words
- **THEN** it is shown as one line, as before

#### Scenario: A long word hit
- **WHEN** the hit is `FLIGHT` in a wide serif at 1280×720
- **THEN** it is drawn smaller than 60 % of the height so that its whole width fits inside 90 % of the frame, and no letter is cut

#### Scenario: The font arrives after the first measure
- **WHEN** the display font is not loaded when the page builds and `NIGHT` is measured in the fallback face
- **THEN** the hit is measured again once the font is in and fits inside 90 % of the frame at 0.4 s on the scene page

#### Scenario: A short word hit
- **WHEN** the hit is `go`
- **THEN** it is drawn at 60 % of the height
