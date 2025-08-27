package main

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// VPCConfig for GPU node group (matches main infrastructure)
type VPCConfig struct {
	VPCId             pulumi.StringOutput
	PrivateSubnetIds  pulumi.StringArrayOutput
	SecurityGroupId   pulumi.StringOutput
}

// GPU Node Group configuration for AWS EKS
func createGPUNodeGroup(ctx *pulumi.Context, cluster *eks.Cluster, vpcConfig VPCConfig) (*eks.NodeGroup, error) {
	// Create GPU-optimized launch template
	gpuLaunchTemplate, err := ec2.NewLaunchTemplate(ctx, "gpu-launch-template", &ec2.LaunchTemplateArgs{
		NamePrefix: pulumi.String("llm-gpu-"),
		ImageId:    pulumi.String("ami-0c02fb55956c7d316"), // Amazon EKS optimized AMI with GPU support
		InstanceType: pulumi.String("g4dn.xlarge"), // Cost-effective GPU instance
		
		UserData: pulumi.String(`#!/bin/bash
# Install NVIDIA drivers and container runtime
/etc/eks/bootstrap.sh llm-cluster --container-runtime containerd

# Install NVIDIA device plugin after cluster is ready
`),
		
		VpcSecurityGroupIds: pulumi.StringArray{vpcConfig.SecurityGroupId},
		
		TagSpecifications: ec2.LaunchTemplateTagSpecificationArray{
			&ec2.LaunchTemplateTagSpecificationArgs{
				ResourceType: pulumi.String("instance"),
				Tags: pulumi.StringMap{
					"Name":        pulumi.String("llm-gpu-node"),
					"NodeGroup":   pulumi.String("gpu"),
					"Environment": pulumi.String("dev"),
					"GPU":         pulumi.String("nvidia-tesla-t4"),
				},
			},
		},
		
		BlockDeviceMappings: ec2.LaunchTemplateBlockDeviceMappingArray{
			&ec2.LaunchTemplateBlockDeviceMappingArgs{
				DeviceName: pulumi.String("/dev/xvda"),
				Ebs: &ec2.LaunchTemplateBlockDeviceMappingEbsArgs{
					VolumeSize: pulumi.Int(50),
					VolumeType: pulumi.String("gp3"),
					DeleteOnTermination: pulumi.String("true"),
					Encrypted:           pulumi.String("true"),
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// Create GPU node group
	gpuNodeGroup, err := eks.NewNodeGroup(ctx, "gpu-node-group", &eks.NodeGroupArgs{
		ClusterName:   cluster.Name,
		NodeGroupName: pulumi.String("gpu-nodes"),
		NodeRoleArn:   pulumi.String("arn:aws:iam::ACCOUNT:role/NodeInstanceRole"), // Replace with actual role
		SubnetIds:     vpcConfig.PrivateSubnetIds,
		
		InstanceTypes: pulumi.StringArray{
			pulumi.String("g4dn.xlarge"),
		},
		
		CapacityType: pulumi.String("SPOT"), // Use spot instances for cost savings
		
		ScalingConfig: &eks.NodeGroupScalingConfigArgs{
			DesiredSize: pulumi.Int(0), // Start with 0, scale on demand
			MaxSize:     pulumi.Int(3), // Maximum for cost control
			MinSize:     pulumi.Int(0), // Allow scale to zero
		},
		
		UpdateConfig: &eks.NodeGroupUpdateConfigArgs{
			MaxUnavailablePercentage: pulumi.Int(25),
		},
		
		LaunchTemplate: &eks.NodeGroupLaunchTemplateArgs{
			Id:      gpuLaunchTemplate.ID(),
			Version: pulumi.String("$Latest"),
		},
		
		Labels: pulumi.StringMap{
			"node-type":     pulumi.String("gpu"),
			"accelerator":   pulumi.String("nvidia-tesla-t4"),
			"compute-type":  pulumi.String("gpu-optimized"),
		},
		
		Taints: eks.NodeGroupTaintArray{
			&eks.NodeGroupTaintArgs{
				Key:    pulumi.String("nvidia.com/gpu"),
				Value:  pulumi.String("true"),
				Effect: pulumi.String("NO_SCHEDULE"),
			},
		},
		
		Tags: pulumi.StringMap{
			"Name":        pulumi.String("llm-gpu-node-group"),
			"Environment": pulumi.String("dev"),
			"GPU":         pulumi.String("enabled"),
			"CostCenter":  pulumi.String("llm-inference"),
		},
	})
	
	return gpuNodeGroup, err
}

// GPU cost estimation and monitoring
type GPUCostConfig struct {
	InstanceType       string
	OnDemandPriceUSD   float64
	SpotPriceUSD       float64
	GPUType           string
	GPUMemoryGB       int
	EstimatedMonthlySpot float64
}

func getAWSGPUCostConfig() GPUCostConfig {
	return GPUCostConfig{
		InstanceType:       "g4dn.xlarge",
		OnDemandPriceUSD:   0.526, // per hour in us-east-1
		SpotPriceUSD:       0.158, // typical spot price (70% discount)
		GPUType:           "NVIDIA Tesla T4",
		GPUMemoryGB:       16,
		EstimatedMonthlySpot: 113.76, // $0.158 * 24 * 30 = ~$114/month
	}
}
