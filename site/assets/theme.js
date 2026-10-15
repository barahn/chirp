// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda
//
// Applies the theme the visitor chose, before the page is drawn, so a page
// does not flash in the system's theme first. It runs in <head>, without
// defer, and does only this; site.js draws the toggle. With no choice saved
// the page follows the system (prefers-color-scheme in site.css).
(function () {
  try {
    var t = localStorage.getItem("chirp-site-theme");
    if (t === "light" || t === "dark") document.documentElement.setAttribute("data-theme", t);
  } catch (e) {}
})();
