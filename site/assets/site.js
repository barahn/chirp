// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda
//
// The site's script: the language switch, the download button for this
// computer, the bird's chirp, and on the link page, showing and copying the
// link. English lives in the markup, so every page reads whole without it.
// The status page adds status.js, which hears of the language from the
// "chirp:lang" event this script sends.
(function () {
  "use strict";

  var store = {
    get: function (k) { try { return localStorage.getItem(k); } catch (e) { return null; } },
    set: function (k, v) { try { localStorage.setItem(k, v); } catch (e) {} }
  };

  // Portuguese (Brasil). A key missing here keeps its English.
  var PT = {
    descriptor: "ajuda remota",
    brandHome: "Chirp — ajuda remota, início",
    navLabel: "Seções",
    navHow: "Como funciona",
    navSafety: "Segurança",
    navFree: "Grátis para sempre",
    navDownload: "Baixar",
    langLabel: "Idioma",
    githubLabel: "Código-fonte no GitHub",
    h1a: "Baixe.",
    h1b: "Abra.",
    h1c: "Mande seu <span class=\"chirp\">chirp</span>.",
    lede: "Ajuda remota entre duas pessoas que confiam uma na outra. Nada para instalar, nenhuma conta, e nada acontece na sua tela até <em>você apertar Permitir</em>.",
    otherSystems: "Outros sistemas",
    fine: "Grátis. Um arquivo só, nada para instalar, o mesmo download para todo mundo.",
    pill: "Em desenvolvimento",
    statusText: "<b>O Chirp ainda não foi lançado.</b> Dois Chirps já se encontram, pedem consentimento e compartilham a tela. Testar entre dois computadores é o próximo passo.",
    shotNostr: "Com a sinalização pelo Nostr (CHIRP_NOSTR): os quatro últimos dígitos só os dois Chirps conhecem. Só com um relay, o código são os seis primeiros.",
    howKicker: "Como funciona",
    howTitle: "Três passos, e o último é seu.",
    miniYourCode: "Seu código",
    miniCopyLink: "Copiar link",
    miniNewCode: "Novo código",
    s1Title: "Abra o Chirp",
    s1Text: "Um programa só, igual para todo mundo: sem instalador, sem conta, sem versão de técnico. Ele abre mostrando o seu chirp, um código curto, e um link que você pode enviar no lugar.",
    miniTheirCode: "Código ou link da pessoa",
    miniConnect: "Conectar",
    s2Title: "Mande seu chirp para alguém de confiança",
    s2Text: "A pessoa digita no Chirp dela, ou cola o seu link. Cada uso do link gera um código novo, de uso único, então um código que vaza depois de usado não abre nada.",
    miniAsk: "Alguém quer controlar este computador",
    miniAllow: "Permitir (3)",
    miniDecline: "Recusar",
    s3Title: "Você decide",
    s3Text: "O seu Chirp pergunta a você, na sua própria tela, quem está pedindo e para quê, e você escolhe o que a pessoa pode fazer. Nada começa até você apertar Permitir, e você pode encerrar a sessão a qualquer momento.",
    whose: "Quem digita o código controla. Quem compartilhou decide.",
    sound: "E, do lado de quem ajuda, o Chirp vai fazer o seu único som quando a outra pessoa deixar entrar. Ele nunca vai tocar no computador que está sendo ajudado.",
    safetyKicker: "Segurança",
    safetyTitle: "Um pedido fácil de recusar",
    safetyText: "Ferramentas de acesso remoto são as preferidas dos golpistas por telefone. A janela de consentimento do Chirp foi feita para que um pedido inesperado seja fácil de recusar.",
    p1: "<b>Recusar já vem selecionado.</b><span>Permitir só fica clicável depois de três segundos, então nem um Enter por reflexo nem um clique perdido deixam alguém entrar.</span>",
    p2: "<b>O nome é só o que a pessoa diz.</b><span>“Diz se chamar João da TI.” O Chirp não verifica nomes, avisa isso, e limpa o nome para que ele não se passe por uma linha da janela.</span>",
    p3: "<b>Você escolhe cada permissão.</b><span>Só Ver sua tela começa ligada. Mouse e teclado e arquivos ficam desligados até você ligar, e dá para retirá-los durante a sessão.</span>",
    p4: "<b>Não fazer nada é recusar.</b><span>Fechar a janela, ou deixar o tempo acabar, é dizer não.</span>",
    surprise: "Não esperava um pedido? Aperte Recusar. Ninguém entra sem você.",
    why: "Esta é a única tela do Chirp sem o passarinho. Uma janela simpática é justamente o que o golpista quer, então ela continua séria.",
    neverKicker: "Promessas",
    neverTitle: "O que o Chirp nunca faz",
    neverText: "O que uma ferramenta de acesso remoto poderia fazer, e o Chirp não faz.",
    never: "Nunca",
    n1: "se instala, nem abre junto com o computador",
    n2: "lê sua área de transferência, a não ser quando você cola",
    n3: "deixa alguém entrar sem você apertar Permitir na sua própria tela",
    n5: "cobra por compartilhar a tela, controlar, transferir arquivos, usar a área de transferência ou vários monitores",
    quitNote: "Fechar o Chirp cancela o seu código e o link na hora. Se ele não conseguir falar com o servidor ao fechar, o servidor os descarta sozinho em cerca de dois minutos. Até lá ninguém entra, porque não sobra um Chirp para perguntar a você.",
    freeKicker: "Grátis para sempre",
    freeTitle: "Tudo, de graça, para sempre",
    f1: "Compartilhar a tela",
    f2: "Mouse e teclado",
    f3: "Vários monitores",
    f4: "Arquivos",
    f5: "Área de transferência",
    freeText: "Sem planos, sem período de teste, sem verificação de licença. O Chirp é código aberto sob a AGPL-3.0, então uma cópia hospedada também tem que compartilhar suas mudanças.",
    ownKicker: "Rode o seu",
    ownTitle: "Seu servidor, sem amarras",
    ownText: "Qualquer pessoa pode rodar o próprio relay, sem restrição nenhuma. O servidor hospedado do Chirp é uma conveniência, nunca uma exigência.",
    ownC1: "# numa máquina sua",
    ownC2: "# aponte os dois Chirps para ele",
    ownC3: "# ou sinalize pelo Nostr: o código ganha quatro dígitos que o relay nunca vê",
    ownNote: "Por enquanto, use seu próprio STUN/TURN; o coturn funciona. Quando o Chirp tiver um servidor público, ele será cego: encontra o outro computador pela primeira parte do seu código e nunca vê o resto.",
    dlKicker: "Download",
    dlTitle: "Um arquivo. Abra, e está pronto.",
    dlText: "Seu sistema pode avisar na primeira vez. Veja o que ele diz, e o que apertar.",
    dlWindows: "Baixar para Windows",
    dlMac: "Baixar para macOS",
    dlLinux: "Baixar para Linux",
    saysWindows: "<b>O SmartScreen pode dizer</b> “O Windows protegeu o computador”. Escolha <b>Mais informações</b> e depois <b>Executar assim mesmo</b>.",
    saysMac: "<b>O macOS diz</b> que não pôde verificar o Chirp. Feche o aviso e, em <b>Ajustes do Sistema → Privacidade e Segurança</b>, escolha <b>Abrir Mesmo Assim</b>, de novo a cada nova versão. Depois permita <b>Gravação de Tela</b> e <b>Acessibilidade</b> para o Chirp, uma vez só: elas continuam depois das atualizações.",
    saysLinux: "<b>Torne o arquivo executável</b> e rode. No Wayland, depois que você aperta Permitir, o sistema pergunta mais uma vez <b>qual tela compartilhar</b>.",
    verify: "Toda cópia é idêntica. Confira a sua com o SHA-256 e a assinatura publicados em cada versão.",
    verifyCode: "sha256 · publicado com a primeira versão",
    notYet: "<h3>Ainda não há o que baixar</h3><p>O Chirp está em desenvolvimento. O que já funciona:</p><ul><li>a janela, com o seu código e link, e o formulário para ajudar alguém;</li><li>a janela de consentimento, e a sua resposta chegando ao relay;</li><li>compartilhar a tela, o mouse e o teclado, com uma barra na sua tela enquanto durar, testado até agora em um computador só;</li><li>com a sinalização pelo Nostr, um código cujos quatro últimos dígitos só os dois Chirps conhecem, nunca o servidor;</li><li>o app em inglês e em português do Brasil, seguindo o idioma do sistema;</li><li>um relay que você mesmo pode hospedar.</li></ul><p>Testar entre dois computadores é o próximo passo. <a href=\"https://github.com/barahn/chirp#status\">Acompanhe o andamento</a>.</p>",
    linkTitle: "Alguém te mandou um link do Chirp",
    linkLede: "Cole no Chirp para ajudar essa pessoa. Ela vai ser perguntada na própria tela antes de qualquer coisa começar.",
    copy: "Copiar link",
    haveTitle: "Já tem o Chirp?",
    haveSteps: "<li>Abra o Chirp.</li><li>Cole o link em <b>Ajudar alguém</b>.</li><li>Digite seu nome e aperte <b>Conectar</b>.</li>",
    newTitle: "Ainda não tem?",
    newSteps: "<li>Baixe o Chirp e abra. Nada para instalar.</li><li>Cole o link em <b>Ajudar alguém</b>.</li><li>Digite seu nome e aperte <b>Conectar</b>.</li>",
    notOut: "Ainda não lançado",
    plain: "<b>Colar um link deixa você ajudar quem enviou.</b><p>Nunca entrega o seu computador a ninguém. Isso só acontece quando você compartilha o seu próprio código e aperta Permitir.</p>",
    hashnote: "Esta página nunca recebe o seu link. A parte depois do # fica no seu navegador, que não a envia.",
    by: "Por Fabrintek · AGPL-3.0",
    source: "Código-fonte",
    howChirpWorks: "Como o Chirp funciona",
    wiki: "Wiki",
    discussions: "Discussões",
    conduct: "Código de Conduta",
    security: "Vulnerabilidades:",
    networkStatus: "Status da rede",
    stKicker: "Rede pública",
    stTitle: "Status da rede",
    stLede: "Cada servidor da rede pública do Chirp, sondado de fora a cada 30 segundos. Nenhum número aqui é o que um servidor diz de si mesmo.",
    filterLabel: "Mostrar",
    fAll: "Todos",
    fDedicated: "Dedicados",
    fNew: "Novos",
    fOffline: "Fora do ar",
    legendLabel: "Cada barra é um dia, em UTC:",
    lgOk: "99,9% ou mais",
    lgWarn: "99% ou mais",
    lgBad: "menos",
    lgNone: "sem dados",
    emptyTitle: "Ainda não há servidores",
    emptyText: "A rede pública começa com o primeiro nó. Até lá, o Chirp funciona com um relay que você mesmo roda, ou que alguém de confiança roda.",
    defsTitle: "Como estes números são feitos",
    dProbe: "Sondagem",
    dProbeText: "O monitor pede o /healthz de cada servidor por HTTPS a cada 30 segundos, de uma máquina fora de todos os servidores que observa, abrindo uma conexão nova a cada vez. O servidor não informa nada sobre si, e o health check do relay não revela estado nenhum.",
    dUptime: "Tempo no ar",
    dUptimeText: "A parte dos minutos em que o servidor esteve no ar, entre os minutos em que foi sondado. Um minuto conta como no ar quando a maioria das sondas acertou; um empate conta como fora. Os números são cortados, nunca arredondados, então 100% quer dizer todos os minutos sondados. Os de 30 e 90 dias contam dias inteiros em UTC, incluindo hoje até agora.",
    dNoData: "Sem dados",
    dNoDataText: "Um minuto que ninguém sondou, porque o próprio monitor estava fora, não conta nem como no ar nem como fora. Na faixa de 90 dias, é um dia em branco.",
    dOffline: "Fora do ar",
    dOfflineText: "As três últimas sondas falharam: cerca de 90 segundos sem resposta. O tempo mostrado conta a partir da última resposta.",
    dFailing: "Falhando",
    dFailingText: "A sonda mais recente falhou, mas ainda não três seguidas.",
    dUnknown: "Desconhecido",
    dUnknownText: "O monitor não sonda o servidor há três rodadas, então esta página não diz o que ele está fazendo.",
    dDedicated: "Dedicado",
    dDedicatedText: "Um relay do Chirp em modo público que entrou na rede, rodado pelo operador indicado nele.",
    dNew: "Novo",
    dNewText: "Entrou na rede há menos de 30 dias. O número de 90 dias dele ainda é um histórico curto.",
    dSuspended: "Suspenso",
    dSuspendedText: "Continua listado, mas quebrou uma regra da rede e fica fora da lista que o Chirp usa até ser corrigido.",
    dLatency: "Latência",
    dLatencyText: "Quanto levou a sonda mais recente, quando ela acertou, contando uma negociação TLS nova. Depende de onde o monitor está.",
    defsMore: "As regras, e como um servidor entra na rede, estão <a href=\"https://github.com/barahn/chirp/blob/main/docs/status-page-proposal.md\">na proposta</a>. Os números brutos estão <a href=\"https://status.usechirp.app/v1/servers\">na API</a>.",
    noToken: "<b>Este link está incompleto.</b> Peça para quem enviou copiar de novo do Chirp, inteiro."
  };

  // Words the script itself writes.
  var JS = {
    en: {
      downloadFor: "Download for ",
      downloadChirp: "Download Chirp",
      phone: "Chirp runs on computers: Windows, macOS and Linux. Open this page on the computer that needs help.",
      copied: "Link copied",
      toDark: "Switch to dark theme",
      toLight: "Switch to light theme",
      selected: "Selected. Copy it with Ctrl+C or ⌘C."
    },
    "pt-BR": {
      downloadFor: "Baixar para ",
      downloadChirp: "Baixar o Chirp",
      phone: "O Chirp roda em computadores: Windows, macOS e Linux. Abra esta página no computador que precisa de ajuda.",
      copied: "Link copiado",
      toDark: "Mudar para o tema escuro",
      toLight: "Mudar para o tema claro",
      selected: "Selecionado. Copie com Ctrl+C ou ⌘C."
    }
  };
  // Each page's title and description, by the id of its <main>.
  var META = {
    home: {
      en: [document.title, ""],
      "pt-BR": ["Chirp — ajuda remota", "Ajuda remota entre duas pessoas que confiam uma na outra. Nada para instalar, nenhuma conta, e nada acontece na sua tela até você apertar Permitir."]
    },
    link: {
      en: [document.title, ""],
      "pt-BR": ["Um link do Chirp — ajuda remota", "Alguém te mandou um link do Chirp. Cole no Chirp para ajudar essa pessoa; ela vai ser perguntada na própria tela antes."]
    },
    status: {
      en: [document.title, ""],
      "pt-BR": ["Status da rede — Chirp", "Cada servidor da rede pública do Chirp, sondado de fora a cada 30 segundos: se está no ar, desde quando está fora, e quanto de cada dia respondeu."]
    }
  };

  var LANGS = ["en", "pt-BR"];
  var lang = "en";
  var page = document.querySelector("main").id;
  var metaDesc = document.querySelector('meta[name="description"]');
  META[page].en[1] = metaDesc ? metaDesc.getAttribute("content") : "";

  // A choice made on the page wins; otherwise the first language the
  // browser prefers that the page has. Any Portuguese gets pt-BR.
  function detectLang() {
    var saved = store.get("chirp-site-lang");
    if (LANGS.indexOf(saved) >= 0) return saved;
    var prefs = (navigator.languages && navigator.languages.length) ? navigator.languages : [navigator.language || ""];
    for (var i = 0; i < prefs.length; i++) {
      var l = String(prefs[i]).toLowerCase();
      if (l.indexOf("pt") === 0) return "pt-BR";
      if (l.indexOf("en") === 0) return "en";
    }
    return "en";
  }

  // A translation's markup is built into nodes here, from an allowlist,
  // never handed to innerHTML: each page's Content-Security-Policy turns on
  // Trusted Types with no policy, so the browser refuses every sink that
  // would parse a string as HTML. A translation with markup outside the
  // list, or unbalanced, keeps its English, and says so in the console. Write
  // characters, not entities, except &lt; &gt; and &amp; for < > and &.
  var TAGS = { a: 1, b: 1, em: 1, h3: 1, li: 1, p: 1, span: 1, ul: 1 };
  var TAG_ATTRS = { a: { href: /^https:\/\// }, span: { "class": /^chirp$/ } };
  var ENTITIES = { lt: "<", gt: ">", amp: "&" };
  function text(t) {
    return document.createTextNode(t.replace(/&(lt|gt|amp);/g, function (e, n) { return ENTITIES[n]; }));
  }
  function build(src) {
    var root = document.createDocumentFragment(), open = [root], m, a;
    var token = /<(\/?)([a-z0-9]+)((?:\s+[a-z-]+="[^"<>]*")*)\s*>|[^<]+|</g;
    while ((m = token.exec(src))) {
      var top = open[open.length - 1];
      if (m[2] === undefined) {
        if (m[0] === "<") return null;
        top.appendChild(text(m[0]));
        continue;
      }
      if (!TAGS[m[2]]) return null;
      if (m[1]) {
        if (top === root || top.nodeName.toLowerCase() !== m[2]) return null;
        open.pop();
        continue;
      }
      var el = document.createElement(m[2]);
      var attr = /([a-z-]+)="([^"]*)"/g;
      while ((a = attr.exec(m[3]))) {
        var allowed = TAG_ATTRS[m[2]] && TAG_ATTRS[m[2]][a[1]];
        if (!allowed || !allowed.test(a[2])) return null;
        el.setAttribute(a[1], a[2]);
      }
      top.appendChild(el);
      open.push(el);
    }
    return open.length === 1 ? root : null;
  }
  var built = {};
  function translated(k) {
    if (!(k in built)) {
      built[k] = PT[k] === undefined ? null : build(PT[k]);
      if (PT[k] !== undefined && !built[k]) {
        console.warn("chirp: translation " + k + " has markup outside the allowlist; showing English");
      }
    }
    return built[k];
  }
  function copyOf(el) {
    var f = document.createDocumentFragment();
    for (var n = el.firstChild; n; n = n.nextSibling) f.appendChild(n.cloneNode(true));
    return f;
  }
  // Fills el with its key's text in the current language.
  function fill(el) {
    var nodes = (lang === "pt-BR" && translated(el.getAttribute("data-i18n"))) || el._en;
    while (el.firstChild) el.removeChild(el.firstChild);
    el.appendChild(nodes.cloneNode(true));
  }

  var texts = Array.prototype.slice.call(document.querySelectorAll("[data-i18n]"));
  texts.forEach(function (el) { el._en = copyOf(el); });
  // Only attributes that are plain text are translated.
  var TEXT_ATTRS = { alt: 1, "aria-label": 1, title: 1 };
  var attrs = [];
  document.querySelectorAll("[data-i18n-attr]").forEach(function (el) {
    el.getAttribute("data-i18n-attr").split(";").forEach(function (pair) {
      var p = pair.split(":");
      if (!TEXT_ATTRS[p[0]]) return;
      attrs.push({ el: el, name: p[0], key: p[1], en: el.getAttribute(p[0]) });
    });
  });

  function setLang(l, remember) {
    lang = LANGS.indexOf(l) >= 0 ? l : "en";
    var pt = lang === "pt-BR";
    texts.forEach(fill);
    attrs.forEach(function (a) {
      a.el.setAttribute(a.name, pt && PT[a.key] !== undefined ? PT[a.key] : a.en);
    });
    document.querySelectorAll("[data-lang-only]").forEach(function (el) {
      el.hidden = el.getAttribute("data-lang-only") !== lang;
    });
    document.documentElement.lang = lang;
    document.title = META[page][lang][0];
    if (metaDesc) metaDesc.setAttribute("content", META[page][lang][1]);
    document.querySelectorAll("[data-lang]").forEach(function (b) {
      b.setAttribute("aria-pressed", String(b.getAttribute("data-lang") === lang));
    });
    applyOS();
    labelTheme();
    if (remember) store.set("chirp-site-lang", lang);
    document.dispatchEvent(new CustomEvent("chirp:lang", { detail: lang }));
  }

  // The theme: the visitor's choice, saved, or the system's. theme.js applies
  // a saved choice before the page is drawn; this draws the toggle. Choosing
  // the system's own theme forgets the choice, so the page follows the system
  // again.
  var darkQuery = window.matchMedia ? window.matchMedia("(prefers-color-scheme: dark)") : null;
  function systemTheme() { return darkQuery && darkQuery.matches ? "dark" : "light"; }
  function currentTheme() { return document.documentElement.getAttribute("data-theme") || systemTheme(); }
  function labelTheme() {
    var label = currentTheme() === "dark" ? JS[lang].toLight : JS[lang].toDark;
    document.querySelectorAll("[data-theme-toggle]").forEach(function (b) {
      b.setAttribute("aria-label", label);
      b.setAttribute("title", label);
    });
  }
  document.querySelectorAll("[data-theme-toggle]").forEach(function (b) {
    b.addEventListener("click", function () {
      var next = currentTheme() === "dark" ? "light" : "dark";
      if (next === systemTheme()) {
        document.documentElement.removeAttribute("data-theme");
        try { localStorage.removeItem("chirp-site-theme"); } catch (e) {}
      } else {
        document.documentElement.setAttribute("data-theme", next);
        store.set("chirp-site-theme", next);
      }
      labelTheme();
    });
  });
  if (darkQuery && darkQuery.addEventListener) darkQuery.addEventListener("change", labelTheme);
  document.querySelectorAll("[data-lang]").forEach(function (b) {
    b.addEventListener("click", function () { setLang(b.getAttribute("data-lang"), true); });
  });

  // Which computer is this? Phones get told Chirp is for computers.
  function detectOS() {
    var ua = (navigator.userAgent || "").toLowerCase();
    var p = ((navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || "").toLowerCase();
    if (/android|iphone|ipad|ipod/.test(ua)) return "phone";
    if (p.indexOf("win") === 0 || /windows/.test(ua)) return "windows";
    if (p.indexOf("mac") === 0 || /mac os/.test(ua)) return "macos";
    if (/linux|x11|cros/.test(p + ua)) return "linux";
    return "";
  }
  var names = { windows: "Windows", macos: "macOS", linux: "Linux" };
  var os = detectOS();
  function applyOS() {
    document.querySelectorAll("[data-os-label]").forEach(function (el) {
      el.textContent = names[os] ? JS[lang].downloadFor + names[os] : JS[lang].downloadChirp;
    });
    if (os === "phone") {
      document.querySelectorAll("[data-os-note]").forEach(function (el) { el.textContent = JS[lang].phone; });
    }
  }
  var here = document.querySelector('.os[data-os="' + os + '"]');
  if (here) here.classList.add("here");

  var toast = document.getElementById("toast"), toastTimer;
  function say(msg) {
    if (!toast) {
      toast = document.createElement("div");
      toast.className = "toast";
      toast.setAttribute("role", "status");
      document.body.appendChild(toast);
    }
    toast.textContent = msg;
    toast.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { toast.hidden = true; }, 2600);
  }

  // The link page shows the link it was opened with, so it can be copied
  // and pasted into Chirp. The token is in the fragment, which the browser
  // never sends: this page, and the server behind it, never receive it. A
  // token has the form Chirp checks (client.ParseTarget): 43 characters of
  // unpadded base64url, then -- from a Chirp that signals over Nostr -- a dot
  // and the link's four secret digits, which are part of the link and must
  // be copied with it.
  var linkBox = document.getElementById("linkbox");
  if (linkBox) {
    var showLink = function () {
      var token = location.hash.slice(1);
      var ok = /^[A-Za-z0-9_-]{43}(\.[0-9]{4})?$/.test(token);
      document.getElementById("the-link").textContent = ok ? location.origin + location.pathname + "#" + token : "";
      linkBox.hidden = !ok;
      document.getElementById("no-token").hidden = ok;
    };
    showLink();
    // A link opened in a tab already on this page changes only the fragment.
    window.addEventListener("hashchange", showLink);
    var copyBtn = document.getElementById("copy-link");
    copyBtn.addEventListener("click", function () {
      var link = document.getElementById("the-link");
      var label = copyBtn.querySelector("[data-i18n]");
      function done() {
        label.textContent = JS[lang].copied;
        setTimeout(function () { fill(label); }, 2000);
      }
      function fallback() {
        var r = document.createRange(); r.selectNodeContents(link);
        var s = window.getSelection(); s.removeAllRanges(); s.addRange(r);
        say(JS[lang].selected);
      }
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(link.textContent).then(done, fallback);
      } else { fallback(); }
    });
  }

  setLang(detectLang(), false);

  // The bird chirps now and then, as it does in the app.
  var still = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  var bird = document.getElementById("hero-bird");
  if (bird && !still) {
    setInterval(function () {
      bird.classList.add("is-chirping");
      setTimeout(function () { bird.classList.remove("is-chirping"); }, 700);
    }, 5200);
  }
})();
