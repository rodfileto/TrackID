# Quarto documents

Each subfolder here is a self-contained Quarto project (its own `_quarto.yml`), so new documents
are added as sibling folders rather than growing a single shared project.

- `technical-report/` — TrackID technical report.

## Rendering

Requires the [Quarto CLI](https://quarto.org/docs/get-started/) installed locally.

```bash
cd quarto/technical-report
quarto render        # renders to ./_output
quarto preview        # live preview while editing
```
