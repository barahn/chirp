## O que muda

<!-- O quê e por quê. O "como" está no diff. -->

## Como foi verificado

<!-- Comandos rodados, casos testados. "CI passou" não é verificação. -->

## Checklist

- [ ] Destino é `develop` (apenas release/hotfix vai para `main`).
- [ ] Commits assinados com DCO (`git commit -s`) -- o CI verifica.
- [ ] `go build ./... && go vet ./... && go test -race ./...` passam localmente.
- [ ] Sem segredo, token ou credencial no diff (inclusive em teste e fixture).
- [ ] Dependência nova: licença compatível e `THIRD_PARTY_NOTICES.md` regerado
      (`./scripts/gen-third-party-notices.sh THIRD_PARTY_NOTICES.md`).
- [ ] Impacto de segurança considerado: entrada não confiável, autenticação,
      dados sensíveis em log. Se houver, descrito acima.

<!--
Contribuição externa: pull requests de fora da Fabrintek não são mesclados
até que o CLA seja verificado automaticamente aqui. Ver CONTRIBUTING.md.
-->
