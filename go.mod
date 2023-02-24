module github.com/brigadecore/brigade-github-gateway

go 1.18

replace k8s.io/client-go => k8s.io/client-go v0.18.2

require (
	github.com/armon/circbuf v0.0.0-20190214190532-5111143e8da2
	github.com/brigadecore/brigade-foundations v0.2.0
	github.com/brigadecore/brigade/sdk/v3 v3.1.0
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/google/go-github/v33 v33.0.0
	github.com/gorilla/mux v1.8.0
	github.com/kr/text v0.2.0 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9 // indirect
	golang.org/x/oauth2 v0.0.0-20180821212333-d2e6202438be
	google.golang.org/appengine v1.6.7 // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/golang/protobuf v1.3.2 // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.6.7 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/net v0.7.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
