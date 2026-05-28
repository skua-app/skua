package events

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abema/go-mp4"
)

// boxBytes wraps payload in an ISO BMFF box with the given 4-cc type. The
// header is the small-form 8 bytes ([size big-endian uint32][type]).
func boxBytes(t *testing.T, typ string, payload []byte) []byte {
	t.Helper()
	if len(typ) != 4 {
		t.Fatalf("box type must be 4 ASCII bytes, got %q", typ)
	}
	out := make([]byte, 8, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(8+len(payload)))
	copy(out[4:8], typ)
	out = append(out, payload...)
	return out
}

// minimalVisualSampleEntryPayload returns 78 bytes shaped as a SampleEntry
// (8 bytes) + VisualSampleEntry (70 bytes). The fields don't matter for the
// box-walk; go-mp4 reads them when expanding the sample entry, but our
// retag only inspects type and offset and never expands hev1/hvc1.
func minimalVisualSampleEntryPayload() []byte {
	buf := new(bytes.Buffer)
	buf.Write(make([]byte, 6))                                // SampleEntry reserved
	_ = binary.Write(buf, binary.BigEndian, uint16(1))        // data_reference_index
	buf.Write(make([]byte, 16))                               // pre_defined + reserved
	_ = binary.Write(buf, binary.BigEndian, uint16(1920))     // width
	_ = binary.Write(buf, binary.BigEndian, uint16(1080))     // height
	_ = binary.Write(buf, binary.BigEndian, uint32(0x480000)) // horiz res (72 dpi 16.16)
	_ = binary.Write(buf, binary.BigEndian, uint32(0x480000)) // vert res
	buf.Write(make([]byte, 4))                                // reserved
	_ = binary.Write(buf, binary.BigEndian, uint16(1))        // frame_count
	buf.Write(make([]byte, 32))                               // compressorname
	_ = binary.Write(buf, binary.BigEndian, uint16(0x18))     // depth
	_ = binary.Write(buf, binary.BigEndian, int16(-1))        // pre_defined
	return buf.Bytes()
}

// minimalAudioSampleEntryPayload returns 28 bytes shaped as SampleEntry +
// AudioSampleEntry. Used for the multi-trak test (audio + video).
func minimalAudioSampleEntryPayload() []byte {
	buf := new(bytes.Buffer)
	buf.Write(make([]byte, 6))                                 // SampleEntry reserved
	_ = binary.Write(buf, binary.BigEndian, uint16(1))         // data_reference_index
	buf.Write(make([]byte, 8))                                 // reserved
	_ = binary.Write(buf, binary.BigEndian, uint16(2))         // channelcount
	_ = binary.Write(buf, binary.BigEndian, uint16(16))        // samplesize
	_ = binary.Write(buf, binary.BigEndian, uint16(0))         // pre_defined
	_ = binary.Write(buf, binary.BigEndian, uint16(0))         // reserved
	_ = binary.Write(buf, binary.BigEndian, uint32(48000<<16)) // samplerate (16.16)
	return buf.Bytes()
}

// buildClip assembles a synthetic MP4 with the given list of sample
// entries, each wrapped in its own trak. The output is parseable by
// go-mp4's ReadBoxStructure end-to-end and is the smallest shape that
// exercises the moov → trak → mdia → minf → stbl → stsd path.
func buildClip(t *testing.T, sampleEntries ...[]byte) []byte {
	t.Helper()

	// ftyp: major_brand=isom, minor_version=0, compatible_brands=isom,mp42
	ftypPayload := []byte("isom\x00\x00\x00\x00isommp42")
	ftyp := boxBytes(t, "ftyp", ftypPayload)

	var traks []byte
	for _, entry := range sampleEntries {
		// stsd: FullBox header (1B version + 3B flags) + uint32 entry_count + entry
		stsdPayload := make([]byte, 0, 8+len(entry))
		stsdPayload = append(stsdPayload, 0, 0, 0, 0) // version + flags
		stsdPayload = binary.BigEndian.AppendUint32(stsdPayload, 1)
		stsdPayload = append(stsdPayload, entry...)
		stsd := boxBytes(t, "stsd", stsdPayload)

		stbl := boxBytes(t, "stbl", stsd)
		minf := boxBytes(t, "minf", stbl)
		mdia := boxBytes(t, "mdia", minf)
		trak := boxBytes(t, "trak", mdia)
		traks = append(traks, trak...)
	}
	moov := boxBytes(t, "moov", traks)

	out := make([]byte, 0, len(ftyp)+len(moov))
	out = append(out, ftyp...)
	out = append(out, moov...)
	return out
}

