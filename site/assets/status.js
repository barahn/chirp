// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda
//
// The status page: fetches the status service's figures (cmd/chirp-status)
// and shows them, every round. Everything it shows comes from the API and is
// written as text, never parsed as HTML. It never shows old figures as
// current: their age is always on the page, measured by the service's clock,
// not the visitor's.
(function () {
  "use strict";

  var main = document.getElementById("status");
  if (!main) return;
  var API = main.getAttribute("data-api");
  var HISTORY_EVERY = 5 * 60 * 1000;
  var DAYS = 90;

  var S = {
    en: {
      loading: "Loading the figures…",
      upOf: function (up, n) { return up + " of " + n + (n === 1 ? " server" : " servers") + " up"; },
      none: "Nothing to probe yet",
      checked: function (ago, v) { return "Checked " + ago + " ago, from " + v; },
      down: "The monitor is not answering",
      downNote: "This page can't say anything about the servers until it answers. It asks again every 30 seconds.",
      lost: function (ago) { return "The monitor stopped answering. These are its figures from " + ago + " ago."; },
      stale: function (ago) { return "The monitor answers, but its latest figures are " + ago + " old: it has stopped probing."; },
      state: { up: "Up", failing: "Failing", offline: "Off-line", unknown: "Unknown" },
      offlineFor: function (d) { return "Off-line for " + d; },
      failingFor: function (d) { return "Failing for " + d; },
      unknownFor: function (d) { return "Not probed for " + d; },
      notYet: "Not probed yet",
      lastAnswered: function (t) { return "last answered " + t; },
      neverAnswered: "has never answered",
      err: {
        timeout: "no answer within 5 s", dns: "name not found", connect: "could not connect",
        tls: "TLS certificate or handshake failed", status: "answered with an error", other: "failed"
      },
      dedicated: "Dedicated", "new": "New", suspended: "Suspended", cdn: "behind a CDN",
      w24h: "24 hours", w30d: "30 days", w90d: "90 days", latency: "Latency",
      shortHistory: "short history", noData: "no data",
      strip: function (n, low) { return "Last 90 days: " + n + " with data" + (low ? ", the lowest " + low : ""); },
      dayTitle: function (d, p) { return d + ": " + p; },
      ago90: "90 days ago", today: "today",
      noMatch: "No server matches this filter."
    },
    "pt-BR": {
      loading: "Carregando os números…",
      upOf: function (up, n) { return up + " de " + n + (n === 1 ? " servidor" : " servidores") + " no ar"; },
      none: "Nada para sondar ainda",
      checked: function (ago, v) { return "Verificado há " + ago + ", a partir de " + v; },
      down: "O monitor não está respondendo",
      downNote: "Esta página não consegue dizer nada sobre os servidores até ele responder. Ela pergunta de novo a cada 30 segundos.",
      lost: function (ago) { return "O monitor parou de responder. Estes são os números dele de " + ago + " atrás."; },
      stale: function (ago) { return "O monitor responde, mas os números mais recentes têm " + ago + ": ele parou de sondar."; },
      state: { up: "No ar", failing: "Falhando", offline: "Fora do ar", unknown: "Desconhecido" },
      offlineFor: function (d) { return "Fora do ar há " + d; },
      failingFor: function (d) { return "Falhando há " + d; },
      unknownFor: function (d) { return "Sem sondagem há " + d; },
      notYet: "Ainda não sondado",
      lastAnswered: function (t) { return "última resposta em " + t; },
      neverAnswered: "nunca respondeu",
      err: {
        timeout: "sem resposta em 5 s", dns: "nome não encontrado", connect: "não conectou",
        tls: "falha no certificado ou na negociação TLS", status: "respondeu com erro", other: "falhou"
      },
      dedicated: "Dedicado", "new": "Novo", suspended: "Suspenso", cdn: "atrás de uma CDN",
      w24h: "24 horas", w30d: "30 dias", w90d: "90 dias", latency: "Latência",
      shortHistory: "histórico curto", noData: "sem dados",
      strip: function (n, low) { return "Últimos 90 dias: " + n + " com dados" + (low ? ", o menor " + low : ""); },
      dayTitle: function (d, p) { return d + ": " + p; },
      ago90: "há 90 dias", today: "hoje",
      noMatch: "Nenhum servidor neste filtro."
    }
  };

  var lang = S[document.documentElement.lang] ? document.documentElement.lang : "en";
  function t() { return S[lang]; }

  // State: the latest snapshot, when it arrived, and the service's clock.
  var snap = null;        // the parsed /v1/servers
  var skew = 0;           // service clock minus this one, in ms
  var failing = false;    // the latest request failed
  var history = {};       // id -> days, from /v1/servers/{id}/history
  var historyAt = 0;
  var filter = "all";
  var cards = {};         // id -> the card's parts, for updating in place
  var cardsLang = "";

  function serverNow() { return Date.now() + skew; }
  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  // Durations, coarsest two units: "12 s", "3 min", "2 h 5 min", "4 d 3 h".
  function dur(ms) {
    var s = Math.max(0, Math.floor(ms / 1000));
    if (s < 60) return s + " s";
    var m = Math.floor(s / 60);
    if (m < 60) return m + " min";
    var h = Math.floor(m / 60);
    if (h < 24) return h + " h" + (m % 60 ? " " + (m % 60) + " min" : "");
    var d = Math.floor(h / 24);
    return d + " d" + (h % 24 ? " " + (h % 24) + " h" : "");
  }
  function when(iso) {
    var f = new Intl.DateTimeFormat(lang, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit", timeZone: "UTC" });
    return f.format(new Date(iso)) + " UTC";
  }
  // The service has already cut each figure to three decimals, so showing
  // at most three never rounds it.
  function pct(p) {
    return new Intl.NumberFormat(lang, { maximumFractionDigits: 3 }).format(p) + "%";
  }
  function level(w) {
    if (!w || !w.observed_minutes || typeof w.percent !== "number") return "none";
    if (w.percent >= 99.9) return "ok";
    if (w.percent >= 99) return "warn";
    return "bad";
  }

  var SEVERITY = { offline: 0, failing: 1, unknown: 2, up: 3 };
  function shown(s) {
    switch (filter) {
      case "dedicated": return s.kind === "node";
      case "new": return !!s["new"];
      case "offline": return s.state === "offline";
    }
    return true;
  }

  // A card is built once, and its figures are then rewritten in place, so a
  // round of new figures never moves the focus or the scroll.
  function buildCard(s) {
    var c = { li: el("li", "server") };
    var head = el("div", "srv-head");
    c.pill = el("span", "pill-state");
    c.pill.appendChild(el("span", "dot"));
    c.pillText = el("span");
    c.pill.appendChild(c.pillText);
    c.pillLink = el("a");
    c.pillLink.appendChild(c.pill);
    head.appendChild(c.pillLink);
    head.appendChild(el("h3", "", s.id));
    c.badges = el("span", "badges");
    head.appendChild(c.badges);
    c.li.appendChild(head);

    c.meta = el("p", "srv-meta");
    c.li.appendChild(c.meta);
    c.since = el("p", "srv-since");
    c.li.appendChild(c.since);

    var figs = el("dl", "figures");
    c.fig = {};
    [["24h", t().w24h], ["30d", t().w30d], ["90d", t().w90d]].forEach(function (w) {
      var d = el("div");
      d.appendChild(el("dt", "", w[1]));
      var dd = el("dd");
      var a = el("a");
      a.href = "#def-uptime";
      dd.appendChild(a);
      d.appendChild(dd);
      figs.appendChild(d);
      c.fig[w[0]] = a;
    });
    var ld = el("div");
    ld.appendChild(el("dt", "", t().latency));
    var ldd = el("dd");
    c.lat = el("a");
    c.lat.href = "#def-latency";
    ldd.appendChild(c.lat);
    ld.appendChild(ldd);
    figs.appendChild(ld);
    c.li.appendChild(figs);

    c.strip = el("div", "strip");
    c.strip.setAttribute("role", "img");
    c.bars = [];
    for (var i = 0; i < DAYS; i++) {
      var b = el("i");
      b.setAttribute("data-level", "none");
      c.strip.appendChild(b);
      c.bars.push(b);
    }
    c.li.appendChild(c.strip);
    var axis = el("div", "strip-axis");
    axis.appendChild(el("span", "", t().ago90));
    axis.appendChild(el("span", "", t().today));
    c.li.appendChild(axis);
    return c;
  }

  function badge(text, def, cls) {
    var a = el("a", "badge" + (cls ? " " + cls : ""), text);
    a.href = def;
    return a;
  }

  function updateCard(c, s) {
    var L = t();
    var state = SEVERITY[s.state] === undefined ? "unknown" : s.state;
    c.li.setAttribute("data-state", state);
    c.pill.setAttribute("data-state", state);
    c.pillText.textContent = L.state[state];
    c.pillLink.href = state === "up" ? "#def-probe" : "#def-" + state;

    while (c.badges.firstChild) c.badges.removeChild(c.badges.firstChild);
    if (s.kind === "node") c.badges.appendChild(badge(L.dedicated, "#def-dedicated"));
    if (s["new"]) c.badges.appendChild(badge(L["new"], "#def-new", "new"));
    if (s.listing === "suspended") c.badges.appendChild(badge(L.suspended, "#def-suspended", "suspended"));

    var host = "";
    try { host = new URL(s.url).host; } catch (e) { host = String(s.url || ""); }
    c.meta.textContent = [s.operator, s.region, host, s.behind_cdn ? L.cdn : ""].filter(Boolean).join(" · ");

    var up = s.uptime || {};
    ["24h", "30d", "90d"].forEach(function (w) {
      var fig = up[w];
      var a = c.fig[w];
      if (w === "90d" && s["new"]) {
        a.textContent = L.shortHistory;
        a.href = "#def-new";
      } else if (fig && typeof fig.percent === "number") {
        a.textContent = pct(fig.percent);
        a.href = "#def-uptime";
      } else {
        a.textContent = L.noData;
        a.href = "#def-nodata";
      }
      a.title = fig ? fig.up_minutes + " / " + fig.observed_minutes + " min" : "";
    });
    c.lat.textContent = typeof s.latency_ms === "number" ? s.latency_ms + " ms" : "—";

    var days = history[s.id];
    var withData = 0, low = null;
    for (var i = 0; i < DAYS; i++) {
      var d = days && days[days.length - DAYS + i];
      var lv = level(d);
      c.bars[i].setAttribute("data-level", lv);
      c.bars[i].title = d ? L.dayTitle(d.day, lv === "none" ? L.noData : pct(d.percent)) : "";
      if (lv !== "none") {
        withData++;
        if (low === null || d.percent < low) low = d.percent;
      }
    }
    c.strip.setAttribute("aria-label", L.strip(withData, low === null ? "" : pct(low)));
    c.s = s;
    tickCard(c);
  }

  // The one line that ages between rounds: since when a server is down.
  function tickCard(c) {
    var s = c.s, L = t(), now = serverNow(), parts = [];
    switch (s.state) {
      case "up":
        c.since.hidden = true;
        return;
      case "offline":
      case "failing":
        if (s.last_ok) {
          parts.push((s.state === "offline" ? L.offlineFor : L.failingFor)(dur(now - Date.parse(s.last_ok))));
          parts.push(L.lastAnswered(when(s.last_ok)));
        } else {
          parts.push(L.state[s.state]);
          parts.push(L.neverAnswered);
        }
        if (s.last_error && L.err[s.last_error]) parts.push(L.err[s.last_error]);
        break;
      default:
        parts.push(s.last_checked ? L.unknownFor(dur(now - Date.parse(s.last_checked))) : L.notYet);
        if (s.last_ok) parts.push(L.lastAnswered(when(s.last_ok)));
    }
    c.since.hidden = false;
    c.since.textContent = parts.join(" · ");
  }

  function render() {
    var L = t();
    var headline = document.getElementById("st-headline");
    var note = document.getElementById("st-monitor");
    var list = document.getElementById("st-servers");
    var servers = snap && Array.isArray(snap.servers) ? snap.servers : [];

    document.getElementById("st-toolbar").hidden = servers.length === 0;
    document.getElementById("st-empty").hidden = !snap || servers.length > 0;

    if (!snap) {
      headline.textContent = failing ? L.down : L.loading;
      note.hidden = !failing;
      note.textContent = L.downNote;
      tick();
      return;
    }
    var up = servers.filter(function (s) { return s.state === "up"; }).length;
    headline.textContent = servers.length ? L.upOf(up, servers.length) : L.none;

    if (cardsLang !== lang) {
      while (list.firstChild) list.removeChild(list.firstChild);
      cards = {};
      cardsLang = lang;
    }
    var order = servers.slice().sort(function (a, b) {
      var d = (SEVERITY[a.state] === undefined ? 2 : SEVERITY[a.state]) - (SEVERITY[b.state] === undefined ? 2 : SEVERITY[b.state]);
      return d || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
    });
    var seen = {};
    var visible = 0;
    order.forEach(function (s, i) {
      seen[s.id] = true;
      var c = cards[s.id] || (cards[s.id] = buildCard(s));
      updateCard(c, s);
      c.li.hidden = !shown(s);
      if (!c.li.hidden) visible++;
      // Moved only when out of place: moving a card takes the focus off it.
      if (list.children[i] !== c.li) list.insertBefore(c.li, list.children[i] || null);
    });
    Object.keys(cards).forEach(function (id) {
      if (!seen[id]) {
        list.removeChild(cards[id].li);
        delete cards[id];
      }
    });
    var nomatch = document.getElementById("st-nomatch");
    nomatch.hidden = servers.length === 0 || visible > 0;
    nomatch.textContent = L.noMatch;
    tick();
  }

  // Every second: the ages on the page.
  function tick() {
    var L = t(), note = document.getElementById("st-monitor"), checked = document.getElementById("st-checked");
    if (!snap) {
      checked.textContent = "";
      return;
    }
    var age = serverNow() - Date.parse(snap.generated_at);
    var interval = (snap.interval_seconds || 30) * 1000;
    checked.textContent = L.checked(dur(age), snap.vantage || "?");
    if (failing) {
      note.hidden = false;
      note.textContent = L.lost(dur(age));
    } else if (age > 3 * interval) {
      note.hidden = false;
      note.textContent = L.stale(dur(age));
    } else {
      note.hidden = true;
    }
    Object.keys(cards).forEach(function (id) { tickCard(cards[id]); });
  }

  function get(path) {
    return fetch(API + path, { cache: "no-store", credentials: "omit", referrerPolicy: "no-referrer" }).then(function (r) {
      if (!r.ok) throw new Error(path + ": " + r.status);
      var date = Date.parse(r.headers.get("Date") || "");
      if (!isNaN(date)) skew = date - Date.now();
      return r.json();
    });
  }

  function loadHistory(servers) {
    historyAt = Date.now();
    return Promise.all(servers.map(function (s) {
      return get("/v1/servers/" + encodeURIComponent(s.id) + "/history?days=" + DAYS).then(function (h) {
        if (h && Array.isArray(h.days)) history[s.id] = h.days;
      }, function () { /* the strip keeps what it had */ });
    }));
  }

  var timer;
  function poll() {
    clearTimeout(timer);
    get("/v1/servers").then(function (data) {
      if (!data || !Array.isArray(data.servers) || typeof data.generated_at !== "string") throw new Error("unexpected answer");
      snap = data;
      failing = false;
      var unseen = data.servers.some(function (s) { return !history[s.id]; });
      if (unseen || Date.now() - historyAt > HISTORY_EVERY) {
        return loadHistory(data.servers);
      }
    }).catch(function () {
      failing = true;
    }).then(function () {
      render();
      if (!document.hidden) {
        timer = setTimeout(poll, ((snap && snap.interval_seconds) || 30) * 1000);
      }
    });
  }

  document.querySelectorAll("[data-filter]").forEach(function (b) {
    b.addEventListener("click", function () {
      filter = b.getAttribute("data-filter");
      document.querySelectorAll("[data-filter]").forEach(function (o) {
        o.setAttribute("aria-pressed", String(o === b));
      });
      render();
    });
  });
  document.addEventListener("chirp:lang", function (e) {
    lang = S[e.detail] ? e.detail : "en";
    render();
  });
  // A hidden tab stops asking, and asks at once when it is seen again.
  document.addEventListener("visibilitychange", function () {
    if (document.hidden) clearTimeout(timer);
    else poll();
  });

  render();
  poll();
  setInterval(tick, 1000);
})();
