// Completes the "standalone" build so `node .next/standalone/server.js` serves
// static assets, as the Dockerfile does with COPY. Cross-platform, no shell.
import { cpSync } from "node:fs";
import { join } from "node:path";

const root = join(import.meta.dirname, "..");
cpSync(join(root, ".next", "static"), join(root, ".next", "standalone", ".next", "static"), { recursive: true });
