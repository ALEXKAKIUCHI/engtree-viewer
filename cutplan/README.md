# cutplan

Cut planners in Go, standard library only. Everything is in millimetres.

- `linear`: parts of a given length cut from bars and tubes, with kerf,
  start and end losses, offcuts used first.
- `rect`: rectangular parts nested on sheets for a panel saw, edge-to-edge
  (guillotine) cuts in 2 or 3 stages, kerf and edge trim.
- `giben`: a `rect` plan written as the .AC/.AD files of a Giben saw
  (GVision). Format in [GIBEN.md](GIBEN.md).

Each planner has an independent verifier (`Verify`) that shares no code
with it. The root package lists the planners with JSON in and out
(`cutplan.Planners`, `cutplan.ByID`), for programs that do not want to
know their types.

```go
import "github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/rect"

res, err := rect.Run(ctx, job, func(better rect.Result) { /* progress */ })
problems := rect.Verify(job, res)
```

Try it with real cases in EngTree Corte, from the root of the repository:

```
go run ./cmd/engtreecorte
```

Tests: `go test ./...` from this folder.
