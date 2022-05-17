package azure

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/managementgroups/armmanagementgroups"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/subscription/armsubscription"
	"github.com/tamu-edu/aiphelper/utils"
)

//go:embed steampipe.gospc
var steampipeTemplateString string

var (
	steampipeTemplate *template.Template
	options           *Options
	cred              *azidentity.DefaultAzureCredential

	subscriptions         []Subscription
	steampipeTemplateData = SteampipeTemplateData{Marker: utils.Marker}
)

type Subscription struct {
	Name           string
	ID             string
	NormalizedName string
}

type SteampipeTemplateData struct {
	AggregationString string
	Subscriptions     []Subscription
	TenantID          string
	Marker            string
}

func Init() {
	steampipeTemplate = template.Must(template.New("steampipeTemplate").Parse(steampipeTemplateString))

	err := authenticate()
	if err != nil {
		log.Fatalf("failed to authenticate: %v", err)
	}

	fmt.Println("Updating Steampipe Azure Plugin config file with connections.")
	updateSteampipeAzureConfigFile()

	fmt.Println("Done.")
}

func authenticate() error {
	var err error

	cred, err = azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Fatalln("Failed to locate Azure credentials.\nBefore running this utility, please use `az login` or environment variables to make credentials available in the current environment.")
		return err
	}
	return nil
}

func enumSubscriptionsForCurrentUser() ([]Subscription, error) {
	subscriptions := []Subscription{}
	var err error

	ctx := context.Background()
	client, err := armsubscription.NewSubscriptionsClient(cred, nil)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
		return nil, err
	}
	pager := client.NewListPager(nil)
	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			log.Fatalf("failed to advance page: %v", err)
			return nil, err
		}
		for _, v := range nextResult.Value {
			var subscription = Subscription{
				Name:           *v.DisplayName,
				ID:             *v.SubscriptionID,
				NormalizedName: utils.SnakeCase(*v.DisplayName),
			}
			subscriptions = append(subscriptions, subscription)
		}
	}
	return subscriptions, nil
}

func enumSubscriptionsByMgmtGroup() ([]Subscription, error) {

	subscriptions := []Subscription{}
	var err error

	ctx := context.Background()
	client, err := armmanagementgroups.NewClient(cred, nil)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
		return nil, err
	}
	pager := client.NewGetDescendantsPager(options.RootManagementGroup, nil)
	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			log.Fatalf("failed to advance page: %v", err)
			return nil, err
		}
		for _, v := range nextResult.Value {
			// TODO: use page item
			if *v.Type != "Microsoft.Management/managementGroups" {
				// log.Printf("Skipping %s", *v.Properties.DisplayName)
				var subscription = Subscription{
					Name:           *v.Properties.DisplayName,
					ID:             *v.Name,
					NormalizedName: utils.SnakeCase(*v.Properties.DisplayName),
				}
				subscriptions = append(subscriptions, subscription)
				// log.Printf("%s (%s: %s). Parent: %s", *v.Properties.DisplayName, *v.Name, *v.Type, *v.Properties.Parent.ID)
			}
		}
	}
	// fmt.Printf("%#v\n", subscriptions)
	return subscriptions, nil
}

func updateSteampipeAzureConfigFile() {
	var spcTemplateBuffer bytes.Buffer
	var err error = nil

	steampipeTemplateData.TenantID = options.TenantID

	if options.EnumManagementGroup == true {
		steampipeTemplateData.Subscriptions, err = enumSubscriptionsByMgmtGroup()
	} else {
		steampipeTemplateData.Subscriptions, err = enumSubscriptionsForCurrentUser()
	}

	fmt.Printf("User has access to %d Azure subscriptions.\n", len(steampipeTemplateData.Subscriptions))

	if err != nil {
		log.Fatalf("failed to enumerate subscriptions: %v", err)
	}

	for _, subscription := range steampipeTemplateData.Subscriptions {
		steampipeTemplateData.AggregationString = steampipeTemplateData.AggregationString + "\"azure_" + subscription.NormalizedName + "\", "
	}

	steampipeTemplateData.AggregationString = strings.Trim(steampipeTemplateData.AggregationString, ", ")

	err = steampipeTemplate.Execute(&spcTemplateBuffer, steampipeTemplateData)
	if err != nil {
		log.Fatalln(err)
	}

	homeDir, _ := os.UserHomeDir()
	spcFilePath := filepath.Join(homeDir, ".steampipe/config/azure.spc")

	err = utils.CreateOrReplaceInFile(spcFilePath, spcTemplateBuffer.String())
	if err != nil {
		log.Fatalln(err)
	}
}
