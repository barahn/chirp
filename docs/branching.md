# Branches, proteção e fluxo de merge

## Modelo

`develop` é a branch padrão (default) do repositório: é dela que sai todo
branch de trabalho e é nela que todo pull request é mesclado.

`main` é a branch de release. Ela só recebe merge vindo de `develop` (ou de
um `hotfix/*`), e cada merge em `main` é um ponto de release, marcado com uma
tag `vX.Y.Z` -- o CI já reconhece essas tags.

```
feature/*  fix/*  deps/*  ──▶  develop  ──▶  main  ──▶  tag vX.Y.Z
                                   ▲                      │
                                   └──── hotfix/* ────────┘
```

Um `hotfix/*` sai de `main`, entra em `main` por pull request e é mesclado de
volta em `develop` no mesmo dia -- senão a correção se perde no próximo
release.

Nomes de branch: `feature/<assunto>`, `fix/<assunto>`, `hotfix/<assunto>`,
`deps/<assunto>`, `docs/<assunto>`.

## O que a proteção garante

Nenhuma das duas branches aceita push direto, force-push ou deleção. As regras
estão versionadas em [`.github/rulesets/`](../.github/rulesets/) e são a
referência: o que estiver configurado no GitHub e não estiver ali é desvio.

| Regra | `main` | `develop` |
| --- | --- | --- |
| Push direto | bloqueado | bloqueado |
| Force-push / deleção | bloqueado | bloqueado |
| Pull request obrigatório | sim | sim |
| Aprovações | 1, de um CODEOWNER | 1 |
| Aprovação descartada a cada push | sim | sim |
| Último push precisa de aprovação de outra pessoa | sim | não |
| Threads de revisão resolvidas | sim | sim |
| Histórico linear | sim | sim |
| Commits assinados (GPG/SSH) | sim | não |
| Checks obrigatórios e atualizados com a base | sim | sim |

Checks obrigatórios: `Build, Test & SAST` (build, vet, lint, testes com
`-race`, DCO, licenças de terceiros, gosec, govulncheck), `Analyze (go)`
(CodeQL) e `Review dependency changes` (vulnerabilidade e licença nas
dependências novas).

`bypass_actors` está vazio de propósito: a regra vale inclusive para quem tem
admin. Se o repositório estiver sendo tocado por uma pessoa só e a exigência
de uma aprovação travar o trabalho em `develop`, a saída correta é adicionar
o admin como bypass actor **apenas na ruleset de `develop`**, de forma
explícita e versionada aqui -- não afrouxar `main`.

## Aplicando as regras

As rulesets são importáveis na interface: **Settings > Rules > Rulesets > New
ruleset > Import a ruleset**, uma vez para cada arquivo de
`.github/rulesets/`.

Pela API (precisa de token com `administration:write`):

```sh
gh api -X POST repos/barahn/chirp/rulesets --input .github/rulesets/main-protected.json
gh api -X POST repos/barahn/chirp/rulesets --input .github/rulesets/develop-protected.json
```

Para trocar a branch padrão:

```sh
gh api -X PATCH repos/barahn/chirp -f default_branch=develop
```

## Fluxo de trabalho

```sh
git switch develop && git pull
git switch -c feature/consent-expiry

# ... alterações ...
git commit -s          # -s é obrigatório: o CI valida o trailer DCO

git push -u origin feature/consent-expiry
# abrir o pull request contra develop
```

Para release: pull request de `develop` para `main`, merge (sem squash, para
o histórico de `main` apontar para os commits reais), e tag `vX.Y.Z` no merge.

## Cultura DevSecOps

O que sustenta isso não é a proteção de branch sozinha:

- **Segurança no pull request, não depois.** SAST (gosec, CodeQL), SCA
  (govulncheck, dependency review) e verificação de licença rodam antes do
  merge e bloqueiam. Nada disso é um relatório para ler depois.
- **Menor privilégio no pipeline.** Todo workflow declara
  `permissions: contents: read` no topo; o que precisa de mais pede no job,
  nominalmente (CodeQL pede `security-events: write` e nada além).
- **Dependência fixada e atualizada.** Toda action é fixada por SHA -- tag é
  mutável, SHA não -- e o Dependabot abre PR semanal contra `develop` para que
  o SHA fixado não envelheça em silêncio.
- **Procedência.** DCO em todo commit, CODEOWNERS em toda mudança, commits
  assinados em `main`.
- **Relato responsável.** [SECURITY.md](../SECURITY.md) define canal privado e
  prazos; vulnerabilidade não vira issue pública.
- **Configuração como código.** Rulesets, workflows e políticas vivem no
  repositório e mudam por pull request, revisadas como código.

### Configuração que não cabe em arquivo

Em **Settings > Code security**, deixe habilitados: Dependabot alerts,
Dependabot security updates, secret scanning e push protection (esta última
rejeita o push que carrega um segredo, em vez de avisar depois que ele já
está no histórico). Essas opções são do repositório e não são versionáveis --
revise-as junto com as rulesets.
