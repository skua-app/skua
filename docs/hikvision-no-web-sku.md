# Known issue: Hikvision DS-2CD2443G2-IW (no-web SKU)

Full analysis of why talk-back is architecturally blocked on the OEM
"no-web" variant of the Hikvision DS-2CD2443G2-IW. CLAUDE.md §12 Gotchas
holds the short summary; this file holds the underlying investigation and
the two replacement options.

The DS-2CD2443G2-IW-W is an OEM no-web-interface SKU sold without the
standard ISAPI surface that the retail DS-2CD2443G2-IW carries. Because
units of this SKU share the same firmware behaviour, any talk-back
strategy has to work for the SKU as a whole or for none of them.

A port scan against one unit shows three responsive services. Port 80
answers HTTP, but only with a 404 webserver — no UI is served. Port 554
serves RTSP normally: the main track is H.265 + AAC and the sub is H.264,
both `recvonly`. Port 8000 is Hikvision's proprietary SDK port — the
transport that the Hik-Connect mobile app uses for two-way audio.

The ISAPI surface is gone, not gated. Every endpoint that would normally
answer returns 404, including:

```
/ISAPI/System/deviceInfo
/ISAPI/System/status
/ISAPI/System/TwoWayAudio/channels
/ISAPI/System/Audio/channels
/ISAPI/Security/userCheck
```

The ONVIF surface, by contrast, is intact. `/onvif/device_service`
answers, `GetServices` reports Media2 with `ProfileCapabilities` including
`AudioOutput` and `AudioDecoder`, and `DeviceIO` reports `AudioSources=1`
and `AudioOutputs=1`. The camera hardware does have a speaker and the
ONVIF stack does expose it — the constraint is purely in the consumption
path.

That constraint sits in go2rtc. v1.9.10 (bundled with Frigate 0.17.1)
lists backchannel-capable incoming sources as `isapi`, `dvrip`, `tapo`,
`ring`, `tuya`, `roborock`, `doorbird`, `exec`, and `browser`. The
`onvif://` source in go2rtc is discovery-only and does not publish a
sendonly-audio track. Tested directly: adding
`onvif://admin:pass@<camera-ip>` as a second source on a camera's main
stream produced a `/api/streams` producer with `format_name=null` and
`medias=null`, and no microphone icon in `stream.html`. RTSP-level
`#backchannel=1` does attach a sendonly producer, but the camera never
accepts incoming audio over it because Hikvision gates the RTSP ANNOUNCE
path behind ISAPI auth, not RTSP auth — and ISAPI is unreachable on this
SKU. The Hik-Connect app's two-way audio goes through HCNetSDK on port
8000, which is Windows-only; reverse-engineering it is out of scope.

Conclusion: with this firmware, talk-back from a browser PWA is
architecturally blocked. If this ever becomes a priority again there are
two options. Replace the affected cameras with retail DS-2CD2443G2-IW
units that ship with the standard ISAPI surface, at which point go2rtc's
`isapi://` source handles backchannel out of the box. Or build a custom
go2rtc exec-source that drives the ONVIF `AudioOutput` SOAP endpoints
directly — a multi-week project that is rarely justified for a small
number of cameras.
