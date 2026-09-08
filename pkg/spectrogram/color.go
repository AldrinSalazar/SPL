package spectrogram

import "image/color"

// DBToColor maps dBFS to RGB using a magma-like gradient.
// t=(dB-dBMin)/(dBMax-dBMin) clamped to [0,1].
func DBToColor(db, dbMin, dbMax float64) color.RGBA {
	t := 0.0
	if dbMax > dbMin {
		t = (db - dbMin) / (dbMax - dbMin)
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	// Magma-ish stops.
	stops := [][3]float64{
		{0, 0, 0},
		{60, 18, 103},
		{126, 33, 119},
		{188, 80, 91},
		{249, 158, 98},
		{252, 253, 191},
	}
	pos := t * float64(len(stops)-1)
	i := int(pos)
	if i >= len(stops)-1 {
		last := stops[len(stops)-1]
		return color.RGBA{uint8(last[0]), uint8(last[1]), uint8(last[2]), 255}
	}
	frac := pos - float64(i)
	r := stops[i][0] + (stops[i+1][0]-stops[i][0])*frac
	g := stops[i][1] + (stops[i+1][1]-stops[i][1])*frac
	b := stops[i][2] + (stops[i+1][2]-stops[i][2])*frac
	return color.RGBA{uint8(r + 0.5), uint8(g + 0.5), uint8(b + 0.5), 255}
}
