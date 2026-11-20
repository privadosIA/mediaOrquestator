# Open questions

## What is known

- The server refuses to listen outside loopback without a token, and the API needs it (`Authorization: Bearer`); one token, no users. `/api/health` is open.
- It starts **censored** (adult projects do not exist for the API) unless started otherwise: an exposed server must never be started with everything shown.
- Tailscale already gives HTTPS to a device of the tailnet (needed for the microphone); Cloudflare would put the server behind a domain.
- The server can launch runs that use the GPUs, upload files and run local programs: it is not a page to leave open.

## To decide

1. **Who is it for.** Only the user and their devices (then the tailnet may be enough, and Cloudflare is not needed), or other people (then it needs more than one shared token).
2. **Which path.** (a) Tailscale only: no public address. (b) Tailscale Funnel: a public address of the tailnet. (c) Cloudflare Tunnel (no open port) with Cloudflare Access in front (login by email or identity provider) and the token behind it. (d) both: Access for people, the tailnet for the user's devices.
3. **Authentication.** Keep the single token behind Cloudflare Access, or add per-person access; what a visitor may do (look only, or launch runs).
4. **What is exposed.** All of `/api` and the dashboard, or a limited part (the Create page and the gallery, without Settings, Environment and the key settings).
5. **Limits.** Upload sizes, the rate of requests, who can start a run while the GPUs are busy, and the wait in the queue.
6. **Content.** The censored mode fixed for a public server, with no option to change it from the page.
7. **Secrets in Settings.** The database, Grafana, Redis and provider keys must never be reachable from a public page: either those cards are not served on the public address or they need the token of the owner.
8. **Where it runs.** The Cloudflare tunnel (`cloudflared`) on this machine, as a service next to the server.

## Recommended direction (to confirm; nothing is built)

The front end now lives in `dashboards-lab` and the server only exposes the API, its token and its security (see `serve-web-from-directory`). Exposure is therefore a server and infrastructure matter. The direction proposed to the owner, to confirm one by one under "To decide":

- The server stays the only place where security is enforced: always with a token, always censored, listening on `127.0.0.1`; only the tunnel (`tailscale serve` or `cloudflared`) reaches it. `serve.sh --local` already does this.
- For the owner's own devices: Tailscale only (option a). For anyone else: Cloudflare Tunnel with Cloudflare Access in front and the token behind it (option c), so a failure of one layer is held by the other.
- The token is never in the front end's files (no token in a `config.json` of a build); it is entered once in the app's connection screen and kept in the browser.
- Front end and API on the same origin in production (the build served with `--web-dir` behind the same tunnel). A front end on another host would need `--cors-origin` open to the internet, which only widens what is exposed; keep it a separate, explicit case.
- The secrets of Settings (item 7) are served only with the owner's token, whatever front end is in front.

## Not decided here

A domain, a Cloudflare account or a tailnet: they are the user's.
