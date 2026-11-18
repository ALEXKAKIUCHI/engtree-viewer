# Giben panel saw files (.AC / .AD)

`giben` writes a rectangular plan as the two files a Giben saw
running GVision loads, in the layout Corte Certo exports for it. The layout
below was worked out from Corte Certo files; `giben/testdata`
holds two of them (product codes changed) and the tests rebuild them byte
for byte.

Both files: plain text, fixed columns, CRLF line ends, Latin-1. Same name,
only the extension changes. Lengths are in tenths of a millimetre.

## .AC — the cuts

One block per sheet layout. Every line starts with the material (15
characters) and the thickness (3 digits, tenths, space-padded: `150`, ` 60`).

Sheet line (37 characters):

| Columns | Example | Meaning |
|---|---|---|
| 18–19 | `01` | layout number |
| 20 | `1` | level 1: the sheet |
| 21–25 | `00003` | sheets cut with this layout |
| 26–30 | `18500` | sheet width |
| 31–35 | `27500` | sheet length |
| 36 | `0` | unknown, always 0 |

Cut line (30 characters):

| Columns | Example | Meaning |
|---|---|---|
| 18–19 | `01` | layout number |
| 20 | `3` | level |
| 21–22 | `08` | part code; `00` for a piece that is cut further |
| 23–27 | `17050` | size of the piece |
| 28–29 | `01` | how many equal pieces in a row |

(Columns count from 0.)

Levels go by direction: 2 and 4 are measured across the sheet width, 3 and
5 along its length. A layout whose first cuts go along the width starts at
level 3. The lines come in cutting order, each piece followed by the pieces
cut from it. Leftover is not written: it is whatever remains after the last
piece, so it must come last.

One rule taken from the files: at the third level from the top, the first
part of each group is written with code `00` and on a line of its own, even
when the next ones are the same part. Seen in every case in the samples;
not confirmed for layouts that start at level 3.

## .AD — parts and labels

Lines of 490 characters: material (15), thickness (4 digits, tenths,
zero-padded), then a record type in column 28. One block per layout, in the
.AC order:

- **1, header**: project code (columns 19–27), description (37–61, 25
  characters), date `2026nov17` (62–70).
- **2, a part** for each part line of the .AC, in the same order: code
  (29–32), width and length (33–38, 39–44), quantity (45–49), then `1`,
  ` 0 0`, the fixed text `02.732` at 124, the label (130–155), `0`, and the
  width and length again as `0200.00970.0` at 449.
- **4, the layouts**: every layout of the file, in every block: number
  (29–32), sheet width and length, sheets cut (45–49), `1`, the material
  cut to 9 characters at 60 and `0` at 125.

The fixed values (`0`, `1`, ` 0 0`, `02.732`) never changed in the samples
and are written as they are; their meaning is not known.

## Before the first real cut

1. Compare: export a job Corte Certo has also exported and compare the
   files.
2. Open the files in GVision without cutting and check them against the
   drawing.
3. Cut a scrap sheet first, with the operator watching.
