## Decisions

1. **The answer is the timeline, never the page.** The model cannot change the paint, the structure or the scripts of a page it only edits the motion of, the answer is a few hundred tokens (`reviseMaxTokens` is 2000, against 8000 for a page), and the splice is byte exact: putting the old block back gives the original page, which a test checks.
2. **Two checks, the cheap one first.** `ValidateTimeline` rejects what is wrong with the shape of the JSON before a browser is opened; the page's own `__motionTimelineProblems`, run by `Check`, stays the authority for what needs the page (a selector that matches nothing, a track that ends after the clip, a CSS animation on a targeted element). The Go ranges and curves are tested equal to those of `timelineScript`.
3. **The instructions are built from the same data.** The properties, their ranges and the curves in the system prompt come from `TimelineProperties`, `timelineRanges` and `TimelineCurves`, so what the model is told cannot drift from what is accepted.
4. **The compression service is optional and never decisive.** `Viewer` is a plain function: `ctxpress.Client.View` returns the page itself on any failure, and `revise` shows the page when the view lost the timeline block (a wrong profile), so the worst case is the page as it was. The motion package does not import the client.
5. **Not for pages without a timeline.** A page that animates with JavaScript has no block to revise (`ErrNoTimeline`); its changes keep going through a new page. Measured on 32 pages of the music-motion catalog, which animate in JavaScript, a compression that keeps the scripts saves 12% and one that drops them leaves no motion for the model to see; this flow is for the pages the generator writes with a timeline.
6. **Same attempts as a new page.** `MaxAttempts` is reused, with the problems and the previous timeline in the repair request.

## Risks

- The saving on the input is modest (27% on the example page of the prompt, 598 to 439 tokens); the gain is the short answer and the paint that cannot change.
- The model can write a timeline that passes every check and still looks wrong: the checks find what is invalid, not what is ugly. The stored page is replaced only on request, as for a new page.
