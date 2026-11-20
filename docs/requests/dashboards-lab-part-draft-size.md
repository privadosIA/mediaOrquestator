# Request: raise the size of a part draft's request

From the `dashboards-lab` repository (`docs/server-request-part-draft-size.md` there), for the server. The editor that hits it is `vector-studio` (repo `dashboards-lab`, library `@lab/vector-editor`); the route is
`POST /api/music-motion/part-drafts` in `contract/openapi.yaml` (`createDraftFromText` in the client).

## What exists today

`handleDraftCreate` (`internal/server/music/partdrafts.go`, line 218 at `b84c4899`) reads the JSON body with `http.MaxBytesReader(w, r.Body, 1<<16)`: **64 KiB for the whole request**
(`{manifest, fragment}` when a part is sent as text). Found on the real server on 2026-11-11 by sending a draft of 65 000 bytes (accepted) and one of 70 000
(`400 invalid_request`: `invalid JSON body: http: request body too large`). The route's multipart branch has its own, larger limit (`maxDraftPicture+1<<20`); the
`PUT /api/music-motion/part-drafts/{id}` (the same `{manifest, fragment}`) reads up to **1 MiB** (`1<<20`): the two routes disagree by a factor of 16 on the same payload.

## Why it matters

A vector part (`@lab/vector-editor`) is an inline SVG, a runtime of about 12 KB and the editable model of the animation embedded in the fragment, so **whatever is drawn is in the
fragment twice**, and a picture's `data:` URL is base64 (a third bigger) and also twice. In practice the most that fits is about **24 KB of SVG or 16 KB of picture**:

- a composed part of two small logos, a text and a 330-byte picture: 22.5 KB, accepted and approved;
- the same with a 53 KB photo: 162 KB, refused;
- an ordinary illustration or logo exported from a design tool is often over 24 KB.

Nothing else in the server limits a part this way (the checks are that it draws, that it moves and that its texts do not overlap), so this is a limit by accident, not by design.

## What is wanted

One of these, in order of preference:

1. Read the JSON body of `POST /api/music-motion/part-drafts` with a limit of about **8 MiB** (the editor's own caps are 2 MB for an imported SVG and 2 MB for a picture, so 8 MiB leaves room
   for both twice and the runtime). Keep the `invalid JSON body` error, now naming the limit.
2. If a large body is a worry, accept the part as `multipart/form-data` (`manifest` and `fragment` as file parts) the way a reference picture already travels, with the same limit.
3. At least say the limit in the contract (`x-max-body-bytes` or in the operation's description), so that clients can check it before sending: today it is only found by trying.

No change for clients that send small parts. If a limit has to stay, tell us its number and we will keep `SERVER_BODY_LIMIT` (`libs/vector-editor/src/lib/runtime/to-part.ts`) equal to it.

## What the client does today

`toPart` returns `bodyBytes` (the request as the server counts it) and the send dialog says "the part is about N KB and the server does not take requests over 64 KB" and disables Send
before asking the server anything. When the limit is raised, changing `SERVER_BODY_LIMIT` and the warnings of `import-svg.ts` (24 KB) and `tools/image.ts` (16 KB) is all that is left to do here.

## Questions for the server

- Is 64 KiB intended for hand-written parts, or only a default that was never revisited?
- Does anything downstream (the store of drafts, the previews, the approved part's files) have a size that matters, so that the new limit should follow it?
