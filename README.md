# EngTree Viewer

**English** · [Português (Brasil)](README.pt-BR.md)

A small, fast desktop viewer for engineering BOMs. Open a product, see its Bill of Materials as a tree, click any part and its drawing, DXF or 3D file opens right next to it.

Made for the shop floor: one portable `.exe`, no install, no database, no account. It reads the spreadsheets and folders you already have (local or on a network share) and never changes them. Runs well on old, low-spec PCs where heavier CAD viewers struggle.

## Features

- **BOM tree** from SolidWorks-style item numbers (1, 1.1, 1.1.1), with expand/collapse, filter and keyboard navigation
- **Files linked to every part**: each line shows which file types exist for it (PDF, DXF, IGES…)
- **Built-in viewers**: PDF, DXF (2D) and IGES (3D, shaded) with zoom, pan, rotate and point/line **measuring**; images and text/CNC files too. Anything else opens in its default program
- **Revisions**: when several files match a part (`ABC-100.pdf`, `ABC-100_rev2.pdf`), pick one from a list; exact name first, then newest
- **ZIP package**: all PDFs (or DXFs, or…) of a product in one click
- **Your spreadsheets, your columns**: `.xlsx`, `.csv` (comma or semicolon) or tab-separated `.txt`. Only four key columns need to be found (item, code, description, quantity); every other column is shown as it is
- **Optional details sheet** joined by part code (finish, color, supplier…), one per product or one shared file
- **Configurable file types**: name, extensions, one or more folders (all types can share one folder) and exact or prefix name matching
- **English and Portuguese (Brazil)** interface: follows the system language, or pick one in Settings
- **Offline-friendly**: if the network share drops, a red bar says so and the product list keeps working from the last scan

## Requirements

- Windows 10 or 11
- Microsoft Edge or Google Chrome (the app window is the browser in "app mode")
- To build: [Go](https://go.dev/dl/) 1.21+

## Getting started

1. Download `EngTreeViewer.exe` from [Releases](https://github.com/ALEXKAKIUCHI/engtree-viewer/releases), or build it (below).
2. Run it. On first start it asks for your folders.
3. In **Settings**, set:
   - **BOM folder**: where your BOM spreadsheets are. Each spreadsheet is one product.
   - **BOM file prefix** (optional): e.g. `BOM - ` if your files are named `BOM - ABC-100 - Side table.xlsx`. Leave empty if every spreadsheet in the folder is a BOM.
   - **File types**: the folder(s) for each kind of file.

### How files are named

| What | Example | Notes |
|---|---|---|
| BOM | `BOM - ABC-100 - Side table.xlsx` | code and description come from the name, split by the separator (` - `) |
| Details (optional) | `PARTS - ABC-100.xlsx` | same folder as the BOM, joined to the BOM by part code |
| Part file, exact match | `ABC-110.dxf` | name = part code |
| Part file, prefix match | `ABC-110 - Leg.pdf`, `ABC-110_rev2.pdf` | name starts with the code followed by a separator. `ABC-1100.pdf` does **not** match `ABC-110` |

With prefix matching, a file is not shown as a revision of a part when its name is the code of another part of the same product: in a BOM that has both `ABC-110` and `ABC-110-01`, the file `ABC-110-01.pdf` belongs to `ABC-110-01` only.

The product row at the top of the tree uses the product code, so a full-product drawing named `ABC-100 - Side table.pdf` shows up there.

### Columns

The BOM header row is found automatically (title lines above it are fine). Column names are matched ignoring case, accents and punctuation, and each key column accepts alternatives separated by `|`. Defaults cover SolidWorks in English and Portuguese:

| Field | Default names |
|---|---|
| Item | `ITEM NO`, `ITEM`, `Nº DO ITEM` … |
| Code | `PART NUMBER`, `PART NO`, `CODE`, `CÓDIGO`, `Nº DA PEÇA` … |
| Description | `DESCRIPTION`, `DESCRIÇÃO` … |
| Quantity | `QTY`, `QUANTITY`, `QTD` … |

If your headers vary, a column can also be given by position: `#2` means the 2nd column.

Columns you don't want to see can be listed under **Hide these columns**.

### Settings file and portable mode

Settings are saved in `%APPDATA%\EngTreeViewer\engtree.json`. If a file named `engtree.json` sits **next to the .exe**, that one is used instead, which makes it easy to copy one setup to several PCs (or keep the exe and its settings on a shared drive).

## Building

```bat
build.bat
```

or

```bat
go build -ldflags="-s -w -H=windowsgui" -o EngTreeViewer.exe .
```

For development on any OS: `go run . -no-window -port 8080` and open `http://127.0.0.1:8080`.

## Privacy

EngTree Viewer:

- only **reads** the folders you set in Settings and never writes to them;
- runs a local server bound to `127.0.0.1` (not reachable from other computers), serves files only from those folders and refuses requests coming from other web sites;
- sends **nothing** over the internet and has **no telemetry or data collection**. The only network access is to your own folders. "Report a problem" just opens this project's GitHub page in your browser.

## Limitations

- Old binary Excel `.xls` is not read: save as `.xlsx` or `.csv`. (Text exports that merely carry the `.xls` extension are read.)
- IGES curved faces are drawn faceted from their outline; it is a quick viewer, not a CAD kernel.
- Hierarchy comes from item numbers (1.2.1). BOMs with a separate "level" column are not supported yet.

## Roadmap

- Parts list / production list for a product with quantities
- Update check through GitHub Releases (optional, off with one click)
- Built-in "report a problem" with an exportable log for PCs without internet

## License

[MIT](LICENSE) © Alexsander Kakiuchi
