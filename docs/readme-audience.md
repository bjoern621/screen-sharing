# README audience

Who `README.md` is written for, and what a sentence in it may assume.
The reader wants to show a screen to a few people and has decided nothing yet.
Nothing else about them is known, so the page is written for the least technical reading.

## The reader

Installs apps and reads settings screens without help.
Has shared a screen over a video call,
and found the picture too soft to read code, a spreadsheet or a terminal on.
Knows a picture has a resolution and a frame rate,
and that a better picture costs more of the connection.

Has never picked an encoder, a pixel format or a transport, and gets no reason to start here.
Has not heard of this project, of what it stands beside, or of the server in the middle.

Four questions, in this order:
what is it, does the picture look better, does it run on my machine, how do I start.
The page ends when those are answered.

## Vocabulary ceiling

A word is allowed where the reader met it away from this project.
H.265 passes: a phone camera and a TV both claim it.
4:4:4 does not: outside video encoding nothing says it.

| Say | Rather than |
| --- | --- |
| text stays sharp, colours stay true | 4:4:4, planar RGB, full range, 8 or 10 bit |
| the graphics card does the encoding | NVENC, Quick Sync, AMF, VAAPI, Vulkan Video, V4L2, Rockchip MPP, x264, SVT-AV1 |
| H.265, once, as the format behind a bitrate | AVC, HEVC, RExt, AV1, VP9, VP8 as a list |
| watch in the app, in a browser, or in a player | SRT, RTSP, RTMP, WHIP, WHEP, HLS, Media over QUIC |
| a server in the middle passes the picture on, and everyone reads from it | MediaMTX, ingest, listener, path prefix, token, lease |
| a group is a key, and holding it is membership | 256-bit, digest, JWT, signature |
| the app measures the delay and shows it | per-stage timing, capture-to-render, the SRT latency window |

Two words the app puts on screen keep their name here: **group** and **relay**.
The reader meets both again on the first run.
Each is explained at its first use, in the sentence that uses it.

`glossary.md` holds every term this table sends away, and `video-stack.md` holds what they mean together.

## Platforms are a download

Windows and Linux, named where a download is picked and nowhere else.
No sentence compares them, and no sentence carries what one of them needs.
A package that behaves differently is a row in the download table with its file name,
and the reason lives one link away.

`install.md` owns every platform detail: what each download carries, what a distribution supplies, and what a first run asks.

## What the page answers

| Question | Answered by |
| --- | --- |
| What is it | the first three lines |
| Does the picture look better | one paragraph on what a video call gives up, then the screenshots |
| What does it do | one list, each entry a thing the reader would notice using it |
| Does it run here | the download table |
| How do I start | four numbered steps |
| Who sees my screen | one paragraph on the server in the middle |
| Everything under it | a link block to `docs/`, and the developer section |

Anything a reader has to know before installing is on the page.
Everything else is a link.

## What a screenshot has to prove

Two at most, each carrying a claim the page makes: a stream being set up, and several arriving at once.
Full window, so the reader sees what the download is.
Text at its own size, unscaled, the claim being that it stays readable.
No cursor, no personal content, no group key in frame.
A tile draws its picture out of GPU memory, so a shot of the grid comes off a running machine.

## Tests

Read the draft against these, one sentence at a time.

- Say it aloud to a friend who edits video for a living and has never shipped code. A sentence that stalls them goes.
- Every sentence moves the reader towards installing it or away from it. One that does neither goes.
- One name per thing, and the app's name wins: group, stream, tile, member.
- A number stays where the reader can feel it, like 60 fps. A number needing its unit explained goes.
- A capability is written as what the reader gets, and the part that produces it stays unnamed.
