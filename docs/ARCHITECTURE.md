# Architecture

EngTree Viewer is one Go executable plus a single-page UI embedded in it. It uses the standard library
and one outside package, [go-pdf/fpdf](https://github.com/go-pdf/fpdf) (MIT), for the cut plan report.

```
Edge/Chrome --app window  <--HTTP 127.0.0.1-->  EngTreeViewer.exe  --reads-->  BOM + file folders
ui/index.html                                   Go server
```

1. On start the server listens on `127.0.0.1` on a free port and opens Edge or Chrome in app mode.
2. The UI only talks to the local API; all disk and network reads happen in Go.
3. Closing the window sends `/api/closing`; the server exits 5 s later (or after 3 min without requests).

## Layout

```
cmd/engtree/      the Windows executable (and the icon resource)
app/              the program itself: app.Run(), the server, the UI
extension/        where cut planners plug in
pkg/geom/         points, rings, a ring with its holes (mm)
pkg/dxfgeom/      DXF -> segments and closed contours
pkg/cutplan/      plugs the planners of ./cutplan into extension
cutplan/          its own Go module: the cut planners (see below)
tools/makeicon/   builds the .syso icon resource
```

`app` is a package, not a `main`, so another build can register features
through `extension` and start the same program. Nothing in this repository
knows whether those features exist: there is no flag, no licence check and
no `if pro`.

`pkg/geom` and `pkg/dxfgeom` are deliberately outside `internal/`, so other
modules can import them.

## Files

| File | Role |
|---|---|
| `cmd/engtree/main.go` | calls `app.Run()`; holds `rsrc_windows_amd64.syso` |
| `app/main.go` | Product list, routes, folder reachability, path safety, window launch, auto exit |
| `app/config.go` | Settings (`engtree.json`), defaults, portable mode, shared settings file, modes |
| `app/sheets.go` | Reads .csv/.txt (delimiter and encoding detection) and dispatches .xlsx |
| `app/xlsx.go` | Minimal .xlsx reader (first sheet) |
| `app/bom.go` | Product discovery from file names, header matching, details join, tree building |
| `app/files.go` | Directory cache, file-type index (exact / prefix keys), attachments, ZIP packages |
| `app/parts.go` | Parts index built from all BOMs (per-BOM cache by modification time, read in parallel), Parts tab routes |
| `app/revisions.go` | Revision requests: one JSON file each in a shared folder, status, export |
| `app/materials.go` | The material list (sheets and bars, code and alternative code, thickness, grain, size, offcuts), kept in `materials.json` in the data folder |
| `app/cutlines.go` | The parts of a product that go to the cut plan: BOM lines with a material code, quantities multiplied down the tree, matched to the material list |
| `app/cutjobs.go` | Conference lines to one planner job per material: edge banding off, equal parts joined, rotation from the grain, offcuts first |
| `app/cutrun.go` | Runs the jobs of a plan one after another through the registered planners, verifies each result, remembers the machine values |
| `app/cutreport.go` | The A4 PDF report in the Corte Certo layout (one page per sheet layout, one per bar material) and the summary spreadsheet |
| `app/xlsxwrite.go` | Minimal .xlsx writer (one sheet), read back by `xlsx.go` |
| `app/cutroutes.go` | Cut plan API: `/api/materials`, `/api/cut_lines`, `/api/cut_machine`, `/api/cut_plan` (start, status, `stop`, `pdf`, `xlsx`) |
| `app/ui/cutplan.js` | The Cut plan tab: conference table, material list, machine values, results |
| `app/cad.go` | The viewer's IGES reader, and the DXF view built on `pkg/dxfgeom` |
| `app/taskbar_windows.go` | Own taskbar identity/icon for the browser window |
| `app/ui/index.html` | The whole interface |
| `pkg/geom/geom.go` | Point, Rect, Polygon (area, direction, containment, rotate), Contour |
| `pkg/dxfgeom/reader.go` | DXF entities to flat segments: bulges, blocks, splines, units |
| `pkg/dxfgeom/contours.go` | Segments joined into closed rings, outlines and holes told apart |
| `pkg/dxfgeom/curve.go` | The curves as drawn (line, arc, ellipse, spline), kept beside the flat segments |
| `extension/extension.go` | The `CutPlanner` interface and its registry |
| `pkg/cutplan/cutplan.go` | The adapter: registers the planners of the `cutplan` module (`linear`, `rectangular`) as `extension.CutPlanner` |

## The cutplan module

`cutplan/` has its own `go.mod` (`github.com/ALEXKAKIUCHI/engtree-viewer/cutplan`)
and depends only on the standard library: it knows nothing about the BOM,
the screen or `extension`. The main `go.mod` points at it with a `replace`.
Its tests run from inside the folder (`cd cutplan && go test ./...`).

| File | Role |
|---|---|
| `cutplan/cutplan.go` | Lists the planners (`Planners`, `ByID`); jobs, results and verification travel as JSON |
| `cutplan/linear/` | Bars and tubes: `types.go` (job, result), `verify.go` (the independent verifier), `plan.go` (the planner) |
| `cutplan/rect/` | Sheets on a panel saw, edge-to-edge cuts in 2 or 3 stages: same three files |
| `cutplan/giben/` | A rectangular plan as the .AC and .AD files of a Giben saw (GVision), as Corte Certo writes them; format in `cutplan/GIBEN.md` |
| `cutplan/cmd/cuttest/` | Local test page: type parts and stock, calculate, see the bars or sheets drawn and the cut list |

Each planner has a verifier written before it and sharing no code with it.
Every plan made in the tests, and every plan shown by `cmd/cuttest`, goes
through the verifier.

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

`app/assets/app_icon.ico` is embedded through `cmd/engtree/rsrc_windows_amd64.syso`. After changing the icon:

```
go run tools/makeicon/main.go app/assets/app_icon.ico
move rsrc_windows_amd64.syso cmd\engtree\
```
