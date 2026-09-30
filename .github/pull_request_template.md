## What changes

<!-- What and why. The how is in the diff. -->

## How it was verified

<!-- Commands run, cases tested. "CI passed" is not verification. -->

## Checklist

- [ ] Targets `develop` (only a release or a hotfix targets `main`).
- [ ] Every commit carries a DCO sign-off (`git commit -s`) — CI checks it.
- [ ] `go build ./... && go vet ./... && go test -race ./...` pass locally.
- [ ] No secret, token or credential in the diff, including tests and fixtures.
- [ ] New dependency: permissive licence, and `THIRD_PARTY_NOTICES.md`
      regenerated (`./scripts/gen-third-party-notices.sh THIRD_PARTY_NOTICES.md`).
- [ ] Security impact considered — untrusted input, authentication, sensitive
      data in logs. If there is any, it is described above.
- [ ] The change sits in the right repository — see [ECOSYSTEM.md](https://github.com/barahn/chirp/blob/develop/ECOSYSTEM.md).

<!--
External contributions: pull requests from outside Fabrintek are not merged
until CLA acceptance is enforced here. See CONTRIBUTING.md.
-->
