#!/usr/bin/env bash
#
# Assembles Chirp's website, served at usechirp.app, into _site/ (or the
# directory given). The pages live in site/; the fonts and the bird icon are
# the ones cmd/chirp embeds, copied in here rather than kept twice, so the
# site and the app cannot drift apart.
#
# The pages load nothing from another origin: their Content-Security-Policy
# allows 'self' only, which keeps any script but the site's own off the link
# page, where the token sits in the fragment. It also requires Trusted Types
# and allows no policy, so even the site's own script cannot parse a string
# as HTML: site.js builds its translations into nodes instead. Cloudflare,
# in front of GitHub Pages, sends the same policy as a header, with
# frame-ancestors added; scripts/check-site.sh keeps the two equal.
#
# Preview: scripts/build-site.sh && python3 -m http.server -d _site 8000
#
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
out="${1:-$root/_site}"
warm="$root/cmd/chirp/internal/warm"

rm -rf "$out"
cp -R "$root/site" "$out"
# The tests for the site's scripts sit next to them, and stay in the repo.
find "$out" -name '*.test.js' -delete
mkdir -p "$out/assets/fonts"
cp "$warm"/fonts/*.ttf "$warm"/fonts/OFL-*.txt "$out/assets/fonts/"
cp "$warm/bird/icon.svg" "$out/favicon.svg"

# Social preview crawlers read <head> without running site.js, so /pt/,
# /pt/l/, /pt/privacy/, /pt/how/, and /pt/community/
# are built from the English pages with the Portuguese Open Graph and Twitter
# Card tags and og-chirp-pt.png rather than kept twice in site/.
mkdir -p "$out/pt/l" "$out/pt/privacy" "$out/pt/how" "$out/pt/community"
sed \
  -e 's|<html lang="en"|<html lang="pt-BR"|' \
  -e 's|<title>Chirp — remote help</title>|<title>Chirp — ajuda remota</title>|' \
  -e 's|content="Remote help between two people who trust each other. Nothing to install, no account, and nothing happens on your screen until you press Allow."|content="Ajuda remota entre duas pessoas que confiam uma na outra. Nada para instalar, nenhuma conta, e nada acontece na sua tela até você apertar Permitir."|g' \
  -e 's|content="Remote help between two people who trust each other. Nothing to install, no account."|content="Ajuda remota entre duas pessoas que confiam uma na outra. Nada para instalar, nenhuma conta."|g' \
  -e 's|content="Chirp — Remote help"|content="Chirp — Ajuda remota"|g' \
  -e 's|content="https://usechirp.app/"|content="https://usechirp.app/pt/"|g' \
  -e 's|https://usechirp.app/assets/og-chirp.png|https://usechirp.app/assets/og-chirp-pt.png|g' \
  -e 's|<meta property="og:locale" content="en_US">|<meta property="og:locale" content="pt_BR">|' \
  -e 's|<meta property="og:locale:alternate" content="pt_BR">|<meta property="og:locale:alternate" content="en_US">|' \
  -e 's|href="/"|href="/pt/"|g' \
  -e 's|href="/#|href="/pt/#|g' \
  -e 's|href="/privacy/"|href="/pt/privacy/"|g' \
  -e 's|href="/how/"|href="/pt/how/"|g' \
  -e 's|href="/community/"|href="/pt/community/"|g' \
  "$out/index.html" > "$out/pt/index.html"

sed \
  -e 's|<html lang="en"|<html lang="pt-BR"|' \
  -e 's|A Chirp link — remote help|Um link do Chirp — ajuda remota|g' \
  -e 's|content="Someone sent you a Chirp link. Paste it into Chirp to help them; they'\''ll be asked on their own screen first."|content="Alguém te mandou um link do Chirp. Cole no Chirp para ajudar essa pessoa; ela vai ser perguntada na própria tela antes."|g' \
  -e 's|content="https://usechirp.app/l/"|content="https://usechirp.app/pt/l/"|g' \
  -e 's|https://usechirp.app/assets/og-chirp.png|https://usechirp.app/assets/og-chirp-pt.png|g' \
  -e 's|<meta property="og:locale" content="en_US">|<meta property="og:locale" content="pt_BR">|' \
  -e 's|<meta property="og:locale:alternate" content="pt_BR">|<meta property="og:locale:alternate" content="en_US">|' \
  -e 's|href="/"|href="/pt/"|g' \
  -e 's|href="/#|href="/pt/#|g' \
  -e 's|href="/privacy/"|href="/pt/privacy/"|g' \
  -e 's|href="/how/"|href="/pt/how/"|g' \
  -e 's|href="/community/"|href="/pt/community/"|g' \
  "$out/l/index.html" > "$out/pt/l/index.html"

sed \
  -e 's|<html lang="en"|<html lang="pt-BR"|' \
  -e 's|Privacy &amp; Terms — Chirp|Privacidade e Termos — Chirp|g' \
  -e 's|content="Chirp is built on radical data minimization: no accounts, no trackers, ephemeral memory-only connection handling, and end-to-end encrypted screen sharing."|content="O Chirp foi construído sob minimização radical de dados: sem contas, sem rastreamento, processamento efêmero em memória e compartilhamento de tela com criptografia ponta a ponta."|g' \
  -e 's|content="Chirp is built on radical data minimization: no accounts, no trackers, ephemeral memory-only connection handling."|content="O Chirp foi construído sob minimização radical de dados: sem contas, sem rastreamento, processamento efêmero em memória."|g' \
  -e 's|content="https://usechirp.app/privacy/"|content="https://usechirp.app/pt/privacy/"|g' \
  -e 's|https://usechirp.app/assets/og-chirp.png|https://usechirp.app/assets/og-chirp-pt.png|g' \
  -e 's|<meta property="og:locale" content="en_US">|<meta property="og:locale" content="pt_BR">|' \
  -e 's|<meta property="og:locale:alternate" content="pt_BR">|<meta property="og:locale:alternate" content="en_US">|' \
  -e 's|href="/"|href="/pt/"|g' \
  -e 's|href="/#|href="/pt/#|g' \
  -e 's|href="/privacy/"|href="/pt/privacy/"|g' \
  -e 's|href="/how/"|href="/pt/how/"|g' \
  -e 's|href="/community/"|href="/pt/community/"|g' \
  "$out/privacy/index.html" > "$out/pt/privacy/index.html"

sed \
  -e 's|<html lang="en"|<html lang="pt-BR"|' \
  -e 's|How Chirp Works — Chirp|Como o Chirp Funciona — Chirp|g' \
  -e 's|content="One binary on both sides, instant connection codes, explicit on-screen consent, and zero telemetry: the definitive product model of Chirp."|content="Um executável único nas duas pontas, códigos de conexão efêmeros, consentimento explícito na tela e zero telemetria: o modelo de produto definitivo do Chirp."|g' \
  -e 's|content="One binary on both sides, instant connection codes, explicit on-screen consent, and zero telemetry."|content="Um executável único nas duas pontas, códigos de conexão efêmeros, consentimento explícito na tela e zero telemetria."|g' \
  -e 's|content="https://usechirp.app/how/"|content="https://usechirp.app/pt/how/"|g' \
  -e 's|https://usechirp.app/assets/og-chirp.png|https://usechirp.app/assets/og-chirp-pt.png|g' \
  -e 's|<meta property="og:locale" content="en_US">|<meta property="og:locale" content="pt_BR">|' \
  -e 's|<meta property="og:locale:alternate" content="pt_BR">|<meta property="og:locale:alternate" content="en_US">|' \
  -e 's|href="/"|href="/pt/"|g' \
  -e 's|href="/#|href="/pt/#|g' \
  -e 's|href="/privacy/"|href="/pt/privacy/"|g' \
  -e 's|href="/how/"|href="/pt/how/"|g' \
  -e 's|href="/community/"|href="/pt/community/"|g' \
  "$out/how/index.html" > "$out/pt/how/index.html"

sed \
  -e 's|<html lang="en"|<html lang="pt-BR"|' \
  -e 's|Community — Chirp|Comunidade — Chirp|g' \
  -e 's|content="A constructive space dedicated to the growth, real-world usage, and architectural evolution of Chirp."|content="Um espaço construtivo dedicado ao crescimento, uso no mundo real e evolução arquitetural do Chirp."|g' \
  -e 's|content="https://usechirp.app/community/"|content="https://usechirp.app/pt/community/"|g' \
  -e 's|https://usechirp.app/assets/og-chirp.png|https://usechirp.app/assets/og-chirp-pt.png|g' \
  -e 's|<meta property="og:locale" content="en_US">|<meta property="og:locale" content="pt_BR">|' \
  -e 's|<meta property="og:locale:alternate" content="pt_BR">|<meta property="og:locale:alternate" content="en_US">|' \
  -e 's|href="/"|href="/pt/"|g' \
  -e 's|href="/#|href="/pt/#|g' \
  -e 's|href="/privacy/"|href="/pt/privacy/"|g' \
  -e 's|href="/how/"|href="/pt/how/"|g' \
  -e 's|href="/community/"|href="/pt/community/"|g' \
  "$out/community/index.html" > "$out/pt/community/index.html"

# Cloudflare tells browsers to keep /assets/ for hours, under the same names
# from one deploy to the next, while the pages expire in minutes. A page from
# a new deploy could then show an old screenshot, or run an old site.js that
# lacks its new translations. Each reference a page makes to an asset, or to
# the favicon, which browsers keep longer still, carries a hash of that file's
# content, so a changed file gets a new URL and an unchanged one stays cached.
for page in "$out"/index.html "$out"/l/index.html "$out"/privacy/index.html "$out"/how/index.html "$out"/community/index.html "$out"/pt/index.html "$out"/pt/l/index.html "$out"/pt/privacy/index.html "$out"/pt/how/index.html "$out"/pt/community/index.html "$out"/status/index.html; do
  for ref in $(grep -oE '"/(assets/[^"?]*|favicon\.svg)"' "$page" | tr -d '"' | sort -u); do
    file="$out$ref"
    [ -f "$file" ] || { echo "$page refers to $ref, which is not in the site" >&2; exit 1; }
    hash="$(sha256sum "$file" | cut -c1-12)"
    sed -i "s#\"$ref\"#\"$ref?v=$hash\"#g" "$page"
  done
done

echo "Site assembled in $out"
