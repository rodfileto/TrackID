# Papers (Quarto)

Each of the three papers described in [`docs/RESEARCH_STRATEGY.md`](../docs/RESEARCH_STRATEGY.md)
is a standalone [Quarto](https://quarto.org) article, with shared defaults
here at the project root and per-paper overrides in each subdirectory.

```
papers/
  _quarto.yml                      # shared defaults: format, bibliography, author
  references.bib                   # citations common to all papers
  paper1-tactical-linking/
    _metadata.yml                  # overrides: title, keywords, format tweaks
    references.bib                 # citations specific to this paper
    index.qmd                      # the article itself
  paper2-strategic-intelligence/
    ...
  paper3-forensic-evidentiary/
    ...
```

## How the override works

`_quarto.yml` at this level defines the project and the general template
(HTML/PDF/DOCX format options, `bibliography`, `author`, etc). Quarto merges
any `_metadata.yml` found in a document's own directory on top of that
project config when rendering — the paper-specific file wins on any key it
sets (including nested keys, e.g. a paper can add `format.pdf.keep-tex`
without redefining the rest of `format.pdf`), and anything it doesn't set
falls through to the shared default. To change something for one paper only,
edit that paper's `_metadata.yml`; to change something for all of them, edit
`papers/_quarto.yml`.

Bibliographies stack the same way: each paper's `_metadata.yml` lists both
`../references.bib` (shared) and its own `references.bib` (paper-specific),
so citations from either are available in that paper.

## Setup

Install the Quarto CLI + TinyTeX (for PDF rendering) once:

```bash
curl -fLO https://github.com/quarto-dev/quarto-cli/releases/download/v1.10.18/quarto-1.10.18-linux-amd64.deb
sudo apt-get install -y ./quarto-1.10.18-linux-amd64.deb xz-utils
rm quarto-1.10.18-linux-amd64.deb
quarto install tinytex --no-prompt
```

Check https://github.com/quarto-dev/quarto-cli/releases for newer versions.

## Rendering

```bash
cd papers
quarto preview                                    # live reload
quarto render                                      # render everything
quarto render paper1-tactical-linking/index.qmd    # render one paper
```

## Editor: VS Code

Desktop VS Code, not a browser-based editor — this is a local machine, so
there's no need for the extra systemd service/password that code-server
adds. Install the Quarto extension:

```bash
code --install-extension quarto.quarto
```

- Open the `papers/` folder in VS Code and use the Quarto extension's visual
  editor for citation autocomplete (`@` triggers it) from whichever
  `bibliography:` files are in scope for that paper, plus its render/preview
  commands.
- **Zotero integration is not wired up yet** — citations currently come from
  the `.bib` files only. When you're ready, add a Zotero Web API key in the
  extension's settings.

## Adding a journal-specific template

Papers 1–3 target different Elsevier/forensic-science journals (see
`docs/RESEARCH_STRATEGY.md` for the current list). When a paper is ready to
match a specific journal's manuscript format, add the relevant
[Quarto Journals](https://github.com/quarto-journals) extension inside that
paper's own directory so it doesn't affect the others:

```bash
cd papers/paper1-tactical-linking
quarto add quarto-journals/elsevier
```

then reference the installed format in that paper's `_metadata.yml`.

## Adding a fourth paper

Copy the structure of an existing paper directory (`_metadata.yml`,
`references.bib`, `index.qmd`), drop it under `papers/`, and it picks up the
shared defaults automatically.
