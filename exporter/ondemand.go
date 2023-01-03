package exporter

import (
	"context"
	"encoding/json"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

const (
	// AWS doesn’t share the relationship between CPU and memory for each instance type, therefore we get this info from GCP.
	// Obviously, it could be some differences between the cpu/memory relationship between the cloud providers but using the GCP
	// relationship could give us a fairly approximate global idea and allow us know the cost of our pods and namespaces.

	// To simplify operations and taking into account an approximate global idea would be accepted the CPU-Memory relationship is
	// calculated as:

	// CPU-cost = 7.2 memory-GB-cost

	// https://engineering.empathy.co/cloud-finops-part-4-kubernetes-cost-report/
	cpuMemRelation = 7.2
)

func (m *Metrics) GetInstances(ctx context.Context) {
	m.getInstances(ctx)
	m.GetOnDemandPricing(ctx)
}

func (m *Metrics) getInstances(ctx context.Context) {
	ec2Svc := ec2.NewFromConfig(m.awsconfig)
	pag := ec2.NewDescribeInstanceTypesPaginator(
		ec2Svc,
		&ec2.DescribeInstanceTypesInput{})
	for pag.HasMorePages() {
		instances, err := pag.NextPage(ctx)
		if err != nil {
			panic(err.Error())
		}
		for _, instance := range instances.InstanceTypes {
			m.Instances[string(instance.InstanceType)] = &Instance{
				Memory: aws.ToInt64(instance.MemoryInfo.SizeInMiB),
				VCpu:   aws.ToInt32(instance.VCpuInfo.DefaultVCpus),
				Kind:   "ec2",
				Type:   string(instance.InstanceType),
			}
		}
	}
}

func (m Metrics) getInstanceMemory(instance string) string {
	return strconv.Itoa(int(m.Instances[instance].Memory))
}

func (m Metrics) getInstanceVCpu(instance string) string {
	return strconv.Itoa(int(m.Instances[instance].VCpu))
}

func (m Metrics) getNormalizedCost(value float64, instance string) (float64, float64) {
	vcpu := m.Instances[instance].VCpu
	memory := m.Instances[instance].Memory / 1024

	memoryCost := value / (cpuMemRelation*float64(vcpu) + float64(memory))
	vcpuCost := cpuMemRelation * memoryCost

	return vcpuCost, memoryCost
}

func (m *Metrics) GetOnDemandPricing(ctx context.Context) {
	config := m.awsconfig
	config.Region = "us-east-1" // this service is only available in us-east-1

	pricingSvc := pricing.NewFromConfig(config)

	pag := pricing.NewGetProductsPaginator(
		pricingSvc,
		&pricing.GetProductsInput{
			ServiceCode: aws.String("AmazonEC2"),
			MaxResults:  aws.Int32(100),
			Filters: []pricingtypes.Filter{
				{
					Field: aws.String("regionCode"),
					Type:  pricingtypes.FilterTypeTermMatch,
					Value: aws.String(os.Getenv("AWS_REGION")),
				},
				{
					Field: aws.String("capacitystatus"),
					Type:  pricingtypes.FilterTypeTermMatch,
					Value: aws.String("Used"),
				},
				{
					Field: aws.String("tenancy"),
					Type:  pricingtypes.FilterTypeTermMatch,
					Value: aws.String("Shared"),
				},
				{
					Field: aws.String("preInstalledSw"),
					Type:  pricingtypes.FilterTypeTermMatch,
					Value: aws.String("NA"),
				},
				{
					Field: aws.String("operatingSystem"),
					Type:  pricingtypes.FilterTypeTermMatch,
					Value: aws.String("Linux"),
				},
			},
		},
	)

	for pag.HasMorePages() {
		pricelist, err := pag.NextPage(ctx)

		if err != nil {
			panic(err.Error())
		}

		for _, price := range pricelist.PriceList {
			var tmp Pricing
			json.Unmarshal([]byte(price), &tmp)

			skuOnDemand := tmp.Product.Sku + "." + TermOnDemand
			skuOnDemandPerHour := skuOnDemand + "." + TermPerHour

			value, _ := strconv.ParseFloat(tmp.Terms.OnDemand[skuOnDemand].PriceDimensions[skuOnDemandPerHour].PricePerUnit["USD"], 64)

			vcpu, memory := m.getNormalizedCost(value, tmp.Product.Attributes["instanceType"])

			m.Instances[tmp.Product.Attributes["instanceType"]].Cost = value
			m.Instances[tmp.Product.Attributes["instanceType"]].VCpuCost = vcpu
			m.Instances[tmp.Product.Attributes["instanceType"]].MemoryCost = memory

		}

	}
}
