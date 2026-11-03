# TrackID frontend

React 19, TypeScript, Vite and Tailwind CSS v4. It started from the
[TailAdmin React](https://github.com/TailAdmin/free-react-tailwind-admin-dashboard) template
(MIT, notice in [LICENSE.md](LICENSE.md)), whose layout and UI components it still uses.

```sh
npm install
npm run dev     # Vite on :5173, proxying /api to the Go server on :8082
npm run build   # dist/, which the Go server serves (internal/web)
```

The interface is in English and Brazilian Portuguese (`src/i18n/locales`). Start the whole stack
with `scripts/start_dev.sh` from the repository root.
