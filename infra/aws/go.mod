module github.com/navillasa/multi-cloud-llm-router/infra/aws

go 1.21

replace github.com/navillasa/multi-cloud-llm-router/infra/common => ../common

require (
	github.com/navillasa/multi-cloud-llm-router/infra/common v0.0.0
	github.com/pulumi/pulumi-aws/sdk/v6 v6.20.0
	github.com/pulumi/pulumi-kubernetes/sdk/v4 v4.6.1
	github.com/pulumi/pulumi/sdk/v3 v3.95.0
)
