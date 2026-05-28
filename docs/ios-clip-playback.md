# Event clip `<video>` on iOS Safari needs three things from the BFF

Full explanation of the three constraints that shape the
`/api/events/:id/clip.mp4` pipeline. CLAUDE.md §12 Gotchas lists the
constraints by name; this file holds the underlying reasoning and the
implementation details for each fix.

iOS Safari is the strictest consumer of `/api/events/:id/clip.mp4`; the
endpoint exists in its current shape because of three independent
constraints, all of which must hold or playback breaks.

1. **HTTP Range, not chunked.** Safari's `<video>` issues a Range
   request on mount (even with `preload="metadata"`) and aborts with a
   broken-video icon if the response is not `206 Partial Content` with
   `Content-Length` and `Accept-Ranges: bytes`. Frigate 0.17's upstream
   `/api/events/<id>/clip.mp4` is chunked Transfer-Encoding and ignores
   Range — desktop browsers tolerate that; Safari does not. Fix: the
   BFF reads the full clip into memory (cap 64 MiB; default 30 s clip
   ≈ 13 MiB) and serves the buffer via `http.ServeContent`, which
   adds proper Range/206/Content-Length headers against a
   `bytes.NewReader`. If a clip ever exceeds 64 MiB the handler returns
   502 with `"exceeds limit"` in the log.

2. **One upstream fetch per event, not per Range.** `<video>` typically
   issues 10–20 parallel Range subrequests per source. Without caching,
   each one re-fetches ~35 MiB upstream, saturating the BFF and pushing
   individual ranges past the 30 s context ceiling — the browser
   cancels them and `MediaError` fires. Fix: the buffered bytes are
   stored in a process-lifetime LRU keyed by event id
   (`backend/internal/events/clipcache.go`) — 16 entries / 512 MiB
   hard caps; oversize single entries are served but not cached. No
   TTL because Frigate event ids are immutable (clips are write-once).
   No request coalescing yet: concurrent misses for the same id each
   fetch upstream once (worst case 2× bandwidth briefly); revisit only
   if a thundering-herd shows up in logs.

3. **HEVC must be tagged `hvc1`, not `hev1`.** Frigate writes
   main-stream recordings as HEVC with the `hev1` sample-entry tag.
   iPhone Safari silently rejects `hev1` in HTML5 <video> — the
   decoder is fine, the box-tag check is the gate. Desktop Chromium,
   Firefox, and macOS Safari all accept `hev1`, which is why this
   went unnoticed until iOS testing. Fix: `events.remuxHvc1` in
   `backend/internal/events/hevc_retag.go` walks the MP4 box tree
   with `github.com/abema/go-mp4`, locates every `hev1` sample-entry
   box's type field, and patches the four bytes in place to `hvc1`
   before the buffer is cached. No re-encode, same length, runs on
   the cache-miss path only. Confirmed across iPhone 13 and 16 Pro at
   2560×1440. If Frigate ever switches recordings to H.264 the retag
   becomes a silent no-op (the function only patches `hev1` boxes).

`GET` and `HEAD` share the same handler — `http.ServeContent` handles
HEAD natively by writing headers only.
