module github.com/fluxcd/notification-controller

go 1.16

replace github.com/fluxcd/notification-controller/api => ./api

require (
	github.com/Azure/azure-amqp-common-go/v3 v3.1.0
	github.com/Azure/azure-event-hubs-go/v3 v3.3.12
	github.com/Azure/azure-sdk-for-go v56.2.0+incompatible // indirect
	github.com/Azure/go-amqp v0.13.11 // indirect
	github.com/fluxcd/notification-controller/api v0.15.1
	github.com/fluxcd/pkg/apis/meta v0.11.0-rc.1
	github.com/fluxcd/pkg/runtime v0.13.0-rc.2
	github.com/getsentry/sentry-go v0.11.0
	github.com/go-logr/logr v0.4.0
	github.com/google/go-github/v32 v32.1.0
	github.com/hashicorp/go-retryablehttp v0.7.0
	github.com/ktrysmt/go-bitbucket v0.9.24
	github.com/microsoft/azure-devops-go-api/azuredevops v1.0.0-b5
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/sethvargo/go-limiter v0.7.0
	github.com/slok/go-http-metrics v0.9.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/whilp/git-urls v1.0.0
	github.com/xanzy/go-gitlab v0.50.3
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97 // indirect
	golang.org/x/oauth2 v0.0.0-20210810183815-faf39c7919d5
	k8s.io/api v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/client-go v0.21.2
	sigs.k8s.io/controller-runtime v0.9.2
)
