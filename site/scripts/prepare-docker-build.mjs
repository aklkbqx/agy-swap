#!/usr/bin/env node
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const dist = path.join(root, "dist");
const index = path.join(dist, "client", "index.html");
const sitemap = path.join(dist, "client", "sitemap.xml");

for (const file of [index, sitemap]) {
  if (!existsSync(file)) throw new Error("Missing Docker build input: " + file);
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

console.log("Prepared Docker static assets; sitemap lastmod:",lastmod);
