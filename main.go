package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/pkg/browser"
)

var (
	regionsInput          string
	regions               []string
	accountsInput         string
	accounts              []ssotypes.AccountInfo
	outputFormatInput     string
	awsTemplate           *template.Template
	steampipeTemplate     *template.Template
	awsTemplateData       *AWSTemplateData
	steampipeTemplateData *SteampipeTemplateData
	beginLine             int
	endLine               int
)

type AWSTemplateData struct {
	SSOStartURL    string
	SSORegion      string
	SSOAccountID   string
	SSORoleName    string
	AssumeRoleName string
	AccountList    []ssotypes.AccountInfo
}

type SteampipeTemplateData struct {
	Regions           []string
	AccountList       []ssotypes.AccountInfo
	AllAccountsString string
	RegionsString     string
}

func init() {
	awsTemplate = template.Must(template.ParseFiles("aws_credentials.tmpl"))
	steampipeTemplate = template.Must(template.ParseFiles("steampipe_aws.spc.tmpl"))
}

func main() {
	awsTemplateData := AWSTemplateData{}
	steampipeTemplateData := SteampipeTemplateData{}

	flag.StringVar(&awsTemplateData.SSOStartURL, "sso-start-url", "", "AWS SSO Start URL")
	flag.StringVar(&awsTemplateData.SSORegion, "sso-region", "", "AWS SSO Region")
	flag.StringVar(&awsTemplateData.SSOAccountID, "sso-account-id", "", "AWS SSO Account ID")
	flag.StringVar(&awsTemplateData.SSORoleName, "sso-role-name", "AdministratorAccess", "SSO Role To Assume (must be the same across all accounts)")
	flag.StringVar(&awsTemplateData.AssumeRoleName, "assume-role-name", "OrganizationAccountAccessRole", "Name of role to assume in linked account")
	flag.StringVar(&regionsInput, "regions", "", "Comma-separated list of regions to tell Steampipe to connect to (default: uses same search order as aws cli)")
	flag.StringVar(&accountsInput, "accounts", "", "Comma-separated list of accounts to tell Steampipe to connect to (default: all accounts assigned to you through SSO)")
	flag.StringVar(&outputFormatInput, "outputFormat", "json", "Output format for AWS CLI")

	flag.Parse()
	if awsTemplateData.SSOStartURL == "" || awsTemplateData.SSORegion == "" || awsTemplateData.SSOAccountID == "" {
		flag.Usage()
		os.Exit(1)
	}

	if regionsInput != "" {
		regions = strings.Split(regionsInput, ",")
	}

	if accountsInput != "" {
		for _, accountID := range strings.Split(accountsInput, ",") {
			accounts = append(accounts, ssotypes.AccountInfo{AccountId: &accountID})
		}
	}

	// load default aws config
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithDefaultRegion(awsTemplateData.SSORegion))
	if err != nil {
		fmt.Println(err)
	}
	// create sso oidc client to trigger login flow
	ssooidcClient := ssooidc.NewFromConfig(cfg)
	if err != nil {
		fmt.Println(err)
	}
	// register your client which is triggering the login flow
	register, err := ssooidcClient.RegisterClient(context.TODO(), &ssooidc.RegisterClientInput{
		ClientName: aws.String("aip/awsssohelper"),
		ClientType: aws.String("public"),
		Scopes:     []string{"sso-portal:*"},
	})
	if err != nil {
		fmt.Println(err)
	}
	// authorize your device using the client registration response
	deviceAuth, err := ssooidcClient.StartDeviceAuthorization(context.TODO(), &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     register.ClientId,
		ClientSecret: register.ClientSecret,
		StartUrl:     aws.String(awsTemplateData.SSOStartURL),
	})
	if err != nil {
		fmt.Println(err)
	}
	// trigger OIDC login. open browser to login. close tab once login is done. press enter to continue
	url := aws.ToString(deviceAuth.VerificationUriComplete)
	fmt.Printf("If browser is not opened automatically, please open link:\n%v\n", url)
	err = browser.OpenURL(url)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("Press ENTER key once login is done")
	_ = bufio.NewScanner(os.Stdin).Scan()
	// generate sso token
	token, err := ssooidcClient.CreateToken(context.TODO(), &ssooidc.CreateTokenInput{
		ClientId:     register.ClientId,
		ClientSecret: register.ClientSecret,
		DeviceCode:   deviceAuth.DeviceCode,
		GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
	})
	if err != nil {
		fmt.Println(err)
	}
	// create sso client
	ssoClient := sso.NewFromConfig(cfg)
	// list accounts
	fmt.Println("Fetching list of all accounts for user")

	accountPaginator := sso.NewListAccountsPaginator(ssoClient, &sso.ListAccountsInput{
		AccessToken: token.AccessToken,
	})
	for accountPaginator.HasMorePages() {
		x, err := accountPaginator.NextPage(context.TODO())
		if err != nil {
			fmt.Println(err)
		}
		for _, account := range x.AccountList {
			if *account.AccountId != awsTemplateData.SSOAccountID {
				accounts = append(accounts, account)
				fmt.Printf("\nAccount ID: %v Name: %v\n", aws.ToString(account.AccountId), aws.ToString(account.AccountName))
			} else {
				fmt.Println("\nSkipping profile creation for master account")
			}
		}
	}

	steampipeTemplateData.AccountList = accounts
	steampipeTemplateData.RegionsString = strings.Join(regions, "\", \"")
	for _, account := range accounts {
		steampipeTemplateData.AllAccountsString = steampipeTemplateData.AllAccountsString + "\"aws_" + *account.AccountId + "\", "
	}

	steampipeTemplateData.AllAccountsString = strings.Trim(steampipeTemplateData.AllAccountsString, ", ")

	// fmt.Println(steampipeTemplateData.AllAccountsString)

	err = steampipeTemplate.Execute(os.Stdout, steampipeTemplateData)
	if err != nil {
		log.Fatalln(err)
	}

	homeDir, _ := os.UserHomeDir()
	awsCredentialsFilePath := filepath.Join(homeDir, ".aws/credentials")
	input, err := ioutil.ReadFile(awsCredentialsFilePath)
	if err != nil {
		log.Fatalln(err)
	}

	lines := strings.Split(string(input), "\n")

	var awsTemplateBuffer bytes.Buffer

	awsTemplateData.AccountList = accounts
	err = awsTemplate.Execute(&awsTemplateBuffer, awsTemplateData)
	if err != nil {
		log.Fatalln(err)
	}

	newLines := strings.Split(awsTemplateBuffer.String(), "\n")

	beginLine, endLine := -1, -1

	for i, line := range lines {
		if line == "### BEGIN_AWSSSOHELPER ###" {
			fmt.Printf("BEGIN_AWSSSOHELPER found on line %d", i)
			beginLine = i
		}
		if line == "### END_AWSSSOHELPER ###" {
			fmt.Printf("END_AWSSSOHELPER found on line %d", i)
			endLine = i
		}
	}

	var fileContents []string
	if beginLine == -1 || endLine == -1 {
		// Append to file
		fmt.Println("AWSSSOHELPER block not found. Appending new block")
		fileContents = append(lines, newLines...)
	} else {
		// Replace block in file
		fmt.Println("Replacing AWSSSOHELPER block")
		contentBefore := lines[0:beginLine]
		contentAfter := lines[endLine+1 : len(lines)-1]
		fileContents = append(contentBefore, newLines...)
		fileContents = append(fileContents, contentAfter...)
	}

	output := strings.Join(fileContents, "\n")
	err = ioutil.WriteFile(awsCredentialsFilePath, []byte(output), 0755)
	if err != nil {
		log.Fatalln(err)
	}
}
