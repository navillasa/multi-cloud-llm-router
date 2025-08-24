package main

import (
	"fmt"
	
	"github.com/navillasa/multi-cloud-llm-router/infra/common"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		
		// Configuration
		region := cfg.Get("region")
		if region == "" {
			region = "us-east-1"
		}
		
		environment := cfg.Get("environment")
		if environment == "" {
			environment = "dev"
		}

		domainName := cfg.Require("domainName") // e.g., "llm.yourdomain.com"
		
		naming := common.NewResourceNaming(environment, "multi-cloud-llm", "aws")
		
		// VPC
		vpc, err := ec2.NewVpc(ctx, naming.GetName("vpc"), &ec2.VpcArgs{
			CidrBlock:          pulumi.String("10.0.0.0/16"),
			EnableDnsHostnames: pulumi.Bool(true),
			EnableDnsSupport:   pulumi.Bool(true),
			Tags:               pulumi.ToStringMap(naming.GetTags()),
		})
		if err != nil {
			return err
		}

		// Internet Gateway
		igw, err := ec2.NewInternetGateway(ctx, naming.GetName("igw"), &ec2.InternetGatewayArgs{
			VpcId: vpc.ID(),
			Tags:  pulumi.ToStringMap(naming.GetTags()),
		})
		if err != nil {
			return err
		}

		// Public Subnets
		publicSubnet1, err := ec2.NewSubnet(ctx, naming.GetName("public-subnet-1"), &ec2.SubnetArgs{
			VpcId:                   vpc.ID(),
			CidrBlock:               pulumi.String("10.0.1.0/24"),
			AvailabilityZone:        pulumi.Sprintf("%sa", region),
			MapPublicIpOnLaunch:     pulumi.Bool(true),
			Tags:                    pulumi.ToStringMap(naming.GetTags()),
		})
		if err != nil {
			return err
		}

		publicSubnet2, err := ec2.NewSubnet(ctx, naming.GetName("public-subnet-2"), &ec2.SubnetArgs{
			VpcId:                   vpc.ID(),
			CidrBlock:               pulumi.String("10.0.2.0/24"),
			AvailabilityZone:        pulumi.Sprintf("%sb", region),
			MapPublicIpOnLaunch:     pulumi.Bool(true),
			Tags:                    pulumi.ToStringMap(naming.GetTags()),
		})
		if err != nil {
			return err
		}

		// Route Table
		routeTable, err := ec2.NewRouteTable(ctx, naming.GetName("route-table"), &ec2.RouteTableArgs{
			VpcId: vpc.ID(),
			Routes: ec2.RouteTableRouteArray{
				&ec2.RouteTableRouteArgs{
					CidrBlock: pulumi.String("0.0.0.0/0"),
					GatewayId: igw.ID(),
				},
			},
			Tags: pulumi.ToStringMap(naming.GetTags()),
		})
		if err != nil {
			return err
		}

		// Route Table Associations
		_, err = ec2.NewRouteTableAssociation(ctx, naming.GetName("rta-1"), &ec2.RouteTableAssociationArgs{
			SubnetId:     publicSubnet1.ID(),
			RouteTableId: routeTable.ID(),
		})
		if err != nil {
			return err
		}

		_, err = ec2.NewRouteTableAssociation(ctx, naming.GetName("rta-2"), &ec2.RouteTableAssociationArgs{
			SubnetId:     publicSubnet2.ID(),
			RouteTableId: routeTable.ID(),
		})
		if err != nil {
			return err
		}

		// EKS Cluster Service Role
		eksRole, err := iam.NewRole(ctx, naming.GetName("eks-role"), &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(`{
				"Version": "2012-10-17",
				"Statement": [
					{
						"Action": "sts:AssumeRole",
						"Effect": "Allow",
						"Principal": {
							"Service": "eks.amazonaws.com"
						}
					}
				]
			}`),
			Tags: pulumi.ToStringMap(naming.GetTags()),
		})
		if err != nil {
			return err
		}

		// Attach required policies to EKS role
		_, err = iam.NewRolePolicyAttachment(ctx, naming.GetName("eks-service-policy"), &iam.RolePolicyAttachmentArgs{
			Role:      eksRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"),
		})
		if err != nil {
			return err
		}

		// Node Group Role
		nodeRole, err := iam.NewRole(ctx, naming.GetName("node-role"), &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(`{
				"Version": "2012-10-17",
				"Statement": [
					{
						"Action": "sts:AssumeRole",
						"Effect": "Allow",
						"Principal": {
							"Service": "ec2.amazonaws.com"
						}
					}
				]
			}`),
			Tags: pulumi.ToStringMap(naming.GetTags()),
		})
		if err != nil {
			return err
		}

		// Attach policies to node role
		nodePolicies := []string{
			"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
			"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
			"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
		}

		for i, policy := range nodePolicies {
			_, err = iam.NewRolePolicyAttachment(ctx, naming.GetName(fmt.Sprintf("node-policy-%d", i)), &iam.RolePolicyAttachmentArgs{
				Role:      nodeRole.Name,
				PolicyArn: pulumi.String(policy),
			})
			if err != nil {
				return err
			}
		}

		// EKS Cluster
		cluster, err := eks.NewCluster(ctx, naming.GetName("cluster"), &eks.ClusterArgs{
			RoleArn: eksRole.Arn,
			VpcConfig: &eks.ClusterVpcConfigArgs{
				SubnetIds: pulumi.StringArray{
					publicSubnet1.ID(),
					publicSubnet2.ID(),
				},
				EndpointPrivateAccess: pulumi.Bool(false),
				EndpointPublicAccess:  pulumi.Bool(true),
			},
			Version: pulumi.String("1.28"),
			Tags:    pulumi.ToStringMap(naming.GetTags()),
		})
		if err != nil {
			return err
		}

		// Node Group - FREE TIER OPTIMIZED
		nodeGroup, err := eks.NewNodeGroup(ctx, naming.GetName("nodegroup"), &eks.NodeGroupArgs{
			ClusterName:   cluster.Name,
			NodeRoleArn:   nodeRole.Arn,
			SubnetIds: pulumi.StringArray{
				publicSubnet1.ID(),
				publicSubnet2.ID(),
			},
			InstanceTypes: pulumi.StringArray{
				pulumi.String("t3.small"), // 2 vCPU, 2GB RAM - Minimum for tiny LLMs
			},
			ScalingConfig: &eks.NodeGroupScalingConfigArgs{
				DesiredSize: pulumi.Int(1), // Only 1 node to minimize cost
				MinSize:     pulumi.Int(1),
				MaxSize:     pulumi.Int(2), // Max 2 for cost control
			},
			AmiType:       pulumi.String("AL2_x86_64"),
			CapacityType:  pulumi.String("SPOT"), // Use SPOT instances for maximum savings
			DiskSize:      pulumi.Int(8), // Minimum disk size
			Tags:          pulumi.ToStringMap(naming.GetTags()),
		})
		if err != nil {
			return err
		}

		// Generate kubeconfig
		kubeconfig := common.GenerateEKSKubeConfig(
			cluster.Name,
			cluster.Endpoint,
			cluster.CertificateAuthority.Data().Elem(),
			region,
		)

		// Create Kubernetes provider
		k8sProvider, err := kubernetes.NewProvider(ctx, naming.GetName("k8s-provider"), &kubernetes.ProviderArgs{
			Kubeconfig: kubeconfig,
		}, pulumi.DependsOn([]pulumi.Resource{nodeGroup}))
		if err != nil {
			return err
		}

		// Install Argo CD
		_, err = helm.NewRelease(ctx, naming.GetName("argocd"), &helm.ReleaseArgs{
			Chart:     pulumi.String("argo-cd"),
			Version:   pulumi.String("5.51.4"),
			Namespace: pulumi.String("argocd"),
			RepositoryOpts: &helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://argoproj.github.io/argo-helm"),
			},
			CreateNamespace: pulumi.Bool(true),
			Values: pulumi.Map{
				"server": pulumi.Map{
					"service": pulumi.Map{
						"type": pulumi.String("LoadBalancer"),
					},
					"extraArgs": pulumi.StringArray{
						pulumi.String("--insecure"), // For initial setup
					},
				},
				"configs": pulumi.Map{
					"params": pulumi.Map{
						"server.insecure": pulumi.Bool(true),
					},
				},
			},
		}, pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}

		// Outputs
		ctx.Export("vpcId", vpc.ID())
		ctx.Export("clusterName", cluster.Name)
		ctx.Export("clusterEndpoint", cluster.Endpoint)
		ctx.Export("kubeconfig", kubeconfig)
		ctx.Export("region", pulumi.String(region))
		ctx.Export("clusterHostname", pulumi.String(domainName))

		return nil
	})
}
