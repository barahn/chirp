// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda
//
// Applies the theme the visitor chose, before the page is drawn, so a page
// does not flash in the system's theme first. It runs in <head>, without
// defer, and does only this and the move below; site.js draws the toggle. With
// no choice saved the page follows the system (prefers-color-scheme in
// site.css).
//
// The status page lives at status.usechirp.app, whose Worker serves this same
// page from usechirp.app/status/. GitHub Pages cannot redirect, and a redirect
// in front of it would take the page from the Worker too, so a visitor who
// reaches it on usechirp.app is moved here, before anything is drawn. On
// status.usechirp.app the host does not match, so it never loops.
(function () {
  var h = location.hostname, p = location.pathname;
  if ((h === "usechirp.app" || h === "www.usechirp.app") && (p === "/status" || p.indexOf("/status/") === 0)) {
    var sub = p.replace(/^\/status\/?/, "");
    location.replace("https://status.usechirp.app/" + (sub ? sub : "") + location.search + location.hash);
  }
})();
(function () {
  try {
    var t = localStorage.getItem("chirp-site-theme");
    if (t === "light" || t === "dark") document.documentElement.setAttribute("data-theme", t);
  } catch (e) {}
})();
