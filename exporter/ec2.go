package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
	log "github.com/sirupsen/logrus"
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
	now := time.Now()
	defer timeTrack(now, "Retrieving EC2 Instance Types")

	m.getInstances(ctx)
	m.GetOnDemandPricing(ctx)
	m.GetSpotPricing(ctx)
}

func (m *Metrics) getInstances(ctx context.Context) {
	ec2Svc := ec2.NewFromConfig(m.awsconfig)
	pag := ec2.NewDescribeInstanceTypesPaginator(
		ec2Svc,
		&ec2.DescribeInstanceTypesInput{})
	instanceCount := 0
	for pag.HasMorePages() {
		instances, err := pag.NextPage(ctx)
		if err != nil {
			panic(err.Error())
		}
		for _, instance := range instances.InstanceTypes {
			m.Instances[string(instance.InstanceType)] = &Instance{
				Memory:       aws.ToInt64(instance.MemoryInfo.SizeInMiB),
				VCpu:         aws.ToInt32(instance.VCpuInfo.DefaultVCpus),
				Type:         string(instance.InstanceType),
				OnDemandCost: &Ec2Cost{},
				SpotCost:     make(map[string]*Ec2Cost, 0),
			}
		}
		instanceCount += len(instances.InstanceTypes)
	}
	log.Infof("Collected %d instance types", instanceCount)
}

func (m Metrics) getInstanceMemory(instance string) string {
	return strconv.Itoa(int(m.Instances[instance].Memory))
}

func (m Metrics) getInstanceVCpu(instance string) string {
	return strconv.Itoa(int(m.Instances[instance].VCpu))
}

func (m Metrics) getNormalizedCost(value float64, instance string) (float64, float64, error) {
	if _, ok := m.Instances[instance]; ok {
		vcpu := m.Instances[instance].VCpu
		memory := m.Instances[instance].Memory / 1024

		memoryCost := value / (cpuMemRelation*float64(vcpu) + float64(memory))
		vcpuCost := cpuMemRelation * memoryCost

		return vcpuCost, memoryCost, nil
	}
	log.Debugf("could not find instance type %s, ignoring", instance)
	return 0, 0, fmt.Errorf("could not find instance type %s", instance)
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

			vcpu, memory, err := m.getNormalizedCost(value, tmp.Product.Attributes["instanceType"])
			if err != nil {
				// somehow we got the price of the instance but it is not returned by instance types api, legacy instance type?
				continue
			}

			m.Instances[tmp.Product.Attributes["instanceType"]].OnDemandCost.Type = "ondemand"
			m.Instances[tmp.Product.Attributes["instanceType"]].OnDemandCost.Total = value
			m.Instances[tmp.Product.Attributes["instanceType"]].OnDemandCost.VCpu = vcpu
			m.Instances[tmp.Product.Attributes["instanceType"]].OnDemandCost.Memory = memory

		}

	}
}

func (m *Metrics) GetSpotPricing(ctx context.Context) {
	config := m.awsconfig

	ec2Svc := ec2.NewFromConfig(config)

	pag := ec2.NewDescribeSpotPriceHistoryPaginator(
		ec2Svc,
		&ec2.DescribeSpotPriceHistoryInput{
			StartTime:           aws.Time(time.Now()),
			ProductDescriptions: []string{"Linux/UNIX"},
		})

	for pag.HasMorePages() {
		history, err := pag.NextPage(ctx)
		if err != nil {
			panic(err.Error())
		}

		for _, price := range history.SpotPriceHistory {
			value, _ := strconv.ParseFloat(*price.SpotPrice, 64)

			vcpu, memory, err := m.getNormalizedCost(value, string(price.InstanceType))
			if err != nil {
				// somehow we got the price of the instance but it is not returned by instance types api, legacy instance type?
				continue
			}

			m.Instances[string(price.InstanceType)].SpotCost[aws.ToString(price.AvailabilityZone)] = &Ec2Cost{Type: "spot", Total: value, VCpu: vcpu, Memory: memory}
		}
	}
}
