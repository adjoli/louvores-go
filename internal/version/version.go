// Package version centraliza a identificação de versão do binário.
//
// Os valores são injetados no momento do build pelo linker, via -ldflags -X
// (ver Makefile). Sem flags, os defaults (Version="dev") são usados — o que
// mantém `go run` e `go build` puros funcionando sem configuração.
package version

import "fmt"

// Valores injetáveis via -ldflags "-X .../internal/version.Version=v1.2.3".
var (
	// Version é a versão semântica do binário (ex.: "1.2.3"). Default "dev".
	Version = "dev"
	// Commit é o hash curto do commit (git rev-parse --short HEAD).
	Commit = ""
	// Date é o timestamp do build (UTC, ISO 8601).
	Date = ""
)

// String devolve a versão formatada para exibição. Quando há um commit
// associado, inclui hash e data; caso contrário, apenas a versão.
func String() string {
	if Commit == "" {
		return Version
	}
	if Date != "" {
		return fmt.Sprintf("%s (%s, %s)", Version, Commit, Date)
	}
	return fmt.Sprintf("%s (%s)", Version, Commit)
}
