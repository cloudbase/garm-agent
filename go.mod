module github.com/cloudbase/garm-agent

go 1.25.0

require (
	github.com/BurntSushi/toml v1.5.0
	github.com/charmbracelet/x/conpty v0.1.1
	github.com/cloudbase/garm v0.2.0-alpha.0.20251012064551-28a1dea9f558
	github.com/cloudbase/garm-provider-common v0.1.8-0.20251001105909-bbcacae60e7c
	github.com/creack/pty v1.1.24
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/gorilla/websocket v1.5.4-0.20240702125206-a62d9d2a8413
	github.com/spf13/cobra v1.10.1
	go.etcd.io/bbolt v1.4.3
	golang.org/x/sys v0.37.0
)

// Temporary until changes merge upstream
replace github.com/cloudbase/garm => github.com/gabriel-samfira/garm v0.1.1-0.20251014003615-7fc87ef3d7ed

require (
	github.com/bradleyfalzon/ghinstallation/v2 v2.16.0 // indirect
	github.com/charmbracelet/x/errors v0.0.0-20240508181413-e8d8b6e2de86 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/google/go-github/v72 v72.0.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/handlers v1.5.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/minio/sio v0.4.2 // indirect
	github.com/nbutton23/zxcvbn-go v0.0.0-20210217022336-fa2cb2858354 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/teris-io/shortid v0.0.0-20220617161101-71ec9f2aa569 // indirect
	golang.org/x/crypto v0.43.0 // indirect
	golang.org/x/net v0.45.0 // indirect
	golang.org/x/oauth2 v0.32.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)
