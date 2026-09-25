# Política de Segurança

## Versões suportadas

Chirp ainda não tem release estável. Correções de segurança são aplicadas em
`develop` e promovidas para `main`; não há backport para tags anteriores.

## Como relatar uma vulnerabilidade

**Não abra issue pública para vulnerabilidade.** Uma issue é indexada antes de
existir correção.

Use um destes canais:

1. **GitHub Security Advisories** (preferido) --
   [Security > Report a vulnerability](https://github.com/barahn/chirp/security/advisories/new).
   O relato fica privado e vira o canal de conversa e o CVE, se for o caso.
2. E-mail para o responsável pelo repositório, se o item 1 não estiver
   disponível para você.

Inclua: versão/commit afetado, impacto, e um caso de reprodução mínimo.

### O que esperar

| Etapa | Prazo alvo |
| --- | --- |
| Confirmação de recebimento | 5 dias úteis |
| Avaliação inicial e severidade | 10 dias úteis |
| Correção ou plano de mitigação | conforme severidade, acordado no advisory |

Divulgação é coordenada: publicamos o advisory junto com a correção. Pedimos
que você não divulgue antes disso.

## Escopo

Vale: código deste repositório, seus workflows de CI e a cadeia de
dependências declarada em `go.mod`.

Não vale: serviços de terceiros, resultado de scanner sem impacto demonstrado,
e engenharia social.

## Controles automatizados

Todo pull request passa por `go vet`, golangci-lint, testes com `-race`,
gosec (SAST), govulncheck, CodeQL e revisão de dependências (vulnerabilidade e
licença). Esses checks são obrigatórios para merge -- ver
[docs/branching.md](docs/branching.md).
