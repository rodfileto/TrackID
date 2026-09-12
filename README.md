# trackid

Rewrite target for [Naturaliza](../naturaliza_dev) — a Django/Next.js case-management system —
into a Rust backend + React (Vite) frontend.

This is a learning/reference project: modules and components are ported over one bounded
slice at a time, not translated line-by-line. See `naturaliza_dev/CLAUDE.md` for domain
context on the source system.

## Structure

- `backend/` — Rust API server (currently an empty `cargo init` skeleton, framework TBD)
- `frontend/` — React + Vite, based on the [TailAdmin React](https://github.com/TailAdmin/free-react-tailwind-admin-dashboard)
  template (the Vite sibling of the TailAdmin Next.js template `naturaliza_dev`'s frontend
  is already built on)

## Status

Scaffolding only — no domain logic ported yet.
