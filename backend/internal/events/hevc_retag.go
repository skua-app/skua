package events

import (
	"bytes"
	"fmt"

	"github.com/abema/go-mp4"
)

// remuxHvc1 rewrites every `hev1` video sample-entry fourCC tag to `hvc1`
// inside an MP4 byte slice, in place. The HEVC bitstream is untouched —
// only the 4-byte box-type field at each sample entry's header is patched.
// Output length equals input length.
//
// Why: iOS Safari refuses to decode HEVC tagged `hev1` in HTML5 <video>.
// The underlying VideoToolbox decoder handles the bitstream fine; the box-
// type check is the gate. Re-tagging is the documented Apple workaround,
// equivalent to `ffmpeg -c copy -tag:v hvc1`. Desktop Chromium, Firefox,
// and macOS Safari all accept `hev1`, so this transform is invisible to
// them.
//
// Behaviour:
//   - Non-MP4 input (no ftyp box at offset 4) is returned unchanged with no
//     error. This keeps the function safe to run unconditionally on any
//     upstream payload and lets unit tests pass opaque bytes through.
//   - Inputs containing no `hev1` boxes (already `hvc1`, or AVC clips
//     tagged `avc1`) are returned unchanged with no error.
//   - Inputs that parse as MP4 but blow up mid-walk return the original
//     bytes with a wrapped error so the caller can degrade gracefully
//     (serve the upstream bytes; iOS plays back only on retag-able clips,
//     which today is every Frigate output).
//   - On a successful retag the input slice is mutated in place and
//     returned. fetchClip is the only caller and owns the slice exclusively
//     before insertion into the cache; the cache treats stored slices as
//     immutable thereafter.
func remuxHvc1(buf []byte) ([]byte, error) {
	if !looksLikeMP4(buf) {
		return buf, nil
	}

	offsets, err := findHev1Offsets(bytes.NewReader(buf))
	if err != nil {
		return buf, fmt.Errorf("walk mp4 box tree: %w", err)
	}
	if len(offsets) == 0 {
		return buf, nil
	}

	for _, off := range offsets {
		if off+4 > uint64(len(buf)) {
			return buf, fmt.Errorf("hev1 patch offset %d out of bounds (len=%d)", off, len(buf))
		}
	}
	for _, off := range offsets {
		copy(buf[off:off+4], hvc1Bytes)
	}
	return buf, nil
}

// looksLikeMP4 reports whether buf appears to be an ISO BMFF/MP4 file. ISO
// BMFF mandates `ftyp` as the first box, so its type field sits at offset
// 4 (after the 4-byte size). This guards remuxHvc1 against accidentally
// walking arbitrary upstream payloads (HTML error pages, HLS playlists,
// short test fixtures) — they are returned untouched, silently.
func looksLikeMP4(buf []byte) bool {
	const ftypAt = 4
	return len(buf) >= ftypAt+4 && string(buf[ftypAt:ftypAt+4]) == "ftyp"
}

// hvc1Bytes is the fourCC tag we patch in. Defined as a package-level
// slice so the inner copy in the patch loop doesn't keep allocating.
var hvc1Bytes = []byte{'h', 'v', 'c', '1'}

// findHev1Offsets walks the MP4 box tree at r down through
// moov/trak/mdia/minf/stbl/stsd and returns the absolute byte offsets of
// every `hev1` sample-entry box's type field (i.e. boxInfo.Offset + 4).
// Multiple traks are handled. Other sample entries (mp4a/avc1/hvc1/...)
// are ignored.
func findHev1Offsets(r *bytes.Reader) ([]uint64, error) {
	var (
		hev1Type = mp4.BoxTypeHev1()
		stsdType = mp4.BoxTypeStsd()
		offsets  []uint64
	)
	// Only the boxes on the path moov → trak → mdia → minf → stbl → stsd
	// contain sample entries we care about. Everything else (mdat, free,
	// hdlr, …) is skipped without recursion, which keeps the walk cheap on
	// large clips: the box headers we visit are a few dozen, not thousands.
	containerTypes := map[mp4.BoxType]struct{}{
		mp4.BoxTypeMoov(): {},
		mp4.BoxTypeTrak(): {},
		mp4.BoxTypeMdia(): {},
		mp4.BoxTypeMinf(): {},
		mp4.BoxTypeStbl(): {},
		stsdType:          {},
	}

	_, err := mp4.ReadBoxStructure(r, func(h *mp4.ReadHandle) (any, error) {
		if h.BoxInfo.Type == hev1Type {
			// Box header layout is [4-byte size][4-byte type], even for the
			// 64-bit-size variant (size==1, then 8-byte largesize) — the
			// type field still sits immediately after the first 4 bytes.
			offsets = append(offsets, h.BoxInfo.Offset+4)
			return nil, nil
		}
		if _, ok := containerTypes[h.BoxInfo.Type]; ok {
			return h.Expand()
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return offsets, nil
}
