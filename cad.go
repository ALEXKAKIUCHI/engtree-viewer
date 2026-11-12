package main

import (
	"math"
	"os"
	"strconv"
	"strings"
)

type Segment2D struct {
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`
}

type DXFData struct {
	Segments []Segment2D `json:"segments"`
	Width    float64     `json:"width"`
	Height   float64     `json:"height"`
	MinX     float64     `json:"min_x"`
	MinY     float64     `json:"min_y"`
	MaxX     float64     `json:"max_x"`
	MaxY     float64     `json:"max_y"`
}

func parseDXF(path string) DXFData {
	var out DXFData
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64

	addSeg := func(x1, y1, x2, y2 float64) {
		out.Segments = append(out.Segments, Segment2D{X1: x1, Y1: y1, X2: x2, Y2: y2})
		for _, x := range []float64{x1, x2} {
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
		}
		for _, y := range []float64{y1, y2} {
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}

	inEntities := false
	i := 0
	for i+1 < len(lines) {
		code := strings.TrimSpace(lines[i])
		val := strings.TrimSpace(lines[i+1])
		i += 2

		if code == "2" && val == "ENTITIES" {
			inEntities = true
			continue
		}
		if code == "0" && val == "ENDSEC" && inEntities {
			break
		}
		if !inEntities || code != "0" {
			continue
		}

		entType := val
		var x1, y1, x2, y2, cx, cy, r, sa, ea float64
		var polyX, polyY []float64
		closed := false

		for i+1 < len(lines) {
			c := strings.TrimSpace(lines[i])
			if c == "0" {
				break
			}
			v := strings.TrimSpace(lines[i+1])
			i += 2
			num, _ := strconv.ParseFloat(v, 64)
			switch c {
			case "10":
				if entType == "LWPOLYLINE" {
					polyX = append(polyX, num)
				} else {
					x1, cx = num, num
				}
			case "20":
				if entType == "LWPOLYLINE" {
					polyY = append(polyY, num)
				} else {
					y1, cy = num, num
				}
			case "11":
				x2 = num
			case "21":
				y2 = num
			case "40":
				r = num
			case "50":
				sa = num
			case "51":
				ea = num
			case "70":
				if int(num)&1 == 1 {
					closed = true
				}
			}
		}

		switch entType {
		case "LINE":
			addSeg(x1, y1, x2, y2)
		case "LWPOLYLINE":
			n := len(polyX)
			if len(polyY) < n {
				n = len(polyY)
			}
			for k := 0; k+1 < n; k++ {
				addSeg(polyX[k], polyY[k], polyX[k+1], polyY[k+1])
			}
			if closed && n > 2 {
				addSeg(polyX[n-1], polyY[n-1], polyX[0], polyY[0])
			}
		case "CIRCLE":
			steps := 36
			for k := 0; k < steps; k++ {
				a1 := 2 * math.Pi * float64(k) / float64(steps)
				a2 := 2 * math.Pi * float64(k+1) / float64(steps)
				addSeg(cx+r*math.Cos(a1), cy+r*math.Sin(a1), cx+r*math.Cos(a2), cy+r*math.Sin(a2))
			}
		case "ARC":
			if ea < sa {
				ea += 360
			}
			steps := 24
			for k := 0; k < steps; k++ {
				a1 := (sa + (ea-sa)*float64(k)/float64(steps)) * math.Pi / 180
				a2 := (sa + (ea-sa)*float64(k+1)/float64(steps)) * math.Pi / 180
				addSeg(cx+r*math.Cos(a1), cy+r*math.Sin(a1), cx+r*math.Cos(a2), cy+r*math.Sin(a2))
			}
		}
	}

	if len(out.Segments) > 0 {
		out.MinX, out.MinY = minX, minY
		out.MaxX, out.MaxY = maxX, maxY
		out.Width = maxX - minX
		out.Height = maxY - minY
	}
	return out
}

type Segment3D struct {
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	Z1 float64 `json:"z1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`
	Z2 float64 `json:"z2"`
}

