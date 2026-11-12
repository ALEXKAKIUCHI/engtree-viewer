# Changelog

## 0.2.0 — 2026-11-12

- **Revisions**: the shop floor asks engineering to fix a part, the part shows
  in red everywhere until it is settled, engineering closes it with a short
  answer, and the whole history exports as a spreadsheet. Records are one file
  each in a shared network folder; nothing is ever deleted. Production and
  engineering modes, set per computer, with a pending alert on start
- **Right-click menu** on lists and tree: open, copy code or description,
  open file, show in folder, ZIP, and the revisions
- **Columns** can be resized and reordered by dragging, saved per list, with
  "Restore columns" on the header menu
- **Shared settings file** on the network: folders, columns and file types for
  every computer; language, mode and column layout stay per machine
- Products that have no BOM spreadsheet, taken from a folder of files
- Rows select like a list: dragging across them no longer selects text
- Arrow keys and Enter in the products and parts lists
- Attachments shown as grey when a part's files were deleted or moved
- BOM spreadsheets are read in parallel, which speeds up the first index
- Fix: the product list had no scrollbar

## 0.1.0 — 2026-11-12

First public version.

- BOM tree from item numbers, with filter, expand/collapse and keyboard navigation
- Configurable key columns (item, code, description, quantity); other columns shown as-is
- Optional details spreadsheet joined by part code (per product or one shared file)
- Reads .xlsx, .csv (comma/semicolon) and tab-separated .txt; UTF-8, UTF-16 and Windows-1252
- Configurable file types (extensions, folders, exact or prefix matching) with per-part availability badges
- Viewers: PDF, DXF and IGES (with measuring), images and text; other files open in their default program
- Several matching files per part (revisions) with a picker; ZIP of all files of one type for a product
- Portable mode with engtree.json next to the executable
- Parts and assemblies tab (index of all BOMs, cached by file date), with filters
- File types can be shown as a list (CNC programs in any formats); attachments folder (📎)
- Products list shows the available file types, with one-click ZIP
- Interface in English and Portuguese (Brazil)
- Files served only from configured folders; requests from other web sites refused; no telemetry
