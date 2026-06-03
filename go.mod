module github.com/drover-org/drover-ontology

go 1.26.0

require gopkg.in/yaml.v3 v3.0.1

require (
	github.com/kr/pretty v0.3.0 // indirect
	github.com/rogpeppe/go-internal v1.8.0 // indirect
	github.com/traefik/yaegi v0.16.1 // for rlm integration spike (Yaegi interp + custom drover symbol injection under -tags rlm)
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

require (
	github.com/XiaoConstantine/rlm-go v0.1.0 // for rlm integration spike (use -tags rlm to build the real harness; pulls Yaegi etc.)
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/google/uuid v1.6.0
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-sqlite3 v1.14.44
	github.com/ollama/ollama v0.24.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	pgregory.net/rapid v1.3.0
)
