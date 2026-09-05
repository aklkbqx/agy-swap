#!/usr/bin/env node
import { copyFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const dist = path.join(root, "dist");
const index = path.join(dist, "client", "index.html");
const sitemap = path.join(dist, "client", "sitemap.xml");
const worker = path.join(root, "worker", "index.js");
const hosting = path.join(root, ".openai", "hosting.json");

for (const file of [index, sitemap, worker, hosting]) {
  if (!existsSync(file)) throw new Error("Missing Sites build input: " + file);
}

const lastmod = new Date().toISOString().slice(0, 10);
const sitemapXml = readFileSync(sitemap, "utf8");
if (!/<lastmod>[^<]*<\/lastmod>/.test(sitemapXml)) {
  throw new Error("sitemap.xml missing lastmod to stamp");
}
writeFileSync(
  sitemap,
  sitemapXml.replace(/<lastmod>[^<]*<\/lastmod>/g, `<lastmod>${lastmod}</lastmod>`),
);

mkdirSync(path.join(dist, "server"), { recursive: true });
mkdirSync(path.join(dist, ".openai"), { recursive: true });
copyFileSync(worker, path.join(dist, "server", "index.js"));
copyFileSync(hosting, path.join(dist, ".openai", "hosting.json"));

console.log("Prepared Sites build: dist/server/index.js and dist/.openai/hosting.json");
console.log("Stamped sitemap lastmod:", lastmod);
