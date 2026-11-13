// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

import { describe, expect, test } from "bun:test";
import { readFileSync } from "fs";
import { resolve } from "path";

describe("site translations in site.js", () => {
  const code = readFileSync(resolve(import.meta.dir, "site.js"), "utf8");

  const ptMatch = code.match(/var PT = \{([\s\S]*?)\n  \};/);
  const tagsMatch = code.match(/var TAGS = \{([\s\S]*?)\};/);
  const tagAttrsMatch = code.match(/var TAG_ATTRS = \{([\s\S]*?)\n  \};/);

  test("site.js contains PT dictionary and TAGS allowlist", () => {
    expect(ptMatch).not.toBeNull();
    expect(tagsMatch).not.toBeNull();
    expect(tagAttrsMatch).not.toBeNull();
  });

  const TAGS = {};
  const tagEntries = tagsMatch[1].split(",").map(s => s.trim()).filter(Boolean);
  for (const entry of tagEntries) {
    const [k] = entry.split(":").map(s => s.trim());
    TAGS[k] = 1;
  }

  // Parse TAG_ATTRS
  const TAG_ATTRS = {
    a: { href: /^(https:\/\/|\/)/, rel: /^noopener noreferrer$/, target: /^_blank$/ },
    span: { class: /^chirp$/ }
  };

  function validateMarkup(key, src) {
    const token = /<(\/?)([a-z0-9]+)((?:\s+[a-z-]+="[^"<>]*")*)\s*>|[^<]+|</g;
    const open = ["root"];
    let m, a;
    while ((m = token.exec(src))) {
      if (m[2] === undefined) {
        if (m[0] === "<") return `stray < in ${key}`;
        continue;
      }
      if (!TAGS[m[2]]) return `unallowed tag <${m[2]}> in ${key}`;
      if (m[1]) {
        const top = open.pop();
        if (top !== m[2]) return `mismatched close </${m[2]}> (expected </${top}>) in ${key}`;
        continue;
      }
      const attr = /([a-z-]+)="([^"]*)"/g;
      while ((a = attr.exec(m[3]))) {
        const allowed = TAG_ATTRS[m[2]] && TAG_ATTRS[m[2]][a[1]];
        if (!allowed || !allowed.test(a[2])) {
          return `unallowed attribute ${a[1]}="${a[2]}" on <${m[2]}> in ${key}`;
        }
      }
      open.push(m[2]);
    }
    if (open.length !== 1) {
      return `unclosed tags in ${key}: ${open.slice(1).join(", ")}`;
    }
    return null;
  }

  // Extract keys and values from PT block
  const lines = ptMatch[1].split("\n");
  const keys = [];
  let currentKey = null;
  let currentValue = "";

  for (const line of lines) {
    const match = line.match(/^\s*([a-zA-Z0-9_]+):\s*"(.*)/);
    if (match) {
      if (currentKey) keys.push([currentKey, currentValue]);
      currentKey = match[1];
      currentValue = match[2];
    } else if (currentKey) {
      currentValue += "\n" + line;
    }
  }
  if (currentKey) keys.push([currentKey, currentValue]);

  for (const [key, rawVal] of keys) {
    test(`translation key "${key}" has balanced and valid markup`, () => {
      // Clean trailing comma or quote
      const val = rawVal.replace(/",?\s*$/, "").replace(/\\"/g, '"');
      const err = validateMarkup(key, val);
      expect(err).toBeNull();
    });
  }
});
