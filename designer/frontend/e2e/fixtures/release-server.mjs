// Stand-in for GitHub's "latest release" endpoint in the end-to-end tests
// (PDX_UPDATE_URL). By default it answers with an old version, so no test
// sees the update dialog. The update test sets a newer release with
// POST /latest and reads the number of checks from GET /count.
//   node release-server.mjs <port>
import { createServer } from "node:http";

const port = Number(process.argv[2] ?? 8797);
const old = { tag_name: "v0.0.1", html_url: "https://github.com/tim-riep/pdx/releases/tag/v0.0.1", body_html: "", published_at: "2026-01-01T00:00:00Z" };
let release = old;
let count = 0;

createServer((req, res) => {
  if (req.url === "/health") return res.end("ok");
  if (req.url === "/count") return res.end(String(count));
  if (req.url === "/latest" && req.method === "POST") {
    let body = "";
    req.on("data", (d) => (body += d));
    req.on("end", () => {
      release = body ? JSON.parse(body) : old;
      count = 0;
      res.end("ok");
    });
    return;
  }
  if (req.url === "/release") {
    count++;
    res.setHeader("Content-Type", "application/json");
    return res.end(JSON.stringify(release));
  }
  res.statusCode = 404;
  res.end();
}).listen(port, "127.0.0.1");
