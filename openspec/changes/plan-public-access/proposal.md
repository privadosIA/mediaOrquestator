# Proposal

## Why

The server is reachable on the local network and, with Tailscale, from the user's own devices (a guide for the microphone's HTTPS exists in `docs/SERVER.md`). The user wants to publish it on the internet, using Cloudflare and Tailscale. Nothing is decided yet; this item collects what has to be decided before any code, because exposing a server that can launch runs, read projects and write files is a security decision before it is a feature.

## What Changes (to be decided)

Nothing yet. This is a plan.

## Impact

`docs/SERVER.md`, the access token and authentication, the content mode (censored by default), perhaps `scripts/serve.sh`, and whatever the chosen option adds.