type Face3D struct {
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	Z1 float64 `json:"z1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`
	Z2 float64 `json:"z2"`
	X3 float64 `json:"x3"`
	Y3 float64 `json:"y3"`
	Z3 float64 `json:"z3"`
}

type IGESData struct {
	Segments []Segment3D `json:"segments"`
	Faces    []Face3D    `json:"faces"`
	DX       float64     `json:"dx"`
	DY       float64     `json:"dy"`
	DZ       float64     `json:"dz"`
}

type igesDirEntry struct {
	DE        int
	EntType   int
	MatrixDE  int
	Blanked   bool
	UseFlag   int // 5 = Parametric 2D (UV)
	ParamText string
	Fields    []string
}

func parseIGESFloat(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "D", "E")
	s = strings.ReplaceAll(s, "d", "e")
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func parseIGESInt(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

func applyIGESMatrix(pt [3]float64, mat *[12]float64) [3]float64 {
	if mat == nil {
		return pt
	}
	return [3]float64{
		mat[0]*pt[0] + mat[1]*pt[1] + mat[2]*pt[2] + mat[3],
		mat[4]*pt[0] + mat[5]*pt[1] + mat[6]*pt[2] + mat[7],
		mat[8]*pt[0] + mat[9]*pt[1] + mat[10]*pt[2] + mat[11],
	}
}

// Evaluates a point on a rational B-spline curve (NURBS, entity 126)
func evalNURBSCurve(u float64, k, m int, knots, weights []float64, ctrl [][3]float64) [3]float64 {
	var numX, numY, numZ, den float64
	for i := 0; i <= k; i++ {
		b := coxDeBoor(i, m, u, knots)
		if b == 0 {
			continue
		}
		w := 1.0
		if i < len(weights) && weights[i] > 0 {
			w = weights[i]
		}
		bw := b * w
		numX += bw * ctrl[i][0]
		numY += bw * ctrl[i][1]
		numZ += bw * ctrl[i][2]
		den += bw
	}
	if den == 0 {
		return ctrl[0]
	}
	return [3]float64{numX / den, numY / den, numZ / den}
}

func coxDeBoor(i, p int, u float64, knots []float64) float64 {
	if i < 0 || i+p+1 >= len(knots) {
		return 0
	}
	if p == 0 {
		if (u >= knots[i] && u < knots[i+1]) || (math.Abs(u-knots[len(knots)-1]) < 1e-9 && math.Abs(u-knots[i+1]) < 1e-9 && u >= knots[i]) {
			return 1.0
		}
		return 0.0
	}
	var left, right float64
	d1 := knots[i+p] - knots[i]
	if d1 > 1e-10 {
		left = (u - knots[i]) / d1 * coxDeBoor(i, p-1, u, knots)
	}
	d2 := knots[i+p+1] - knots[i+1]
	if d2 > 1e-10 {
		right = (knots[i+p+1] - u) / d2 * coxDeBoor(i+1, p-1, u, knots)
	}
	return left + right
}

func parseIGES(path string) IGESData {
	var out IGESData
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}

	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	var dLines []string
	pByDE := make(map[int]*strings.Builder)

	for _, line := range lines {
		if len(line) < 80 {
			continue
		}
		sec := line[72]
		switch sec {
		case 'D':
			dLines = append(dLines, line)
		case 'P':
			dePtr := parseIGESInt(line[64:72])
			if dePtr > 0 {
				b, ok := pByDE[dePtr]
				if !ok {
					b = &strings.Builder{}
					pByDE[dePtr] = b
				}
				b.WriteString(strings.TrimSpace(line[:64]))
			}
		}
	}

	entries := make(map[int]*igesDirEntry)
	for i := 0; i+1 < len(dLines); i += 2 {
		l1 := dLines[i]
		deNum := parseIGESInt(l1[73:80])
		entType := parseIGESInt(l1[0:8])
		matDE := parseIGESInt(l1[48:56])
		status := strings.TrimSpace(l1[64:72])
		for len(status) < 8 {
			status = "0" + status
		}
		blanked := status[0:2] == "01"
		useFlag := parseIGESInt(status[4:6])

		var pText string
		if b, ok := pByDE[deNum]; ok {
			pText = b.String()
			if idx := strings.Index(pText, ";"); idx >= 0 {
				pText = pText[:idx]
			}
		}
		entries[deNum] = &igesDirEntry{
			DE:        deNum,
			EntType:   entType,
			MatrixDE:  matDE,
			Blanked:   blanked,
			UseFlag:   useFlag,
			ParamText: pText,
			Fields:    strings.Split(pText, ","),
		}
	}

	// Transformation matrices (entity 124)
	matrices := make(map[int]*[12]float64)
	for de, e := range entries {
		if e.EntType == 124 && len(e.Fields) >= 13 {
			var m [12]float64
			for k := 0; k < 12; k++ {
				m[k], _ = parseIGESFloat(e.Fields[k+1])
			}
			matrices[de] = &m
		}
	}

	// Drop every parametric 2D (UV) curve to avoid ghost lines
	ignoreDE := make(map[int]bool)
	modelCurveDE := make(map[int]bool)

	for de, e := range entries {
		// Note: do NOT filter on the "Blanked" status. SolidWorks marks every
		// curve of the part as blanked (status 01xx) because they are
		// subordinate to faces; filtering on it leaves the drawing empty.
		if e.UseFlag == 5 || e.UseFlag == 1 || e.UseFlag == 2 {
			ignoreDE[de] = true
		}
		// Entity 142 (Curve on a Parametric Surface):
		// field 3 = BPTR (2D UV curve, produces stretched lines, ignored)
		// field 4 = CPTR (the real 3D curve of the part)
		if e.EntType == 142 && len(e.Fields) >= 5 {
			bptr := parseIGESInt(e.Fields[3])
			cptr := parseIGESInt(e.Fields[4])
			if bptr > 0 {
				ignoreDE[bptr] = true
			}
			if cptr > 0 {
				modelCurveDE[cptr] = true
			}
		}
	}

	// Propagate exclusion/inclusion to composite curves (entity 102)
	for pass := 0; pass < 3; pass++ {
		for de, e := range entries {
			if e.EntType == 102 && len(e.Fields) >= 3 {
				n := parseIGESInt(e.Fields[1])
				for k := 0; k < n && 2+k < len(e.Fields); k++ {
					childDE := parseIGESInt(e.Fields[2+k])
					if ignoreDE[de] {
						ignoreDE[childDE] = true
					}
					if modelCurveDE[de] && !ignoreDE[childDE] {
						modelCurveDE[childDE] = true
					}
				}
			}
		}
	}

	// Extract a 3D polyline from a single curve (110, 100, 126) or a composite one (102)
	var extractCurvePts func(de int, depth int) [][3]float64
	extractCurvePts = func(de int, depth int) [][3]float64 {
		if depth > 6 {
			return nil
		}
		e, ok := entries[de]
		if !ok || e == nil {
			return nil
		}
		mat := matrices[e.MatrixDE]

		switch e.EntType {
		case 110: // 3D line
			if len(e.Fields) >= 7 {
				x1, ok1 := parseIGESFloat(e.Fields[1])
				y1, ok2 := parseIGESFloat(e.Fields[2])
				z1, ok3 := parseIGESFloat(e.Fields[3])
				x2, ok4 := parseIGESFloat(e.Fields[4])
				y2, ok5 := parseIGESFloat(e.Fields[5])
				z2, ok6 := parseIGESFloat(e.Fields[6])
				if ok1 && ok2 && ok3 && ok4 && ok5 && ok6 {
					p1 := applyIGESMatrix([3]float64{x1, y1, z1}, mat)
					p2 := applyIGESMatrix([3]float64{x2, y2, z2}, mat)
					return [][3]float64{p1, p2}
				}
			}
		case 100: // circular arc
			if len(e.Fields) >= 8 {
				zt, _ := parseIGESFloat(e.Fields[1])
				cx, _ := parseIGESFloat(e.Fields[2])
				cy, _ := parseIGESFloat(e.Fields[3])
				x2, _ := parseIGESFloat(e.Fields[4])
				y2, _ := parseIGESFloat(e.Fields[5])
				x3, _ := parseIGESFloat(e.Fields[6])
				y3, _ := parseIGESFloat(e.Fields[7])
				r := math.Hypot(x2-cx, y2-cy)
				a1 := math.Atan2(y2-cy, x2-cx)
				a2 := math.Atan2(y3-cy, x3-cx)
				if a2 <= a1+1e-6 {
					a2 += 2 * math.Pi
				}
				steps := 16
				pts := make([][3]float64, 0, steps+1)
				for s := 0; s <= steps; s++ {
					ang := a1 + (a2-a1)*float64(s)/float64(steps)
					pt := applyIGESMatrix([3]float64{cx + r*math.Cos(ang), cy + r*math.Sin(ang), zt}, mat)
					pts = append(pts, pt)
				}
				return pts
			}
		case 126: // rational B-spline (NURBS)
			if len(e.Fields) >= 14 {
				k := parseIGESInt(e.Fields[1])
				m := parseIGESInt(e.Fields[2])
				if k >= 1 && m >= 1 {
					n := 1 + k - m
					numKnots := n + 2*m + 1
					numWeights := k + 1
					ctrlStart := 7 + numKnots + numWeights
					if ctrlStart+3*(k+1) <= len(e.Fields) {
						knots := make([]float64, numKnots)
						for idx := 0; idx < numKnots; idx++ {
							knots[idx], _ = parseIGESFloat(e.Fields[7+idx])
						}
						weights := make([]float64, numWeights)
						for idx := 0; idx < numWeights; idx++ {
							weights[idx], _ = parseIGESFloat(e.Fields[7+numKnots+idx])
						}
						ctrl := make([][3]float64, k+1)
						for idx := 0; idx <= k; idx++ {
							base := ctrlStart + idx*3
							px, _ := parseIGESFloat(e.Fields[base])
							py, _ := parseIGESFloat(e.Fields[base+1])
							pz, _ := parseIGESFloat(e.Fields[base+2])
							ctrl[idx] = applyIGESMatrix([3]float64{px, py, pz}, mat)
						}
						if k == 1 || m == 1 {
							return ctrl
						}
						u0, u1 := knots[0], knots[len(knots)-1]
						if u1 > u0 {
							steps := 14
							pts := make([][3]float64, 0, steps+1)
							for s := 0; s <= steps; s++ {
								u := u0 + (u1-u0)*float64(s)/float64(steps)
								pts = append(pts, evalNURBSCurve(u, k, m, knots, weights, ctrl))
							}
							return pts
						}
						return ctrl
					}
				}
			}
		case 102: // composite curve (chains sub-curves in order)
			if len(e.Fields) >= 3 {
				n := parseIGESInt(e.Fields[1])
				var chains [][][3]float64
				for idx := 0; idx < n && 2+idx < len(e.Fields); idx++ {
					cDE := parseIGESInt(e.Fields[2+idx])
					pts := extractCurvePts(cDE, depth+1)
					if len(pts) >= 2 {
						chains = append(chains, pts)
					}
				}
				return stitchChains(chains)
			}
		}
		return nil
	}

	minX, minY, minZ := math.MaxFloat64, math.MaxFloat64, math.MaxFloat64
	maxX, maxY, maxZ := -math.MaxFloat64, -math.MaxFloat64, -math.MaxFloat64

	updateBounds := func(p [3]float64) {
		if p[0] < minX {
			minX = p[0]
		}
		if p[0] > maxX {
			maxX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
		if p[2] < minZ {
			minZ = p[2]
		}
		if p[2] > maxZ {
			maxZ = p[2]
		}
	}

	addPolyline := func(pts [][3]float64) {
		for i := 0; i+1 < len(pts); i++ {
			p1, p2 := pts[i], pts[i+1]
			if math.Hypot(math.Hypot(p2[0]-p1[0], p2[1]-p1[1]), p2[2]-p1[2]) < 1e-5 {
				continue
			}
			out.Segments = append(out.Segments, Segment3D{
				X1: p1[0], Y1: p1[1], Z1: p1[2],
				X2: p2[0], Y2: p2[1], Z2: p2[2],
			})
			updateBounds(p1)
			updateBounds(p2)
		}
	}

	hasModelCurves := len(modelCurveDE) > 0

	// 1. Extract only the real 3D curves of the model (no parametric UV curves)
	for de, e := range entries {
		if ignoreDE[de] {
			continue
		}
		if e.EntType == 110 || e.EntType == 100 || e.EntType == 126 {
			if hasModelCurves && !modelCurveDE[de] {
				continue
			}
			pts := extractCurvePts(de, 0)
			addPolyline(pts)
		}
	}

	// Fallback for IGES files without entity 142
	if len(out.Segments) == 0 {
		for de, e := range entries {
			if ignoreDE[de] {
				continue
			}
			if e.EntType == 110 || e.EntType == 100 || e.EntType == 126 {
				addPolyline(extractCurvePts(de, 0))
			}
		}
	}

	// 2. Build shaded faces from trimmed surfaces (entity 144)
	for _, e := range entries {
		if e.EntType == 144 && len(e.Fields) >= 5 {
			ptoDE := parseIGESInt(e.Fields[4])
			nHoles := parseIGESInt(e.Fields[3])
			ptoEntry, ok := entries[ptoDE]
			if !ok || ptoEntry.EntType != 142 || len(ptoEntry.Fields) < 5 {
				continue
			}
			outerCurveDE := parseIGESInt(ptoEntry.Fields[4])
			outerPts := extractCurvePts(outerCurveDE, 0)
			if len(outerPts) < 3 {
				continue
			}
			var holes [][][3]float64
			for h := 0; h < nHoles && 5+h < len(e.Fields); h++ {
				ptiDE := parseIGESInt(e.Fields[5+h])
				if ptiEntry, ok := entries[ptiDE]; ok && ptiEntry.EntType == 142 && len(ptiEntry.Fields) >= 5 {
					hCurveDE := parseIGESInt(ptiEntry.Fields[4])
					hPts := extractCurvePts(hCurveDE, 0)
					if len(hPts) >= 3 {
						holes = append(holes, hPts)
					}
				}
			}
			tris := triangulateFace3D(outerPts, holes)
			for _, t := range tris {
				updateBounds([3]float64{t.X1, t.Y1, t.Z1})
				updateBounds([3]float64{t.X2, t.Y2, t.Z2})
				updateBounds([3]float64{t.X3, t.Y3, t.Z3})
			}
			out.Faces = append(out.Faces, tris...)
		}
	}

	// 3. Center the part at (0, 0, 0) for balanced rotation and compute dimensions
	if len(out.Segments) > 0 || len(out.Faces) > 0 {
		out.DX = maxX - minX
		out.DY = maxY - minY
		out.DZ = maxZ - minZ
		cx := (minX + maxX) / 2.0
		cy := (minY + maxY) / 2.0
		cz := (minZ + maxZ) / 2.0

		for i := range out.Segments {
			out.Segments[i].X1 -= cx
			out.Segments[i].Y1 -= cy
			out.Segments[i].Z1 -= cz
			out.Segments[i].X2 -= cx
			out.Segments[i].Y2 -= cy
			out.Segments[i].Z2 -= cz
		}
		for i := range out.Faces {
			out.Faces[i].X1 -= cx
			out.Faces[i].Y1 -= cy
			out.Faces[i].Z1 -= cz
			out.Faces[i].X2 -= cx
			out.Faces[i].Y2 -= cy
			out.Faces[i].Z2 -= cz
			out.Faces[i].X3 -= cx
			out.Faces[i].Y3 -= cy
			out.Faces[i].Z3 -= cz
		}
	}

	return out
}

func dist3(a, b [3]float64) float64 {
	return math.Hypot(math.Hypot(a[0]-b[0], a[1]-b[1]), a[2]-b[2])
}

func reversePts(pts [][3]float64) [][3]float64 {
	out := make([][3]float64, len(pts))
	for i := range pts {
		out[i] = pts[len(pts)-1-i]
	}
	return out
}

// Joins the sub-curves of an entity 102 into one continuous loop
func stitchChains(chains [][][3]float64) [][3]float64 {
	if len(chains) == 0 {
		return nil
	}
	used := make([]bool, len(chains))
	res := append([][3]float64{}, chains[0]...)
	used[0] = true

	for step := 1; step < len(chains); step++ {
		last := res[len(res)-1]
		bestIdx := -1
		bestRev := false
		bestD := math.MaxFloat64

		for i, ch := range chains {
			if used[i] || len(ch) == 0 {
				continue
			}
			dStart := dist3(last, ch[0])
			dEnd := dist3(last, ch[len(ch)-1])
			if dStart < bestD {
				bestD = dStart
				bestIdx = i
				bestRev = false
			}
			if dEnd < bestD {
				bestD = dEnd
				bestIdx = i
				bestRev = true
			}
		}
		if bestIdx < 0 {
			break
		}
		used[bestIdx] = true
		next := chains[bestIdx]
		if bestRev {
			next = reversePts(next)
		}
		if dist3(last, next[0]) < 1e-3 && len(next) > 1 {
			res = append(res, next[1:]...)
		} else {
			res = append(res, next...)
		}
	}
	return res
}

type pt2D struct {
	U  float64
	V  float64
	P3 [3]float64
}

func cleanLoop(pts [][3]float64) [][3]float64 {
	var out [][3]float64
	for _, p := range pts {
		if len(out) == 0 || dist3(out[len(out)-1], p) > 1e-3 {
			out = append(out, p)
		}
	}
	if len(out) > 2 && dist3(out[0], out[len(out)-1]) <= 1e-3 {
		out = out[:len(out)-1]
	}
	return out
}

func signedArea2D(poly []pt2D) float64 {
	var a float64
	n := len(poly)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		a += poly[i].U*poly[j].V - poly[j].U*poly[i].V
	}
	return a * 0.5
}

