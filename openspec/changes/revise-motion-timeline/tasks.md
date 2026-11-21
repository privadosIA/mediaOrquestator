## 1. Revision

- [x] 1.1 `ExtractTimeline`, `SpliceTimeline` (byte exact: putting the old block back gives the original page) and `ExtractTimelineJSON`, with tests
- [x] 1.2 `ValidateTimeline`, its ranges and curves tested equal to those of `timelineScript`
- [x] 1.3 `Revise` with the repair loop, the optional `Viewer` and `ErrNoTimeline`, tested with the scripted writer and a fake check
- [x] 1.4 `internal/ctxpress`: the optional client of the compression service, tested against a fake service
- [x] 1.5 ctxpress row in `THIRD_PARTY.md`
- [ ] 1.6 An entry point: an editor control, an endpoint or a command that calls `Revise` and stores the page (on request, like a new page)
- [ ] 1.7 The service's address in the Settings page (today only `CTXPRESS_URL` and `CTXPRESS_TOKEN_FILE`)
- [ ] 1.8 Measure on pages written by the generator, with a real model, and fix the instructions by eye
- [ ] 1.9 Documentation (`docs/`): the environment variables and the `animation-timeline` profile
