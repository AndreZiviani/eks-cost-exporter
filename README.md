# eks-cost-exporter
Expose the cost of each pod relative to the cost of the instance that it is running on as prometheus metrics

# metrics

```
eks_cost_pod_cpu{kind="fargate",namespace="kube-system",pod="coredns-687d8ddc5b-psk7t",type="fargate"} 0.00020879999999999998
eks_cost_pod_memory{kind="fargate",namespace="kube-system",pod="coredns-687d8ddc5b-psk7t",type="fargate"} 0.000132550048828125
eks_cost_pod_total{kind="fargate",namespace="kube-system",pod="coredns-687d8ddc5b-psk7t",type="fargate"} 0.00034135004882812495
...
eks_cost_pod_cpu{kind="ec2",namespace="kube-system",pod="aws-node-4mqg5",type="m6i.xlarge"} 0.0001475357142857143
eks_cost_pod_memory{kind="ec2",namespace="kube-system",pod="aws-node-4mqg5",type="m6i.xlarge"} 0.0002456796169281006
eks_cost_pod_total{kind="ec2",namespace="kube-system",pod="aws-node-4mqg5",type="m6i.xlarge"} 0.00039321533121381493
```

# permissions

The following IAM permissions are required:
```
"ec2:DescribeAvailabilityZones",
"ec2:DescribeSpotPriceHistory",
"ec2:DescribeInstanceTypes",
"pricing:DescribeServices",
"pricing:GetProducts"
```
