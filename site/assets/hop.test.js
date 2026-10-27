// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

import { describe, expect, test } from "bun:test";
import { nextHop, shooer, flee, HOP, SHOO } from "./hop.js";

// seeded is a small deterministic random source, so a walk can be replayed.
function seeded(seed) {
  var s = seed >>> 0;
  return function () {
    s = (s * 1664525 + 1013904223) >>> 0;
    return s / 4294967296;
  };
}

function walk(steps, bounds, random) {
  var bird = { x: 0, facing: -1 };
  var hops = [];
  for (var i = 0; i < steps; i++) {
    var hop = nextHop(bird, bounds, random);
    hops.push({ from: bird, to: hop });
    bird = hop;
  }
  return hops;
}

const window460 = { min: -300, max: 0 };

describe("the bird hops along the top of the window", () => {
  test("it never leaves the window", () => {
    for (const { to } of walk(2000, window460, seeded(1))) {
      expect(to.x).toBeGreaterThanOrEqual(window460.min);
      expect(to.x).toBeLessThanOrEqual(window460.max);
    }
  });

  test("each hop is a small one", () => {
    for (const { from, to } of walk(2000, window460, seeded(2))) {
      expect(Math.abs(to.x - from.x)).toBeLessThanOrEqual(HOP.longest);
    }
  });

  test("it hops the way it faces", () => {
    for (const { from, to } of walk(2000, window460, seeded(3))) {
      if (to.x !== from.x) expect(Math.sign(to.x - from.x)).toBe(to.facing);
    }
  });

  test("it rests between hops, for a moment", () => {
    for (const { to } of walk(500, window460, seeded(4))) {
      expect(to.rest).toBeGreaterThanOrEqual(HOP.shortestRest);
      expect(to.rest).toBeLessThanOrEqual(HOP.longestRest);
    }
  });

  test("it goes from one side to the other and back", () => {
    var hops = walk(400, window460, seeded(5));
    var reached = (x) => hops.some(({ to }) => Math.abs(to.x - x) < HOP.longest);
    expect(reached(window460.min)).toBe(true);
    expect(reached(window460.max)).toBe(true);
  });

  test("it turns around at an edge instead of hopping past it", () => {
    var hop = nextHop({ x: window460.min, facing: -1 }, window460, () => 0.5);
    expect(hop.facing).toBe(1);
    expect(hop.x).toBeGreaterThan(window460.min);
  });

  test("a window made narrower brings it back inside", () => {
    var hop = nextHop({ x: -500, facing: -1 }, window460, () => 0.5);
    expect(hop.x).toBeGreaterThanOrEqual(window460.min);
  });

  test("a window too narrow to hop in keeps it where it perches", () => {
    var hop = nextHop({ x: 0, facing: -1 }, { min: 0, max: 0 }, () => 0.5);
    expect(hop.x).toBe(0);
  });
});

describe("the pointer passing over the bird three times shoos it", () => {
  test("three passes in a row shoo it", () => {
    var pass = shooer();
    expect(pass(0)).toBe(false);
    expect(pass(800)).toBe(false);
    expect(pass(1600)).toBe(true);
  });

  test("passes spread out do not", () => {
    var pass = shooer();
    expect(pass(0)).toBe(false);
    expect(pass(SHOO.within)).toBe(false);
    expect(pass(2 * SHOO.within)).toBe(false);
  });

  test("once shooed, it takes three new passes", () => {
    var pass = shooer();
    pass(0); pass(100); pass(200);
    expect(pass(300)).toBe(false);
    expect(pass(400)).toBe(false);
    expect(pass(500)).toBe(true);
  });

  test("shooed, it flies to the far side of the window", () => {
    expect(flee({ x: -20, facing: -1 }, window460).x).toBe(window460.min);
    expect(flee({ x: -280, facing: 1 }, window460).x).toBe(window460.max);
  });

  test("it lands facing back the way it came, toward the pointer", () => {
    expect(flee({ x: -20, facing: -1 }, window460).facing).toBe(1);
    expect(flee({ x: -280, facing: -1 }, window460).facing).toBe(-1);
  });
});
