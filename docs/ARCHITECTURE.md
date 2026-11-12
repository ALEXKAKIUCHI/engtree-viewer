# Architecture

EngTree Viewer is one Go executable (standard library only) plus a single-page UI embedded in it.

```
Edge/Chrome --app window  <--HTTP 127.0.0.1-->  EngTreeViewer.exe  --reads-->  BOM + file folders
ui/index.html                                   Go server
```

1. On start the server listens on `127.0.0.1` on a free port and opens Edge or Chrome in app mode.
2. The UI only talks to the local API; all disk and network reads happen in Go.
3. Closing the window sends `/api/closing`; the server exits 5 s later (or after 3 min without requests).

## Files

| File | Role |
|---|---|
| `main.go` | Product list, routes, folder reachability, path safety, window launch, auto exit |
| `config.go` | Settings (`engtree.json`), defaults, portable mode, shared settings file, modes |
| `sheets.go` | Reads .csv/.txt (delimiter and encoding detection) and dispatches .xlsx |
| `xlsx.go` | Minimal .xlsx reader (first sheet) |
| `bom.go` | Product discovery from file names, header matching, details join, tree building |
| `files.go` | Directory cache, file-type index (exact / prefix keys), attachments, ZIP packages |
| `parts.go` | Parts index built from all BOMs (per-BOM cache by modification time, read in parallel), Parts tab routes |
| `revisions.go` | Revision requests: one JSON file each in a shared folder, status, export |
| `cad.go` | DXF (2D) and IGES (3D curves and trimmed faces) parsing |
| `taskbar_windows.go` | Own taskbar identity/icon for the browser window |
| `ui/index.html` | The whole interface |

## Matching files to codes

For each file type, an index `key -> files` is built from the folder listings (cached 45 s). With *exact* matching the key is the file name without extension. With *prefix* matching the name is also cut at every non-alphanumeric character, so `abc-100_rev2` yields `abc-100_rev2`, `abc-100` and `abc`; `abc-1000` never yields `abc-100`. A lookup is then a map access, which keeps big trees fast. A prefix match is dropped when the file name also yields a longer key that is another code of the same product (`abc-100-01.pdf` is not a revision of `abc-100` if `abc-100-01` is in the BOM).

## Languages

All UI text lives in the `I18N` table at the top of the script in `ui/index.html` (`en`, `pt-BR`). The server sends warning codes, not sentences. To add a language, add a block to `I18N` and an option to the language select.

## Settings: local and shared

`Config` holds both this computer's settings (language, mode, column layout,
last requester) and the shared ones (folders, columns, file types). When
`shared_config` points at an `engtree.json` on the network, `applyShared`
overwrites only the shared half with what that file says, and a copy is cached
locally for when the network is down. The local file is written without the
shared half, so a copy of it is never mistaken for the settings themselves.
Saving writes back to the shared file only from an engineering computer, when
the file is writable.

## Revisions

One JSON file per request in `revisions_folder`, named `rev_<id>.json`. Two
computers never write the same file, which is what makes a plain network share
enough: no locking, no server. Closing rewrites that one file. A request's
status is worked out when read: `closed` when engineering closed it,
`updated` when the part's main file is newer than it was when the request was
made (the drawing was replaced), otherwise `open`. The folder listing is
cached for 15 s and invalidated after every write.

## API

| Route | Notes |
|---|---|
| `GET /api/status` | configured, folder online, scanning |
| `GET /api/products?q=&sort=code\|desc\|date&asc=` | product list |
| `GET /api/refresh` | rescan the BOM folder |
| `GET/POST /api/config`, `GET /api/default_config` | settings; the answer carries the shared-file state |
| `POST /api/mode?mode=` | production or engineering, this computer only |
| `GET/POST /api/column_layout` | column order and widths per list |
| `GET /api/revisions/state` | what the alert, the flag and the legend show |
| `GET /api/revisions?code=&scope=item\|product` | history |
| `POST /api/revisions/new`, `POST /api/revisions/close` | ask and settle |
| `GET /api/revisions/export?lang=` | spreadsheet of every request |
| `GET /api/product?code=` | BOM tree of a product, or a part with its sub-tree, with per-node file availability |
| `GET /api/parts`, `GET /api/parts_facets`, `GET /api/item_zip?code=&type=` | Parts tab list and filters; ZIP of one code's files |
| `GET /api/item_files?code=` | files per type for one code |
| `GET /api/file?path=[&download=1]` | only inside configured folders |
| `GET /api/cad_data?path=&kind=dxf\|iges` | geometry as JSON |
| `GET /api/package_zip?code=&type=` | ZIP of one file type for the whole product |
| `POST /api/open_external`, `POST /api/open_folder` | default program / Explorer; executables refused |
| `POST /api/closing` | window closed |

## Icon

`assets/app_icon.ico` is embedded through `rsrc_windows_amd64.syso`. After changing the icon:

```
go run tools/makeicon/main.go assets/app_icon.ico
```
