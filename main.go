package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/pkg/browser"
)

type AWSAccountInfo struct {
	NormalizedAccountName string
	ssotypes.AccountInfo
}

var (
	regionsInput      string
	regions           []string
	accountsInput     string
	accounts          []AWSAccountInfo
	outputFormatInput string
	awsTemplate       *template.Template
	steampipeTemplate *template.Template
	// awsTemplateData       *AWSTemplateData
	// steampipeTemplateData *SteampipeTemplateData
	beginLine int
	endLine   int
)

type AWSTemplateData struct {
	SSOStartURL    string
	SSORegion      string
	SSOAccountId   string
	SSORoleName    string
	AssumeRoleName string
	AccountList    []AWSAccountInfo
	DefaultRegion  string
	DefaultFormat  string
}

type SteampipeTemplateData struct {
	Regions           []string
	AccountList       []AWSAccountInfo
	AllAccountsString string
	RegionsString     string
}

var steampipeTemplateData = SteampipeTemplateData{}
var awsTemplateData = AWSTemplateData{}
var templateSectionMarker = "AIPSSOHELPER"

func init() {
	awsTemplate = template.Must(template.ParseFiles("aws_config.tmpl"))
	steampipeTemplate = template.Must(template.ParseFiles("steampipe_aws.spc.tmpl"))
}

func main() {
	flag.StringVar(&awsTemplateData.SSOStartURL, "sso-start-url", "", "AWS SSO Start URL")
	flag.StringVar(&awsTemplateData.SSORegion, "sso-region", "", "AWS SSO Region")
	flag.StringVar(&awsTemplateData.SSORoleName, "sso-role-name", "AdministratorAccess", "SSO Role To Assume (must be the same across all accounts)")
	flag.StringVar(&regionsInput, "regions", "", "Comma-separated list of regions to tell Steampipe to connect to (default: uses same search order as aws cli)")
	flag.StringVar(&accountsInput, "accounts", "", "Comma-separated list of accounts to tell Steampipe to connect to (default: all accounts assigned to you through SSO)")
	flag.StringVar(&awsTemplateData.DefaultFormat, "default-aws-format", "json", "Output format for AWS CLI")
	flag.StringVar(&awsTemplateData.DefaultRegion, "default-aws-region", "us-east-1", "Default region for AWS CLI operations")

	flag.Parse()
	if awsTemplateData.SSOStartURL == "" || awsTemplateData.SSORegion == "" {
		flag.Usage()
		os.Exit(1)
	}

	if regionsInput != "" {
		regions = strings.Split(regionsInput, ",")
	}

	if accountsInput != "" {
		for _, accountID := range strings.Split(accountsInput, ",") {
			accounts = append(accounts,
				AWSAccountInfo{
					AccountInfo: ssotypes.AccountInfo{
						AccountId: &accountID,
					},
				},
			)
		}
	}

	// load default aws config
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithDefaultRegion(awsTemplateData.SSORegion))
	if err != nil {
		fmt.Println(err)
	}

	accessToken, err := searchForSsoCachedCredentials(awsTemplateData.SSOStartURL, awsTemplateData.SSORegion)
	if err != nil {
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

		// trigger OIDC login. open browser to login. begin polling for token. close tab once login is done.
		url := aws.ToString(deviceAuth.VerificationUriComplete)
		fmt.Printf("If browser is not opened automatically, please open link:\n%v\n", url)
		err = browser.OpenURL(url)
		if err != nil {
			fmt.Println(err)
		}

		// Wait for sso token
		var token *ssooidc.CreateTokenOutput

		var slowDownDelay = 5 * time.Second
		var retryInterval = 5 * time.Second //default value

		if i := deviceAuth.Interval; i > 0 {
			retryInterval = time.Duration(i) * time.Second // acceptable value from AWS
		}

		for {
			tokenInput := ssooidc.CreateTokenInput{
				ClientId:     register.ClientId,
				ClientSecret: register.ClientSecret,
				DeviceCode:   deviceAuth.DeviceCode,
				GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
			}
			newToken, err := ssooidcClient.CreateToken(context.TODO(), &tokenInput)

			if err != nil {
				// fmt.Printf("Got oidc error: %v\n", err)
				var sde *ssooidctypes.SlowDownException
				if errors.As(err, &sde) {
					retryInterval += slowDownDelay
				}

				var ape *ssooidctypes.AuthorizationPendingException
				if errors.As(err, &ape) {
					// fmt.Printf("Waiting %d seconds before trying again\n", retryInterval)
					time.Sleep(retryInterval)
					continue
				}
				log.Fatal(err)
			} else {
				token = newToken
				accessToken = *token.AccessToken
				break
			}
		}

		var now = time.Now()
		var exp = now.Add(time.Second * time.Duration(token.ExpiresIn))
		ssoCacheFile := SSOCachedCredential{
			AccessToken: accessToken,
			Region:      awsTemplateData.SSORegion,
			StartUrl:    awsTemplateData.SSOStartURL,
			ExpiresAt:   exp.UTC(),
		}
		fmt.Printf("Time now: %s, time with %d seconds added: %s", now.String(), time.Duration(token.ExpiresIn), exp.UTC().Format(time.RFC3339))

		if err := putSsoCachedCredentials(ssoCacheFile); err != nil {
			log.Printf("Error occurred writing the credentials to cache: %s", err)
		}
	} else {
		fmt.Println("Using existing access token in SSO cache")
	}

	// create sso client
	ssoClient := sso.NewFromConfig(cfg)
	// list accounts
	fmt.Print("Fetching list of all accounts... ")

	accountPaginator := sso.NewListAccountsPaginator(ssoClient, &sso.ListAccountsInput{
		AccessToken: &accessToken,
	})

	for accountPaginator.HasMorePages() {
		x, err := accountPaginator.NextPage(context.TODO())
		if err != nil {
			fmt.Println(err)
		}
		for _, account := range x.AccountList {
			account := AWSAccountInfo{AccountInfo: account}
			account.NormalizedAccountName = snakeCase(*account.AccountName)
			accounts = append(accounts, account)
		}
	}

	fmt.Printf("User has access to %d AWS accounts.\n", len(accounts))

	fmt.Println("Updating AWS config file with profiles.")
	updateAwsConfigFile()

	fmt.Println("Updating Steampipe AWS Plugin config file with connections.")
	updateSteampipeAwsConfigFile()

	fmt.Println("Done.")
}

