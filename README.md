# eks-cost-exporter
Expose the cost of each pod relative to the cost of the instance that it is running on as prometheus metrics

# metrics

```
eks_cost_pod_cpu{kind="fargate",namespace="kube-system",pod="coredns-687d8ddc5b-psk7t",type="fargate"} 0.00020879999999999998
eks_cost_pod_memory{kind="fargate",namespace="kube-system",pod="coredns-687d8ddc5b-psk7t",type="fargate"} 0.00013095550537109375
eks_cost_pod_total{kind="fargate",namespace="kube-system",pod="coredns-687d8ddc5b-psk7t",type="fargate"} 0.00033975550537109373
...
eks_cost_pod_cpu{kind="m6i.xlarge",namespace="kube-system",pod="aws-node-4mqg5",type="ec2"} 9.835714285714287e-05
eks_cost_pod_memory{kind="m6i.xlarge",namespace="kube-system",pod="aws-node-4mqg5",type="ec2"} 0.0002456796169281006
eks_cost_pod_total{kind="m6i.xlarge",namespace="kube-system",pod="aws-node-4mqg5",type="ec2"} 0.0003440367597852435
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