func reverse2D(poly []pt2D) {
	for i, j := 0, len(poly)-1; i < j; i, j = i+1, j-1 {
		poly[i], poly[j] = poly[j], poly[i]
	}
}

func ptInTri2D(p, a, b, c pt2D) bool {
	c1 := (b.U-a.U)*(p.V-a.V) - (b.V-a.V)*(p.U-a.U)
	c2 := (c.U-b.U)*(p.V-b.V) - (c.V-b.V)*(p.U-b.U)
	c3 := (a.U-c.U)*(p.V-c.V) - (a.V-c.V)*(p.U-c.U)
	hasNeg := (c1 < -1e-7) || (c2 < -1e-7) || (c3 < -1e-7)
	hasPos := (c1 > 1e-7) || (c2 > 1e-7) || (c3 > 1e-7)
	return !(hasNeg && hasPos)
}

// Triangulates a trimmed 3D face (holes supported) by 2D projection and ear clipping
func triangulateFace3D(outer [][3]float64, holes [][][3]float64) []Face3D {
	outer = cleanLoop(outer)
	if len(outer) < 3 || len(outer) > 250 {
		return nil
	}

	// Face normal by Newell's method
	var nx, ny, nz float64
	for i := 0; i < len(outer); i++ {
		curr := outer[i]
		next := outer[(i+1)%len(outer)]
		nx += (curr[1] - next[1]) * (curr[2] + next[2])
		ny += (curr[2] - next[2]) * (curr[0] + next[0])
		nz += (curr[0] - next[0]) * (curr[1] + next[1])
	}
	ax, ay, az := math.Abs(nx), math.Abs(ny), math.Abs(nz)
	if ax < 1e-7 && ay < 1e-7 && az < 1e-7 {
		return nil
	}

	project := func(p [3]float64) pt2D {
		if az >= ax && az >= ay {
			return pt2D{U: p[0], V: p[1], P3: p}
		} else if ay >= ax {
			return pt2D{U: p[0], V: p[2], P3: p}
		}
		return pt2D{U: p[1], V: p[2], P3: p}
	}

	poly := make([]pt2D, len(outer))
	for i, p := range outer {
		poly[i] = project(p)
	}
	if signedArea2D(poly) < 0 {
		reverse2D(poly)
	}

	// Bridge inner holes to the outer loop
	for _, hRaw := range holes {
		hClean := cleanLoop(hRaw)
		if len(hClean) < 3 {
			continue
		}
		hPoly := make([]pt2D, len(hClean))
		for i, p := range hClean {
			hPoly[i] = project(p)
		}
		if signedArea2D(hPoly) > 0 {
			reverse2D(hPoly)
		}

		// Rightmost point of the hole
		maxIdx := 0
		for i := 1; i < len(hPoly); i++ {
			if hPoly[i].U > hPoly[maxIdx].U {
				maxIdx = i
			}
		}
		hp := hPoly[maxIdx]

		// Closest outer vertex
		bestOut := 0
		bestDist := math.MaxFloat64
		for i, op := range poly {
			d := math.Hypot(op.U-hp.U, op.V-hp.V)
			if d < bestDist {
				bestDist = d
				bestOut = i
			}
		}

		var merged []pt2D
		merged = append(merged, poly[:bestOut+1]...)
		for i := 0; i < len(hPoly); i++ {
			merged = append(merged, hPoly[(maxIdx+i)%len(hPoly)])
		}
		merged = append(merged, hPoly[maxIdx])
		merged = append(merged, poly[bestOut:]...)
		poly = merged
	}

	var tris []Face3D
	guard := len(poly) * 3
	for len(poly) >= 3 && guard > 0 {
		guard--
		n := len(poly)
		earFound := false
		for i := 0; i < n; i++ {
			prev := poly[(i+n-1)%n]
			curr := poly[i]
			next := poly[(i+1)%n]

			cross := (curr.U-prev.U)*(next.V-curr.V) - (curr.V-prev.V)*(next.U-curr.U)
			if cross <= 1e-7 {
				continue
			}

			hasInside := false
			for j := 0; j < n; j++ {
				if j == (i+n-1)%n || j == i || j == (i+1)%n {
					continue
				}
				if dist3(poly[j].P3, prev.P3) < 1e-4 || dist3(poly[j].P3, curr.P3) < 1e-4 || dist3(poly[j].P3, next.P3) < 1e-4 {
					continue
				}
				if ptInTri2D(poly[j], prev, curr, next) {
					hasInside = true
					break
				}
			}
			if !hasInside {
				tris = append(tris, Face3D{
					X1: prev.P3[0], Y1: prev.P3[1], Z1: prev.P3[2],
					X2: curr.P3[0], Y2: curr.P3[1], Z2: curr.P3[2],
					X3: next.P3[0], Y3: next.P3[1], Z3: next.P3[2],
				})
				poly = append(poly[:i], poly[i+1:]...)
				earFound = true
				break
			}
		}
		if !earFound {
			// Simple fallback on numeric degeneracy
			for i := 1; i+1 < len(poly); i++ {
				tris = append(tris, Face3D{
					X1: poly[0].P3[0], Y1: poly[0].P3[1], Z1: poly[0].P3[2],
					X2: poly[i].P3[0], Y2: poly[i].P3[1], Z2: poly[i].P3[2],
					X3: poly[i+1].P3[0], Y3: poly[i+1].P3[1], Z3: poly[i+1].P3[2],
				})
			}
			break
		}
	}
	return tris
}