// findFirstTagOffset finds the byte offset of the first occurrence of tag
// in buf. Used to assert the retag patched the right four bytes.
func findFirstTagOffset(t *testing.T, buf []byte, tag string) int {
	t.Helper()
	idx := bytes.Index(buf, []byte(tag))
	if idx < 0 {
		t.Fatalf("tag %q not present in buffer", tag)
	}
	return idx
}

func TestRemuxHvc1_SingleHev1Trak(t *testing.T) {
	hev1 := boxBytes(t, "hev1", minimalVisualSampleEntryPayload())
	clip := buildClip(t, hev1)
	hev1Off := findFirstTagOffset(t, clip, "hev1")

	original := append([]byte(nil), clip...)

	out, err := remuxHvc1(clip)
	if err != nil {
		t.Fatalf("remuxHvc1: %v", err)
	}
	if len(out) != len(original) {
		t.Fatalf("length changed: in=%d out=%d", len(original), len(out))
	}
	if bytes.Contains(out, []byte("hev1")) {
		t.Errorf("hev1 still present after retag")
	}
	if string(out[hev1Off:hev1Off+4]) != "hvc1" {
		t.Errorf("tag at offset %d = %q, want hvc1", hev1Off, out[hev1Off:hev1Off+4])
	}

	// Everything except the four patched bytes must be identical.
	for i, b := range out {
		if i >= hev1Off && i < hev1Off+4 {
			continue
		}
		if b != original[i] {
			t.Fatalf("byte %d changed: original=0x%02x, out=0x%02x", i, original[i], b)
		}
	}
}

func TestRemuxHvc1_MultiTrakOnlyVideoPatched(t *testing.T) {
	hev1 := boxBytes(t, "hev1", minimalVisualSampleEntryPayload())
	mp4a := boxBytes(t, "mp4a", minimalAudioSampleEntryPayload())
	clip := buildClip(t, hev1, mp4a)

	mp4aOff := findFirstTagOffset(t, clip, "mp4a")

	out, err := remuxHvc1(clip)
	if err != nil {
		t.Fatalf("remuxHvc1: %v", err)
	}
	if bytes.Contains(out, []byte("hev1")) {
		t.Errorf("hev1 still present after retag")
	}
	if string(out[mp4aOff:mp4aOff+4]) != "mp4a" {
		t.Errorf("audio sample entry was mutated: tag=%q at offset %d", out[mp4aOff:mp4aOff+4], mp4aOff)
	}
}

func TestRemuxHvc1_AlreadyHvc1NoOp(t *testing.T) {
	hvc1 := boxBytes(t, "hvc1", minimalVisualSampleEntryPayload())
	clip := buildClip(t, hvc1)
	original := append([]byte(nil), clip...)

	out, err := remuxHvc1(clip)
	if err != nil {
		t.Fatalf("remuxHvc1: %v", err)
	}
	if !bytes.Equal(out, original) {
		t.Errorf("bytes mutated when no hev1 present")
	}
}

func TestRemuxHvc1_AVCNoOp(t *testing.T) {
	avc1 := boxBytes(t, "avc1", minimalVisualSampleEntryPayload())
	clip := buildClip(t, avc1)
	original := append([]byte(nil), clip...)

	out, err := remuxHvc1(clip)
	if err != nil {
		t.Fatalf("remuxHvc1: %v", err)
	}
	if !bytes.Equal(out, original) {
		t.Errorf("bytes mutated when no hev1 present (avc1 clip)")
	}
}

