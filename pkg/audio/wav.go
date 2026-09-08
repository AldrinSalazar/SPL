package audio

import (
	"encoding/binary"
	"fmt"
	"math"

	"spl/pkg/spl"
)

// EncodeFloatWAV encodes mono IEEE float32 WAV. Conversion from float64 is
// explicit and checked; values above full scale are preserved.
func EncodeFloatWAV(samples []float64, rate int) ([]byte, *spl.Diagnostic) {
	if rate <= 0 {
		return nil, &spl.Diagnostic{Code: spl.CodeExportError, Message: "invalid sample rate"}
	}
	n := len(samples)
	// 44-byte header + 4*n data.
	dataSize := uint64(n) * 4
	fileSize := 36 + dataSize // RIFF size field (file length - 8)
	if fileSize > 0xFFFFFFFF {
		return nil, &spl.Diagnostic{Code: spl.CodeExportError,
			Message: fmt.Sprintf("WAV size %d bytes exceeds 4GB RIFF limit", fileSize+8)}
	}
	buf := make([]byte, 44+dataSize)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(fileSize))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)
	binary.LittleEndian.PutUint16(buf[20:22], 3) // IEEE float
	binary.LittleEndian.PutUint16(buf[22:24], 1) // mono
	binary.LittleEndian.PutUint32(buf[24:28], uint32(rate))
	binary.LittleEndian.PutUint32(buf[28:32], uint32(rate*4))
	binary.LittleEndian.PutUint16(buf[32:34], 4)
	binary.LittleEndian.PutUint16(buf[34:36], 32)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataSize))
	off := 44
	for _, v := range samples {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, &spl.Diagnostic{Code: spl.CodeExportError, Message: "nonfinite sample cannot be encoded to WAV"}
		}
		f := float32(v)
		if math.IsInf(float64(f), 0) {
			return nil, &spl.Diagnostic{Code: spl.CodeExportError,
				Message: fmt.Sprintf("sample %g overflows float32", v)}
		}
		binary.LittleEndian.PutUint32(buf[off:off+4], math.Float32bits(f))
		off += 4
	}
	return buf, nil
}

// DecodedWAV is an independent minimal reader for verification.
type DecodedWAV struct {
	Rate    int
	Samples []float32
}

// DecodeFloatWAV decodes mono IEEE float32 WAV produced by EncodeFloatWAV
// (also accepts files with extra chunks / extended fmt).
func DecodeFloatWAV(data []byte) (*DecodedWAV, error) {
	if len(data) < 44 {
		return nil, fmt.Errorf("too short")
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("not a WAVE file")
	}
	off := 12
	var rate int
	var fmtOK bool
	var samples []float32
	for off+8 <= len(data) {
		id := string(data[off : off+4])
		size := binary.LittleEndian.Uint32(data[off+4 : off+8])
		start := off + 8
		end := start + int(size)
		if end > len(data) {
			return nil, fmt.Errorf("chunk %s overruns file", id)
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, fmt.Errorf("bad fmt chunk")
			}
			audioFmt := binary.LittleEndian.Uint16(data[start : start+2])
			ch := binary.LittleEndian.Uint16(data[start+2 : start+4])
			rate = int(binary.LittleEndian.Uint32(data[start+4 : start+8]))
			bits := binary.LittleEndian.Uint16(data[start+14 : start+16])
			if audioFmt != 3 || ch != 1 || bits != 32 {
				return nil, fmt.Errorf("only mono float32 supported (fmt=%d ch=%d bits=%d)", audioFmt, ch, bits)
			}
			fmtOK = true
		case "data":
			if !fmtOK {
				return nil, fmt.Errorf("data before fmt")
			}
			if size%4 != 0 {
				return nil, fmt.Errorf("data size not multiple of 4")
			}
			count := int(size) / 4
			samples = make([]float32, count)
			for i := 0; i < count; i++ {
				bits := binary.LittleEndian.Uint32(data[start+i*4 : start+i*4+4])
				samples[i] = math.Float32frombits(bits)
			}
		}
		off = end
		if size%2 == 1 {
			off++ // pad byte
		}
	}
	if !fmtOK || samples == nil {
		return nil, fmt.Errorf("missing fmt/data chunks")
	}
	return &DecodedWAV{Rate: rate, Samples: samples}, nil
}