func snakeCase(str string) string {
	str = strings.ToLower(str)
	var match1 = regexp.MustCompile(`[^a-z0-9]`)
	var match2 = regexp.MustCompile(`(_)*`)
	str = match1.ReplaceAllString(str, "_")
	str = match2.ReplaceAllString(str, "${1}")
	str = strings.Trim(str, "_")
	return str
}

func updateAwsConfigFile() {
	homeDir, _ := os.UserHomeDir()
	awsConfigFilePath := filepath.Join(homeDir, ".aws/config")

	if _, err := os.Stat(awsConfigFilePath); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(awsConfigFilePath), 0755)
		os.Create(awsConfigFilePath)
	}

	input, err := ioutil.ReadFile(awsConfigFilePath)
	if err != nil {
		log.Fatalln(err)
	}

	var awsTemplateBuffer bytes.Buffer

	awsTemplateData.AccountList = accounts
	err = awsTemplate.Execute(&awsTemplateBuffer, awsTemplateData)
	if err != nil {
		log.Fatalln(err)
	}

	output, _ := replaceFileSectionTemplate(string(input), awsTemplateBuffer.String())

	err = ioutil.WriteFile(awsConfigFilePath, []byte(output), 0755)
	if err != nil {
		log.Fatalln(err)
	}
}

func updateSteampipeAwsConfigFile() {
	var spcTemplateBuffer bytes.Buffer

	steampipeTemplateData.AccountList = accounts
	steampipeTemplateData.RegionsString = strings.Join(regions, "\", \"")
	for _, account := range accounts {
		steampipeTemplateData.AllAccountsString = steampipeTemplateData.AllAccountsString + "\"aws_" + *account.AccountId + "\", "
	}

	steampipeTemplateData.AllAccountsString = strings.Trim(steampipeTemplateData.AllAccountsString, ", ")

	err := steampipeTemplate.Execute(&spcTemplateBuffer, steampipeTemplateData)
	if err != nil {
		log.Fatalln(err)
	}

	homeDir, _ := os.UserHomeDir()
	spcFilePath := filepath.Join(homeDir, ".steampipe/config/aws.spc")

	if _, err := os.Stat(spcFilePath); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(spcFilePath), 0755)
		os.Create(spcFilePath)
	}

	input, err := ioutil.ReadFile(spcFilePath)
	if err != nil {
		log.Fatalln(err)
	}

	output, _ := replaceFileSectionTemplate(string(input), spcTemplateBuffer.String())

	err = ioutil.WriteFile(spcFilePath, []byte(output), 0755)
	if err != nil {
		log.Fatalln(err)
	}
}

func replaceFileSectionTemplate(fileContents string, newSection string) (string, error) {
	lines := strings.Split(fileContents, "\n")
	newLines := strings.Split(newSection, "\n")

	beginLine, endLine := -1, -1

	for i, line := range lines {
		if line == fmt.Sprintf("### BEGIN_%s ###", templateSectionMarker) {
			beginLine = i
		}
		if line == fmt.Sprintf("### END_%s ###", templateSectionMarker) {
			endLine = i
		}
	}

	var newFileContents []string
	if beginLine == -1 || endLine == -1 {
		// Append to file
		// fmt.Println("block not found. Appending new block")
		newFileContents = append(lines, newLines...)
	} else {
		// Replace block in file
		// fmt.Println("Replacing block")
		contentBefore := lines[0:beginLine]
		contentAfter := []string{}
		if len(lines) > endLine+1 {
			contentAfter = lines[endLine+1 : len(lines)-1]
		}

		newFileContents = append(contentBefore, newLines...)
		newFileContents = append(newFileContents, contentAfter...)
	}

	return strings.Join(newFileContents, "\n"), nil
}