func TestRemuxHvc1_CorruptMP4ReturnsError(t *testing.T) {
	// ftyp present so looksLikeMP4 passes — but the rest of the buffer is a
	// truncated box header that will trip the walker mid-parse.
	corrupt := make([]byte, 12)
	binary.BigEndian.PutUint32(corrupt[0:4], 24) // ftyp claims 24-byte size
	copy(corrupt[4:8], "ftyp")
	// payload is 4 bytes, far short of the declared 24. The walker sees the
	// box header, attempts to seek past the box, then fails to read the next
	// header at EOF — non-nil error.
	out, err := remuxHvc1(corrupt)
	if err == nil {
		t.Fatal("expected error on corrupt MP4")
	}
	if !bytes.Equal(out, corrupt) {
		t.Errorf("on error, original bytes must be returned untouched")
	}
}

func TestRemuxHvc1_NonMP4InputSilent(t *testing.T) {
	// Opaque payload (no ftyp) — the function must return the input
	// unchanged with no error and no log noise.
	in := []byte("this-is-not-an-mp4-at-all")
	out, err := remuxHvc1(in)
	if err != nil {
		t.Fatalf("expected silent pass-through, got error: %v", err)
	}
	if !bytes.Equal(out, in) {
		t.Errorf("bytes mutated for non-MP4 input")
	}
}

func TestRemuxHvc1_ParseableByGoMP4(t *testing.T) {
	// Sanity check that our synthetic fixture is itself a valid MP4 from
	// go-mp4's perspective — if this ever stops parsing, the rest of the
	// tests in this file are useless.
	hev1 := boxBytes(t, "hev1", minimalVisualSampleEntryPayload())
	clip := buildClip(t, hev1)
	if _, err := mp4.ReadBoxStructure(bytes.NewReader(clip), func(h *mp4.ReadHandle) (any, error) {
		switch h.BoxInfo.Type {
		case mp4.BoxTypeMoov(), mp4.BoxTypeTrak(), mp4.BoxTypeMdia(),
			mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd():
			return h.Expand()
		}
		return nil, nil
	}); err != nil {
		t.Fatalf("synthetic clip fixture is not a valid MP4: %v", err)
	}
}

// TestServeClip_RetagsHev1OnCacheMiss is the integration smoke for the
// fetchClip path: when the upstream serves a real-shape MP4 carrying
// hev1, the BFF's served bytes (and cached bytes) come out tagged hvc1.
func TestServeClip_RetagsHev1OnCacheMiss(t *testing.T) {
	hev1 := boxBytes(t, "hev1", minimalVisualSampleEntryPayload())
	clip := buildClip(t, hev1)
	if !bytes.Contains(clip, []byte("hev1")) {
		t.Fatalf("fixture missing hev1 tag")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(clip)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/events/hevc-1/clip.mp4", nil)
	rec := httptest.NewRecorder()
	if err := c.ServeClip(req.Context(), "hevc-1", rec, req, ""); err != nil {
		t.Fatalf("ServeClip: %v", err)
	}
	body := rec.Body.Bytes()
	if bytes.Contains(body, []byte("hev1")) {
		t.Errorf("served body still contains hev1 — retag did not apply")
	}
	if !bytes.Contains(body, []byte("hvc1")) {
		t.Errorf("served body missing hvc1 — retag did not apply")
	}

	// Cache hit on a second request must also serve the retagged bytes.
	req2 := httptest.NewRequest(http.MethodGet, "/api/events/hevc-1/clip.mp4", nil)
	rec2 := httptest.NewRecorder()
	if err := c.ServeClip(req2.Context(), "hevc-1", rec2, req2, ""); err != nil {
		t.Fatalf("ServeClip (cache hit): %v", err)
	}
	if !bytes.Equal(rec.Body.Bytes(), rec2.Body.Bytes()) {
		t.Errorf("cache-hit body differs from first response")
	}
	if !strings.Contains(rec2.Body.String(), "hvc1") {
		t.Errorf("cache-hit body missing hvc1")
	}
}
