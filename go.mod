module github.com/ALEXKAKIUCHI/engtree-viewer

go 1.21

require github.com/ALEXKAKIUCHI/engtree-viewer/cutplan v0.0.0

// The cut planners live in their own module in ./cutplan.
replace github.com/ALEXKAKIUCHI/engtree-viewer/cutplan => ./cutplan
