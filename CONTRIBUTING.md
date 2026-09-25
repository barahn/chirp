# Contributing to Chirp

## Contributions are closed right now

Pull requests from outside Fabrintek will not be merged until the Contributor
License Agreement below is enforced automatically in this repository.

Issues, bug reports, reproduction cases and design discussion are welcome and
wanted. It is only code that has to wait.

**Why.** Chirp is dual-licensed: AGPL-3.0-only to the public, and separately to
Fabrintek for use in its proprietary platform. That arrangement works because
Fabrintek holds the copyright in all of Chirp (see [LICENSING.md](LICENSING.md)).
A contribution merged without a CLA is licensed to Fabrintek under the AGPL and
nothing else — its author keeps their copyright, and Fabrintek can no longer
relicense Chirp as a whole.

Nothing would break. No build fails, no test fails, and the licence identifier
on this repository stays `AGPL-3.0-only`. The problem would surface years later
in a due-diligence review, by which point the contribution is deep in the
history and the contributor may be unreachable.

Accepting contributions under a CLA that lives in a different repository and is
shown to nobody would be worse than accepting none. So: none, until the process
is real.

## Contributor License Agreement

These are the terms that will apply. They are identical to the ones used across
all three Fabrintek repositories, so that a contributor signs once.

1. **Definitions**
   - *"You"* (or *"Contributor"*) means the copyright owner or legal entity
     authorized by the copyright owner submitting the Contribution.
   - *"Contribution"* means any source code, documentation, art assets, or
     other materials submitted via Git commit, pull request, or issue tracker
     to the project.
2. **Grant of Copyright License**: You hereby grant to the project maintainer
   (**Fabrintek Engenharia Digital Ltda**) a perpetual, worldwide,
   non-exclusive, no-charge, royalty-free, irrevocable copyright license to
   reproduce, prepare derivative works of, publicly display, publicly perform,
   sublicense, and distribute your Contributions and such derivative works.
3. **Grant of Patent License**: You hereby grant a perpetual, worldwide,
   non-exclusive, no-charge, royalty-free, irrevocable patent license to make,
   have made, use, offer to sell, sell, import, and otherwise transfer the
   Work, where such license applies only to those patent claims licensable by
   You that are necessarily infringed by Your Contribution(s).
4. **Originality & Authority**: You represent that each of Your Contributions
   is your original creation, and that you have the full legal right to grant
   the licenses set forth above. If your employer has rights to intellectual
   property that includes your Contributions, you represent that you have
   received permission to make Contributions on behalf of that employer.
5. **Open Source Commitment**: Contributions to Chirp are published under the
   **GNU Affero General Public License v3.0**. Fabrintek commits to keeping
   Chirp available under that licence or a compatible open-source successor.
   The sublicensing right in clause 2 exists so that Fabrintek can continue to
   use Chirp in its proprietary platform; it is not a route to closing Chirp.
6. **Dependency Direction**: Dependencies point inwards only. remotekit depends
   on neither of the others; Chirp may depend on remotekit; the Barahn platform
   may depend on both. A change that needs the arrow reversed is a design
   problem, not a licensing exception.

### Sign-off is required, and is not the CLA

Every commit must carry a `Signed-off-by` trailer:

```bash
git commit -s -m "feat(session): expire a PIN after three failed redemptions"
```

That trailer is the [Developer Certificate of
Origin](https://developercertificate.org/). It certifies where the code came
from and that you have the right to submit it. It does **not** grant the
copyright and patent licences in clauses 2 and 3 above — those need separate,
explicit acceptance, which is the part this repository does not yet have.

Treating a DCO sign-off as CLA acceptance is a common shortcut and a bad one:
the text a contributor certifies says nothing about relicensing. When CLA
enforcement lands here it will be a distinct, recorded act, not an inference
drawn from a sign-off.

## What is enforced today

CI checks that every non-merge commit in a pull request carries a
`Signed-off-by` trailer, and that the dependency licences in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) are current and permissive.

Fabrintek's ownership of Chirp is no defence against third-party code arriving
here under terms Fabrintek cannot pass on. The licence gate treats an
unrecognised or copyleft dependency as a build failure rather than a warning,
for that reason.

## Reporting security vulnerabilities

Do **not** open a public issue for a security-sensitive bug. Email
[security@mendsec.com](mailto:security@mendsec.com).

## Onde a mudança entra

`develop` é a branch padrão e a base de todo pull request; `main` é a branch
de release e só recebe merge vindo de `develop` ou de um `hotfix/*`. Ambas são
protegidas: sem push direto, sem force-push, com revisão e checks obrigatórios.
O fluxo completo, as regras de cada branch e as práticas de segurança do
pipeline estão em [docs/branching.md](docs/branching.md).

Relato de vulnerabilidade não vai para issue pública -- ver
[SECURITY.md](SECURITY.md).
