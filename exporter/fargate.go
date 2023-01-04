package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

func (m *Metrics) GetFargatePricing(ctx context.Context) {
	now := time.Now()
	defer timeTrack(now, "Retrieving Fargate pricing")

	config := m.awsconfig
	config.Region = "us-east-1" // this service is only available in us-east-1

	pricingSvc := pricing.NewFromConfig(config)

	m.Instances["fargate"] = &Instance{Type: "fargate", Kind: "fargate"}

	pag := pricing.NewGetProductsPaginator(
		pricingSvc,
		&pricing.GetProductsInput{
			ServiceCode: aws.String("AmazonEKS"),
			MaxResults:  aws.Int32(100),
			Filters: []pricingtypes.Filter{
				{
					Field: aws.String("regionCode"),
					Type:  pricingtypes.FilterTypeTermMatch,
					Value: aws.String(os.Getenv("AWS_REGION")),
				},
				{
					Field: aws.String("tenancy"),
					Type:  pricingtypes.FilterTypeTermMatch,
					Value: aws.String("Shared"),
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

			skuOnDemand := fmt.Sprintf("%s.%s", tmp.Product.Sku, TermOnDemand)
			skuOnDemandPerHour := fmt.Sprintf("%s.%s", skuOnDemand, TermPerHour)

			value, _ := strconv.ParseFloat(tmp.Terms.OnDemand[skuOnDemand].PriceDimensions[skuOnDemandPerHour].PricePerUnit["USD"], 64)

			description := tmp.Terms.OnDemand[skuOnDemand].PriceDimensions[skuOnDemandPerHour].Description
			if strings.Contains(description, "AWS Fargate - vCPU - ") {
				m.Instances["fargate"].VCpuCost = value
			} else if strings.Contains(description, "AWS Fargate - Memory - ") {
				m.Instances["fargate"].MemoryCost = value
			}
		}
	}
}
