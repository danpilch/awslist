package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/olekukonko/tablewriter"
)

// SingleResource defines how we want to describe each AWS resource
type SingleResource struct {
	Region  *string
	Service *string
	Product *string
	Details *string
	ID      *string
	ARN     *string
}

func PrettyPrintResources(resources []*SingleResource) {
	var data [][]string

	for _, r := range resources {
		row := []string{
			DerefNilPointerStrings(r.Region),
			DerefNilPointerStrings(r.Service),
			DerefNilPointerStrings(r.Product),
			DerefNilPointerStrings(r.ID),
		}
		data = append(data, row)
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Region", "Service", "Product", "ID"})
	table.SetBorder(true)
	table.AppendBulk(data)
	table.Render()
}

// GetServiceFromArn removes the arn:aws: component string of
// the name and returns the first keyword that appears, svc
func ServiceNameFromARN(arn *string) *string {
	shortArn := strings.Replace(*arn, "arn:aws:", "", -1)
	sliced := strings.Split(shortArn, ":")
	return &sliced[0]
}

// Short ARN removes the unnecessary info from the ARN we already
// know at this point like region, account id and the service name.
func ShortArn(arn *string) string {
	slicedArn := strings.Split(*arn, ":")
	shortArn := slicedArn[5:]
	return strings.Join(shortArn, "/")
}

// awsEC2 type is created for ARNs belonging to the EC2 service
type awsEC2 string

// awsECS type is created for ARNs belonging to the ECS service
type awsECS string

// awsGeneric is a is a generic AWS for services ARNs that don't have
// a dedicated type within our application.
type awsGeneric string

// Generic Resource Handler
func (aws *awsGeneric) ConverToResource(shortArn, svc, rgn *string) *SingleResource {
	return &SingleResource{ARN: shortArn, Region: rgn, Service: svc, ID: shortArn}
}

// ConvertToRow converts EC2 shortened ARNs to to a SingleResource type
func (aws *awsEC2) ConvertToResource(shortArn, svc, rgn *string) *SingleResource {
	s := strings.Split(*shortArn, "/")
	return &SingleResource{ARN: shortArn, Region: rgn, Service: svc, Product: &s[0], ID: &s[1]}
}

// ConvertToRow converts ECS shortened ARNs to to a SingleResource type
func (aws *awsECS) ConvertToResource(shortArn, svc, rgn *string) *SingleResource {
	s := strings.Split(*shortArn, "/")
	return &SingleResource{ARN: shortArn, Region: rgn, Service: svc, Product: &s[0], ID: &s[1]}
}

// GetResourceRow shortens the ARN and assigns it to the right
// service type calling its "ConvertToRow" method. Since we have
// a default behaviour funneled towards our awsGeneric type, all
// services will be handled.
func ConvertArnToSingleResource(arn, svc, rgn *string) *SingleResource {
	shortArn := ShortArn(arn)

	switch *svc {
	case "ec2":
		res := awsEC2(*svc)
		return res.ConvertToResource(&shortArn, svc, rgn)
	case "ecs":
		res := awsECS(*svc)
		return res.ConvertToResource(&shortArn, svc, rgn)
	default:
		res := awsGeneric(*svc)
		return res.ConverToResource(&shortArn, svc, rgn)
	}
}

// DerefNilPointerStrings utility func to make sure we don't run into
// a "nil pointer dereference" issue during runtime.
func DerefNilPointerStrings(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func main() {
	var resources []*SingleResource

	var region = os.Args[1]

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))

	// Creating the actual AWS client from the SDK
	r := resourcegroupstaggingapi.NewFromConfig(cfg)

	// The results will come paginated, so we create an empty
	// one outside the next for loop so we can keep updating
	// it and check if there are still more results to come or
	// not. We could isolate this function and call it recursively
	// if we wanted to tidy up our code.
	var paginationToken string = ""
	var in *resourcegroupstaggingapi.GetResourcesInput
	var out *resourcegroupstaggingapi.GetResourcesOutput

	// Let's start an infinite for loop until there are no
	for {
		if len(paginationToken) == 0 {
			in = &resourcegroupstaggingapi.GetResourcesInput{
				ResourcesPerPage: aws.Int32(50),
			}
			out, err = r.GetResources(context.Background(), in)
			if err != nil {
				fmt.Println(err)
			}
		} else {
			in = &resourcegroupstaggingapi.GetResourcesInput{
				ResourcesPerPage: aws.Int32(50),
				PaginationToken:  &paginationToken,
			}
		}

		out, err = r.GetResources(context.Background(), in)
		if err != nil {
			fmt.Println(err)
		}

		for _, resource := range out.ResourceTagMappingList {
			svc := ServiceNameFromARN(resource.ResourceARN)
			rgn := region

			resources = append(resources, ConvertArnToSingleResource(resource.ResourceARN, svc, &rgn))
		}

		paginationToken = *out.PaginationToken
		if *out.PaginationToken == "" {
			break
		}
	}

	// Finally print the results
	PrettyPrintResources(resources)
}
