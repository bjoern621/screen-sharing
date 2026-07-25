package webviewer

// viewerPage is the standalone browser viewer. It reads the stream name from its
// own path (/name), opens the matching WebSocket, decodes VP9 with WebCodecs and
// draws to a canvas. Self-contained so any browser on the LAN can open it with
// no build step; the web grid uses the same WebSocket through its own sink.
const viewerPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>stream viewer</title>
<style>
  html, body { margin: 0; height: 100%; background: #000; color: #ddd;
    font: 14px system-ui, sans-serif; }
  #stage { display: flex; align-items: center; justify-content: center; height: 100%; }
  canvas { max-width: 100%; max-height: 100%; }
  #msg { position: fixed; top: 12px; left: 12px; padding: 6px 10px;
    background: rgba(0,0,0,.6); border-radius: 6px; }
</style>
</head>
<body>
<div id="stage"><canvas id="c"></canvas></div>
<div id="msg">connecting…</div>
<script>
const name = decodeURIComponent(location.pathname.replace(/^\/+/, ""));
const msg = document.getElementById("msg");
const canvas = document.getElementById("c");
const ctx = canvas.getContext("2d");
const CODEC = "vp09.01.10.08";
const HEADER = 9;

function fail(text) { msg.textContent = text; }

async function main() {
  if (!name) return fail("no stream name in the URL");
  if (typeof VideoDecoder === "undefined") return fail("this browser has no WebCodecs");
  const support = await VideoDecoder.isConfigSupported({ codec: CODEC }).catch(() => null);
  if (!support || !support.supported) return fail("this browser cannot decode VP9 profile 1");

  const decoder = new VideoDecoder({
    output(frame) {
      if (canvas.width !== frame.displayWidth) canvas.width = frame.displayWidth;
      if (canvas.height !== frame.displayHeight) canvas.height = frame.displayHeight;
      ctx.drawImage(frame, 0, 0);
      frame.close();
    },
    error(e) { fail(e.message); },
  });
  decoder.configure({ codec: CODEC });

  const ws = new WebSocket(` + "`ws://${location.host}/ws/${encodeURIComponent(name)}`" + `);
  ws.binaryType = "arraybuffer";
  ws.onopen = () => { msg.textContent = name; setTimeout(() => msg.style.display = "none", 1500); };
  ws.onerror = () => fail("websocket error");
  ws.onclose = () => fail("stream ended");
  ws.onmessage = e => {
    const buf = e.data;
    if (buf.byteLength <= HEADER) return;
    const view = new DataView(buf);
    const keyframe = (view.getUint8(0) & 1) !== 0;
    const pts = Number(view.getBigUint64(1));
    decoder.decode(new EncodedVideoChunk({
      type: keyframe ? "key" : "delta",
      timestamp: pts,
      data: new Uint8Array(buf, HEADER),
    }));
  };
}
main();
</script>
</body>
</html>`
