// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// How the hero bird walks along the top of the app window: small hops from
// one side to the other, a rest between them, and off to the far side when
// the pointer passes over it three times in a row. Only the choices live
// here, with no page in sight, so hop.test.js can replay them; site.js moves
// the bird.
//
// x is how far the bird is from where it lands, in pixels: 0 at its perch on
// the right, down to bounds.min at the left. facing is 1 for right, -1 for
// left.
(function (root) {
  "use strict";

  var HOP = { shortest: 18, longest: 30, shortestRest: 800, longestRest: 3000, turnChance: 0.12 };
  var SHOO = { passes: 3, within: 4000 };

  function between(lo, hi, random) { return lo + (hi - lo) * random(); }

  function clamp(x, bounds) { return Math.min(bounds.max, Math.max(bounds.min, x)); }

  function nextHop(bird, bounds, random) {
    var x = clamp(bird.x, bounds);
    var facing = random() < HOP.turnChance ? -bird.facing : bird.facing;
    var length = between(HOP.shortest, HOP.longest, random);
    var room = facing > 0 ? bounds.max - x : x - bounds.min;
    if (room < length) {
      facing = -facing;
      length = Math.min(length, facing > 0 ? bounds.max - x : x - bounds.min);
    }
    return {
      x: Math.round(x + facing * length),
      facing: facing,
      rest: Math.round(between(HOP.shortestRest, HOP.longestRest, random))
    };
  }

  // shooer counts the pointer's passes over the bird; each call is one pass,
  // at time now in milliseconds, and says whether it was the one that shoos.
  function shooer() {
    var passes = [];
    return function (now) {
      passes = passes.filter(function (t) { return now - t < SHOO.within; });
      passes.push(now);
      if (passes.length < SHOO.passes) return false;
      passes = [];
      return true;
    };
  }

  function flee(bird, bounds) {
    var toLeft = bird.x - bounds.min > bounds.max - bird.x;
    return { x: toLeft ? bounds.min : bounds.max, facing: toLeft ? 1 : -1 };
  }

  var api = { HOP: HOP, SHOO: SHOO, nextHop: nextHop, shooer: shooer, flee: flee };
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.ChirpHop = api;
})(this);
