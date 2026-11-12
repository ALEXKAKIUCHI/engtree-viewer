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
| `config.go` | Settings (`engtree.json`), defaults, portable mode |
| `sheets.go` | Reads .csv/.txt (delimiter and encoding detection) and dispatches .xlsx |
| `xlsx.go` | Minimal .xlsx reader (first sheet) |
| `bom.go` | Product discovery from file names, header matching, details join, tree building |
| `files.go` | Directory cache, file-type index (exact / prefix keys), ZIP packages |
| `cad.go` | DXF (2D) and IGES (3D curves and trimmed faces) parsing |
| `taskbar_windows.go` | Own taskbar identity/icon for the browser window |
| `ui/index.html` | The whole interface |

## Matching files to codes

For each file type, an index `key -> files` is built from the folder listings (cached 45 s). With *exact* matching the key is the file name without extension. With *prefix* matching the name is also cut at every non-alphanumeric character, so `abc-100_rev2` yields `abc-100_rev2`, `abc-100` and `abc`; `abc-1000` never yields `abc-100`. A lookup is then a map access, which keeps big trees fast.

## API

| Route | Notes |
|---|---|
| `GET /api/status` | configured, folder online, scanning |
| `GET /api/products?q=&sort=code\|desc\|date&asc=` | product list |
| `GET /api/refresh` | rescan the BOM folder |
| `GET/POST /api/config`, `GET /api/default_config` | settings |
| `GET /api/product?code=` | BOM tree with per-node file availability |
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
