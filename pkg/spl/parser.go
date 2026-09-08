package spl

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	intPattern    = regexp.MustCompile(`^[0-9]+$`)
	numberPattern = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)
)

// logicalLine is a nonempty line after comment stripping.
type logicalLine struct {
	Num    int // physical line number (1-based)
	Raw    string
	Clean  string
	Tokens []string
	Cols   []int // 1-based byte column of each token in Clean
}

func splitLines(input []byte) []string {
	s := string(input)
	parts := strings.Split(s, "\n")
	for i, p := range parts {
		if strings.HasSuffix(p, "\r") {
			parts[i] = strings.TrimSuffix(p, "\r")
		}
	}
	if len(parts) > 0 && strings.HasSuffix(s, "\n") {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func tokenize(clean string) ([]string, []int) {
	var toks []string
	var cols []int
	i := 0
	for i < len(clean) {
		c := clean[i]
		if c == ' ' || c == '\t' {
			i++
			continue
		}
		start := i
		for i < len(clean) && clean[i] != ' ' && clean[i] != '\t' {
			i++
		}
		toks = append(toks, clean[start:i])
		cols = append(cols, start+1)
	}
	return toks, cols
}

// Parse parses and fully validates SPL source.
func Parse(input []byte) (*Document, []Diagnostic) {
	return ParseWithLimits(input, DefaultLimits())
}

// ParseWithLimits parses with explicit resource limits (e.g. BrowserLimits).
func ParseWithLimits(input []byte, lim Limits) (*Document, []Diagnostic) {
	var diags []Diagnostic
	if len(input) > lim.MaxInputBytes {
		diags = append(diags, Diagnostic{Code: CodeResourceLimit,
			Message: fmt.Sprintf("input size %d bytes exceeds limit %d bytes", len(input), lim.MaxInputBytes),
			Line:    0, Field: "INPUT"})
		return nil, diags
	}
	phys := splitLines(input)
	for idx, raw := range phys {
		if strings.Contains(raw, "```") {
			diags = append(diags, Diagnostic{Code: CodeMarkdownFence,
				Message: "markdown fences are invalid; provide raw SPL text only",
				Line:    idx + 1})
		}
	}
	if len(diags) > 0 {
		return nil, diags
	}
	var logical []logicalLine
	for idx, raw := range phys {
		clean := raw
		if h := strings.Index(clean, "#"); h >= 0 {
			clean = clean[:h]
		}
		if strings.Trim(clean, " \t") == "" {
			continue
		}
		toks, cols := tokenize(clean)
		if len(toks) == 0 {
			continue
		}
		logical = append(logical, logicalLine{Num: idx + 1, Raw: raw, Clean: clean, Tokens: toks, Cols: cols})
	}
	if len(logical) == 0 {
		diags = append(diags, Diagnostic{Code: CodeHeaderMissing, Message: "missing header line `spl 2 RATE DURATION SEED`", Line: 0})
		return nil, diags
	}
	hdr := logical[0]
	headerRate, headerDurTok, headerDur, headerSeed, hdrDiags := parseHeader(hdr, lim)
	diags = append(diags, hdrDiags...)
	if len(hdrDiags) > 0 {
		return nil, diags
	}
	nyquist := float64(headerRate) / 2
	n, sdiag := SampleCount(headerRate, headerDurTok, lim)
	if sdiag != nil {
		sdiag.Line = hdr.Num
		diags = append(diags, *sdiag)
		return nil, diags
	}
	if n <= 0 {
		diags = append(diags, Diagnostic{Code: CodeDurationRange, Field: "DURATION",
			Message: fmt.Sprintf("DURATION %s must be positive", headerDurTok),
			Line:    hdr.Num, Column: colOf(hdr, 3)})
		return nil, diags
	}
	doc := &Document{Rate: headerRate, DurationToken: headerDurTok, Duration: headerDur,
		Seed: headerSeed, NumSamples: n, HeaderLine: hdr.Num}
	totalRows := 0
	noiseIndex := 0
	i := 1
	for i < len(logical) {
		ln := logical[i]
		kw := ln.Tokens[0]
		switch kw {
		case "track", "harmonics", "noise", "hit":
			if len(doc.Blocks) >= lim.MaxBlocks {
				diags = append(diags, Diagnostic{Code: CodeResourceLimit,
					Message:   fmt.Sprintf("block count exceeds limit %d", lim.MaxBlocks),
					Line:      ln.Num, BlockLine: ln.Num})
				i++
				continue
			}
			switch kw {
			case "track":
				b, next, d := parseTrack(logical, i, lim, &totalRows)
				diags = append(diags, d...)
				if b != nil {
					doc.Blocks = append(doc.Blocks, b)
				}
				i = next
			case "harmonics":
				b, next, d := parseHarmonics(logical, i, lim, &totalRows)
				diags = append(diags, d...)
				if b != nil {
					doc.Blocks = append(doc.Blocks, b)
				}
				i = next
			case "noise":
				b, next, d := parseNoise(logical, i, lim, &totalRows, noiseIndex)
				diags = append(diags, d...)
				if b != nil {
					doc.Blocks = append(doc.Blocks, b)
					noiseIndex++
				}
				i = next
			case "hit":
				b, next, d := parseHit(logical, i, lim, &totalRows)
				diags = append(diags, d...)
				if b != nil {
					doc.Blocks = append(doc.Blocks, b)
				}
				i = next
			}
		case "end":
			diags = append(diags, Diagnostic{Code: CodeUnexpectedEnd,
				Message: "unexpected `end` without an open block", Line: ln.Num, Column: ln.Cols[0]})
			i++
		case "spectrum", "curve":
			diags = append(diags, Diagnostic{Code: CodeUnexpectedSect,
				Message: fmt.Sprintf("`%s` outside a harmonics block", kw), Line: ln.Num, Column: ln.Cols[0]})
			i++
		default:
			diags = append(diags, Diagnostic{Code: CodeUnknownBlock,
				Message: fmt.Sprintf("unknown command %q; expected track, harmonics, noise, hit, or end", kw),
				Line: ln.Num, Column: ln.Cols[0]})
			i++
		}
	}
	if totalRows > lim.MaxTotalRows {
		diags = append(diags, Diagnostic{Code: CodeResourceLimit,
			Message: fmt.Sprintf("total row count %d exceeds limit %d", totalRows, lim.MaxTotalRows), Line: 0})
	}
	sem := validateSemantics(doc, nyquist)
	diags = append(diags, sem...)
	if len(diags) > 0 {
		return nil, diags
	}
	if pd := Preflight(doc, lim); pd != nil {
		return nil, []Diagnostic{*pd}
	}
	return doc, nil
}

// Validate reports diagnostics for source without returning a document.
func Validate(input []byte, lim Limits) []Diagnostic {
	_, diags := ParseWithLimits(input, lim)
	return diags
}

func colOf(ln logicalLine, tokIdx int) int {
	if tokIdx >= 0 && tokIdx < len(ln.Cols) {
		return ln.Cols[tokIdx]
	}
	return 0
}

func checkExponentBound(tok string, lim Limits) *Diagnostic {
	i := strings.IndexAny(tok, "eE")
	if i < 0 {
		return nil
	}
	es := tok[i+1:]
	if es == "" {
		return &Diagnostic{Code: CodeNumberFormat, Message: fmt.Sprintf("malformed exponent in %q", tok)}
	}
	digits := strings.TrimPrefix(strings.TrimPrefix(es, "+"), "-")
	if len(digits) > 7 {
		return &Diagnostic{Code: CodeResourceLimit,
			Message: fmt.Sprintf("exponent in %q exceeds limit ±%d", tok, lim.MaxExponentAbs)}
	}
	v := 0
	for _, c := range digits {
		if c < '0' || c > '9' {
			return &Diagnostic{Code: CodeNumberFormat, Message: fmt.Sprintf("malformed exponent in %q", tok)}
		}
		v = v*10 + int(c-'0')
	}
	if strings.HasPrefix(es, "-") {
		v = -v
	}
	if v > lim.MaxExponentAbs || v < -lim.MaxExponentAbs {
		return &Diagnostic{Code: CodeResourceLimit,
			Message: fmt.Sprintf("exponent %d in %q exceeds limit ±%d", v, tok, lim.MaxExponentAbs)}
	}
	return nil
}

func parseHeader(hdr logicalLine, lim Limits) (rate int, durTok string, dur float64, seed uint32, diags []Diagnostic) {
	t := hdr.Tokens
	if len(t) != 5 {
		diags = append(diags, Diagnostic{Code: CodeHeaderFieldCount,
			Message: fmt.Sprintf("header requires `spl 2 RATE DURATION SEED`; found %d fields", len(t)),
			Line: hdr.Num, Column: colOf(hdr, 0), Field: "HEADER"})
		return
	}
	if t[0] != "spl" {
		diags = append(diags, Diagnostic{Code: CodeHeaderKeyword,
			Message: fmt.Sprintf("header must start with `spl`; found %q", t[0]),
			Line: hdr.Num, Column: colOf(hdr, 0), Field: "HEADER"})
	}
	if t[1] != "2" {
		diags = append(diags, Diagnostic{Code: CodeHeaderVersion,
			Message: fmt.Sprintf("header version must be `2`; found %q", t[1]),
			Line: hdr.Num, Column: colOf(hdr, 1), Field: "HEADER"})
	}
	rateTok := t[2]
	rateOK := false
	if len(rateTok) > lim.MaxTokenLen {
		diags = append(diags, Diagnostic{Code: CodeResourceLimit, Field: "RATE",
			Message: fmt.Sprintf("RATE token length %d exceeds limit %d", len(rateTok), lim.MaxTokenLen),
			Line: hdr.Num, Column: colOf(hdr, 2)})
	} else if !intPattern.MatchString(rateTok) {
		diags = append(diags, Diagnostic{Code: CodeRateFormat, Field: "RATE",
			Message: fmt.Sprintf("RATE %q must match [0-9]+", rateTok),
			Line: hdr.Num, Column: colOf(hdr, 2)})
	} else {
		v, err := strconv.Atoi(rateTok)
		if err != nil {
			diags = append(diags, Diagnostic{Code: CodeRateRange, Field: "RATE",
				Message: fmt.Sprintf("RATE %q out of range 8000 through 192000", rateTok),
				Line: hdr.Num, Column: colOf(hdr, 2)})
		} else if v < 8000 || v > 192000 {
			diags = append(diags, Diagnostic{Code: CodeRateRange, Field: "RATE",
				Message: fmt.Sprintf("RATE %d out of range 8000 through 192000", v),
				Line: hdr.Num, Column: colOf(hdr, 2)})
		} else {
			rate = v
			rateOK = true
		}
	}
	dtok := t[3]
	durOK := false
	if len(dtok) > lim.MaxTokenLen {
		diags = append(diags, Diagnostic{Code: CodeResourceLimit, Field: "DURATION",
			Message: fmt.Sprintf("DURATION token length %d exceeds limit %d", len(dtok), lim.MaxTokenLen),
			Line: hdr.Num, Column: colOf(hdr, 3)})
	} else if !numberPattern.MatchString(dtok) {
		diags = append(diags, Diagnostic{Code: CodeDurationFormat, Field: "DURATION",
			Message: fmt.Sprintf("DURATION %q must match -?[0-9]+(\\.[0-9]+)?([eE][+-]?[0-9]+)?", dtok),
			Line: hdr.Num, Column: colOf(hdr, 3)})
	} else if ed := checkExponentBound(dtok, lim); ed != nil {
		ed.Line = hdr.Num
		ed.Column = colOf(hdr, 3)
		ed.Field = "DURATION"
		diags = append(diags, *ed)
	} else {
		f, err := strconv.ParseFloat(dtok, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			diags = append(diags, Diagnostic{Code: CodeNonfiniteNumber, Field: "DURATION",
				Message: fmt.Sprintf("DURATION %q must parse to a finite binary64 value", dtok),
				Line: hdr.Num, Column: colOf(hdr, 3)})
		} else if f <= 0 {
			diags = append(diags, Diagnostic{Code: CodeDurationRange, Field: "DURATION",
				Message: fmt.Sprintf("DURATION %s must be positive", dtok),
				Line: hdr.Num, Column: colOf(hdr, 3)})
		} else {
			dur = f
			durTok = dtok
			durOK = true
		}
	}
	stok := t[4]
	seedOK := false
	if len(stok) > lim.MaxTokenLen {
		diags = append(diags, Diagnostic{Code: CodeResourceLimit, Field: "SEED",
			Message: fmt.Sprintf("SEED token length %d exceeds limit %d", len(stok), lim.MaxTokenLen),
			Line: hdr.Num, Column: colOf(hdr, 4)})
	} else if !intPattern.MatchString(stok) {
		diags = append(diags, Diagnostic{Code: CodeSeedFormat, Field: "SEED",
			Message: fmt.Sprintf("SEED %q must match [0-9]+", stok),
			Line: hdr.Num, Column: colOf(hdr, 4)})
	} else {
		v, err := strconv.ParseUint(stok, 10, 32)
		if err != nil {
			diags = append(diags, Diagnostic{Code: CodeSeedRange, Field: "SEED",
				Message: fmt.Sprintf("SEED %q out of range 0 through 4294967295", stok),
				Line: hdr.Num, Column: colOf(hdr, 4)})
		} else {
			seed = uint32(v)
			seedOK = true
		}
	}
	if !(rateOK && durOK && seedOK) && len(diags) == 0 {
		diags = append(diags, Diagnostic{Code: CodeHeaderFieldCount, Field: "HEADER",
			Message: "invalid header", Line: hdr.Num})
	}
	return rate, durTok, dur, seed, diags
}

// parseNumbers parses exactly want general numbers from a row line.
func parseNumbers(ln logicalLine, want int, what string, lim Limits, blockOpen int) ([]float64, []Diagnostic) {
	var diags []Diagnostic
	t := ln.Tokens
	if len(t) != want {
		names := what
		diags = append(diags, Diagnostic{Code: CodeFieldCount,
			Message:   fmt.Sprintf("%s requires %d numbers; found %d", names, want, len(t)),
			Line:      ln.Num, Column: colOf(ln, 0), Field: "FIELD_COUNT", BlockLine: blockOpen})
		return nil, diags
	}
	vals := make([]float64, want)
	for i, tok := range t {
		if len(tok) > lim.MaxTokenLen {
			diags = append(diags, Diagnostic{Code: CodeResourceLimit,
				Message: fmt.Sprintf("token length %d exceeds limit %d", len(tok), lim.MaxTokenLen),
				Line: ln.Num, Column: ln.Cols[i], BlockLine: blockOpen})
			return nil, diags
		}
		if !numberPattern.MatchString(tok) {
			diags = append(diags, Diagnostic{Code: CodeNumberFormat,
				Message: fmt.Sprintf("malformed number %q; expected -?[0-9]+(\\.[0-9]+)?([eE][+-]?[0-9]+)?", tok),
				Line: ln.Num, Column: ln.Cols[i], BlockLine: blockOpen})
			return nil, diags
		}
		if ed := checkExponentBound(tok, lim); ed != nil {
			ed.Line = ln.Num
			ed.Column = ln.Cols[i]
			ed.BlockLine = blockOpen
			diags = append(diags, *ed)
			return nil, diags
		}
		f, err := strconv.ParseFloat(tok, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			diags = append(diags, Diagnostic{Code: CodeNonfiniteNumber,
				Message: fmt.Sprintf("number %q must parse to a finite binary64 value", tok),
				Line: ln.Num, Column: ln.Cols[i], BlockLine: blockOpen})
			return nil, diags
		}
		vals[i] = f
	}
	return vals, nil
}

func isBlockStart(kw string) bool {
	return kw == "track" || kw == "harmonics" || kw == "noise" || kw == "hit"
}

func parseTrack(logical []logicalLine, start int, lim Limits, totalRows *int) (*TrackBlock, int, []Diagnostic) {
	var diags []Diagnostic
	open := logical[start]
	if len(open.Tokens) != 1 {
		diags = append(diags, Diagnostic{Code: CodeFieldCount,
			Message: fmt.Sprintf("`track` takes no fields; found %d", len(open.Tokens)-1),
			Line: open.Num, Column: colOf(open, 1), BlockLine: open.Num})
	}
	b := &TrackBlock{Open: open.Num}
	i := start + 1
	rows := 0
	closed := false
	for i < len(logical) {
		ln := logical[i]
		kw := ln.Tokens[0]
		if kw == "end" {
			if len(ln.Tokens) != 1 {
				diags = append(diags, Diagnostic{Code: CodeFieldCount,
					Message: fmt.Sprintf("`end` takes no fields; found %d", len(ln.Tokens)-1),
					Line: ln.Num, Column: colOf(ln, 1), BlockLine: open.Num})
			}
			b.Close = ln.Num
			closed = true
			i++
			break
		}
		if isBlockStart(kw) {
			break
		}
		if kw == "spectrum" || kw == "curve" {
			diags = append(diags, Diagnostic{Code: CodeUnexpectedSect,
				Message: fmt.Sprintf("`%s` inside a track block", kw), Line: ln.Num, Column: ln.Cols[0], BlockLine: open.Num})
			i++
			continue
		}
		rows++
		*totalRows++
		if rows > lim.MaxRowsPerBlock {
			diags = append(diags, Diagnostic{Code: CodeResourceLimit,
				Message: fmt.Sprintf("row count exceeds per-block limit %d", lim.MaxRowsPerBlock),
				Line: ln.Num, BlockLine: open.Num})
			i++
			continue
		}
		vals, d := parseNumbers(ln, 3, "track rows require TIME FREQUENCY GAIN", lim, open.Num)
		if len(d) > 0 {
			diags = append(diags, d...)
			i++
			continue
		}
		b.Rows = append(b.Rows, TrajRow{Line: ln.Num, Time: vals[0], Freq: vals[1], Gain: vals[2]})
		i++
	}
	if !closed {
		diags = append(diags, Diagnostic{Code: CodeUnclosedBlock,
			Message: fmt.Sprintf("expected end for track opened on line %d", open.Num),
			Line: open.Num, BlockLine: open.Num})
		b.Close = open.Num
	}
	return b, i, diags
}

func parseNoise(logical []logicalLine, start int, lim Limits, totalRows *int, index int) (*NoiseBlock, int, []Diagnostic) {
	var diags []Diagnostic
	open := logical[start]
	if len(open.Tokens) != 1 {
		diags = append(diags, Diagnostic{Code: CodeFieldCount,
			Message: fmt.Sprintf("`noise` takes no fields; found %d", len(open.Tokens)-1),
			Line: open.Num, Column: colOf(open, 1), BlockLine: open.Num})
	}
	b := &NoiseBlock{Open: open.Num, Index: index}
	i := start + 1
	rows := 0
	closed := false
	for i < len(logical) {
		ln := logical[i]
		kw := ln.Tokens[0]
		if kw == "end" {
			if len(ln.Tokens) != 1 {
				diags = append(diags, Diagnostic{Code: CodeFieldCount,
					Message: fmt.Sprintf("`end` takes no fields; found %d", len(ln.Tokens)-1),
					Line: ln.Num, Column: colOf(ln, 1), BlockLine: open.Num})
			}
			b.Close = ln.Num
			closed = true
			i++
			break
		}
		if isBlockStart(kw) {
			break
		}
		if kw == "spectrum" || kw == "curve" {
			diags = append(diags, Diagnostic{Code: CodeUnexpectedSect,
				Message: fmt.Sprintf("`%s` inside a noise block", kw), Line: ln.Num, Column: ln.Cols[0], BlockLine: open.Num})
			i++
			continue
		}
		rows++
		*totalRows++
		if rows > lim.MaxRowsPerBlock {
			diags = append(diags, Diagnostic{Code: CodeResourceLimit,
				Message: fmt.Sprintf("row count exceeds per-block limit %d", lim.MaxRowsPerBlock),
				Line: ln.Num, BlockLine: open.Num})
			i++
			continue
		}
		vals, d := parseNumbers(ln, 5, "noise rows require TIME LOW HIGH GAIN SLOPE", lim, open.Num)
		if len(d) > 0 {
			diags = append(diags, d...)
			i++
			continue
		}
		b.Rows = append(b.Rows, NoiseRow{Line: ln.Num, Time: vals[0], Low: vals[1], High: vals[2], Gain: vals[3], Slope: vals[4]})
		i++
	}
	if !closed {
		diags = append(diags, Diagnostic{Code: CodeUnclosedBlock,
			Message: fmt.Sprintf("expected end for noise opened on line %d", open.Num),
			Line: open.Num, BlockLine: open.Num})
		b.Close = open.Num
	}
	return b, i, diags
}

func parseHit(logical []logicalLine, start int, lim Limits, totalRows *int) (*HitBlock, int, []Diagnostic) {
	var diags []Diagnostic
	open := logical[start]
	if len(open.Tokens) != 1 {
		diags = append(diags, Diagnostic{Code: CodeFieldCount,
			Message: fmt.Sprintf("`hit` takes no fields; found %d", len(open.Tokens)-1),
			Line: open.Num, Column: colOf(open, 1), BlockLine: open.Num})
	}
	b := &HitBlock{Open: open.Num}
	i := start + 1
	rows := 0
	closed := false
	for i < len(logical) {
		ln := logical[i]
		kw := ln.Tokens[0]
		if kw == "end" {
			if len(ln.Tokens) != 1 {
				diags = append(diags, Diagnostic{Code: CodeFieldCount,
					Message: fmt.Sprintf("`end` takes no fields; found %d", len(ln.Tokens)-1),
					Line: ln.Num, Column: colOf(ln, 1), BlockLine: open.Num})
			}
			b.Close = ln.Num
			closed = true
			i++
			break
		}
		if isBlockStart(kw) {
			break
		}
		if kw == "spectrum" || kw == "curve" {
			diags = append(diags, Diagnostic{Code: CodeUnexpectedSect,
				Message: fmt.Sprintf("`%s` inside a hit block", kw), Line: ln.Num, Column: ln.Cols[0], BlockLine: open.Num})
			i++
			continue
		}
		rows++
		*totalRows++
		if rows > lim.MaxRowsPerBlock {
			diags = append(diags, Diagnostic{Code: CodeResourceLimit,
				Message: fmt.Sprintf("row count exceeds per-block limit %d", lim.MaxRowsPerBlock),
				Line: ln.Num, BlockLine: open.Num})
			i++
			continue
		}
		vals, d := parseNumbers(ln, 6, "hit rows require TIME LENGTH LOW HIGH GAIN SLOPE", lim, open.Num)
		if len(d) > 0 {
			diags = append(diags, d...)
			i++
			continue
		}
		b.Rows = append(b.Rows, HitRow{Line: ln.Num, Time: vals[0], Length: vals[1], Low: vals[2], High: vals[3], Gain: vals[4], Slope: vals[5]})
		i++
	}
	if !closed {
		diags = append(diags, Diagnostic{Code: CodeUnclosedBlock,
			Message: fmt.Sprintf("expected end for hit opened on line %d", open.Num),
			Line: open.Num, BlockLine: open.Num})
		b.Close = open.Num
	}
	return b, i, diags
}

func parseHarmonics(logical []logicalLine, start int, lim Limits, totalRows *int) (*HarmonicsBlock, int, []Diagnostic) {
	var diags []Diagnostic
	open := logical[start]
	if len(open.Tokens) != 1 {
		diags = append(diags, Diagnostic{Code: CodeFieldCount,
			Message: fmt.Sprintf("`harmonics` takes no fields; found %d", len(open.Tokens)-1),
			Line: open.Num, Column: colOf(open, 1), BlockLine: open.Num})
	}
	b := &HarmonicsBlock{Open: open.Num}
	i := start + 1
	// Expect spectrum.
	if i >= len(logical) || logical[i].Tokens[0] != "spectrum" || len(logical[i].Tokens) != 1 {
		got := "end of file"
		line := open.Num
		if i < len(logical) {
			got = "`" + logical[i].Tokens[0] + "`"
			line = logical[i].Num
		}
		diags = append(diags, Diagnostic{Code: CodeSectionRequired,
			Message: fmt.Sprintf("harmonics requires spectrum before curve; found %s", got),
			Line: line, BlockLine: open.Num})
		// Recovery: if next is curve, continue to curve; if end or block start or EOF, close.
		if i < len(logical) && logical[i].Tokens[0] == "curve" {
			// fall through to curve handling with empty spectrum
		} else if i < len(logical) && logical[i].Tokens[0] == "end" {
			b.Close = logical[i].Num
			i++
			return b, i, diags
		} else if i >= len(logical) || isBlockStart(logical[i].Tokens[0]) {
			diags = append(diags, Diagnostic{Code: CodeUnclosedBlock,
				Message: fmt.Sprintf("expected end for harmonics opened on line %d", open.Num),
				Line: open.Num, BlockLine: open.Num})
			b.Close = open.Num
			return b, i, diags
		} else {
			// Unexpected row before spectrum: skip until spectrum/curve/end.
			for i < len(logical) {
				kw := logical[i].Tokens[0]
				if kw == "spectrum" || kw == "curve" || kw == "end" || isBlockStart(kw) {
					break
				}
				diags = append(diags, Diagnostic{Code: CodeSectionRequired,
					Message: "harmonics requires spectrum before curve", Line: logical[i].Num, BlockLine: open.Num})
				i++
			}
			if i >= len(logical) || isBlockStart(logical[i].Tokens[0]) {
				diags = append(diags, Diagnostic{Code: CodeUnclosedBlock,
					Message: fmt.Sprintf("expected end for harmonics opened on line %d", open.Num),
					Line: open.Num, BlockLine: open.Num})
				b.Close = open.Num
				return b, i, diags
			}
			if i < len(logical) && logical[i].Tokens[0] == "end" {
				b.Close = logical[i].Num
				i++
				return b, i, diags
			}
		}
	}
	if i < len(logical) && logical[i].Tokens[0] == "spectrum" {
		if len(logical[i].Tokens) != 1 {
			diags = append(diags, Diagnostic{Code: CodeFieldCount,
				Message: "`spectrum` takes no fields", Line: logical[i].Num, BlockLine: open.Num})
		}
		b.SpectrumLine = logical[i].Num
		i++
	}
	// Spectrum rows until curve.
	specRows := 0
	sawCurve := false
	closed := false
	for i < len(logical) {
		ln := logical[i]
		kw := ln.Tokens[0]
		if kw == "curve" {
			sawCurve = true
			break
		}
		if kw == "end" {
			diags = append(diags, Diagnostic{Code: CodeSectionRequired,
				Message: "harmonics requires spectrum before curve", Line: ln.Num, BlockLine: open.Num})
			if len(ln.Tokens) != 1 {
				diags = append(diags, Diagnostic{Code: CodeFieldCount,
					Message: "`end` takes no fields", Line: ln.Num, BlockLine: open.Num})
			}
			b.Close = ln.Num
			closed = true
			i++
			break
		}
		if isBlockStart(kw) {
			diags = append(diags, Diagnostic{Code: CodeSectionRequired,
				Message: "harmonics requires spectrum before curve", Line: open.Num, BlockLine: open.Num})
			diags = append(diags, Diagnostic{Code: CodeUnclosedBlock,
				Message: fmt.Sprintf("expected end for harmonics opened on line %d", open.Num),
				Line: open.Num, BlockLine: open.Num})
			b.Close = open.Num
			return b, i, diags
		}
		if kw == "spectrum" {
			diags = append(diags, Diagnostic{Code: CodeUnexpectedSect,
				Message: "duplicate `spectrum` section", Line: ln.Num, BlockLine: open.Num})
			i++
			continue
		}
		specRows++
		*totalRows++
		if specRows > lim.MaxRowsPerBlock {
			diags = append(diags, Diagnostic{Code: CodeResourceLimit,
				Message: fmt.Sprintf("row count exceeds per-block limit %d", lim.MaxRowsPerBlock),
				Line: ln.Num, BlockLine: open.Num})
			i++
			continue
		}
		vals, d := parseNumbers(ln, 2, "spectrum rows require FREQUENCY WEIGHT", lim, open.Num)
		if len(d) > 0 {
			diags = append(diags, d...)
			i++
			continue
		}
		b.Spectrum = append(b.Spectrum, SpectrumRow{Line: ln.Num, Freq: vals[0], Weight: vals[1]})
		i++
	}
	if closed {
		return b, i, diags
	}
	if !sawCurve {
		if i >= len(logical) {
			diags = append(diags, Diagnostic{Code: CodeSectionRequired,
				Message: "harmonics requires spectrum before curve", Line: open.Num, BlockLine: open.Num})
			diags = append(diags, Diagnostic{Code: CodeUnclosedBlock,
				Message: fmt.Sprintf("expected end for harmonics opened on line %d", open.Num),
				Line: open.Num, BlockLine: open.Num})
			b.Close = open.Num
			return b, i, diags
		}
		// isBlockStart case already returned.
	}
	// Consume curve line.
	if i < len(logical) && logical[i].Tokens[0] == "curve" {
		if len(logical[i].Tokens) != 1 {
			diags = append(diags, Diagnostic{Code: CodeFieldCount,
				Message: "`curve` takes no fields", Line: logical[i].Num, BlockLine: open.Num})
		}
		b.CurveLine = logical[i].Num
		i++
	}
	// Curve rows until end.
	curveRows := 0
	closed = false
	for i < len(logical) {
		ln := logical[i]
		kw := ln.Tokens[0]
		if kw == "end" {
			if len(ln.Tokens) != 1 {
				diags = append(diags, Diagnostic{Code: CodeFieldCount,
					Message: "`end` takes no fields", Line: ln.Num, BlockLine: open.Num})
			}
			b.Close = ln.Num
			closed = true
			i++
			break
		}
		if isBlockStart(kw) {
			break
		}
		if kw == "spectrum" || kw == "curve" {
			diags = append(diags, Diagnostic{Code: CodeUnexpectedSect,
				Message: fmt.Sprintf("unexpected `%s` after curve", kw), Line: ln.Num, BlockLine: open.Num})
			i++
			continue
		}
		curveRows++
		*totalRows++
		if curveRows+specRows > lim.MaxRowsPerBlock {
			diags = append(diags, Diagnostic{Code: CodeResourceLimit,
				Message: fmt.Sprintf("row count exceeds per-block limit %d", lim.MaxRowsPerBlock),
				Line: ln.Num, BlockLine: open.Num})
			i++
			continue
		}
		vals, d := parseNumbers(ln, 3, "curve rows require TIME PITCH GAIN", lim, open.Num)
		if len(d) > 0 {
			diags = append(diags, d...)
			i++
			continue
		}
		b.Curve = append(b.Curve, TrajRow{Line: ln.Num, Time: vals[0], Freq: vals[1], Gain: vals[2]})
		i++
	}
	if !closed {
		diags = append(diags, Diagnostic{Code: CodeUnclosedBlock,
			Message: fmt.Sprintf("expected end for harmonics opened on line %d", open.Num),
			Line: open.Num, BlockLine: open.Num})
		b.Close = open.Num
	}
	return b, i, diags
}
