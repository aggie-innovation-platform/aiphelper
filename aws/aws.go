package aws

import (
	"bytes"
	"context"
	"crypto/sha1"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"golang.org/x/exp/slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
	ssooidc "github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/pkg/browser"

	"github.com/tamu-edu/aiphelper/utils"
)

//go:embed aws_config.tmpl
var awsTemplateString string

//go:embed steampipe.gospc
var steampipeTemplateString string

type SSOCachedCredential struct {
	StartUrl    string    `json:"startUrl"`
	Region      string    `json:"region"`
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type AWSAccountInfo struct {
	NormalizedAccountName string
	ssotypes.AccountInfo
}

var (
	awsTemplate       *template.Template
	steampipeTemplate *template.Template

	accounts              []AWSAccountInfo
	steampipeTemplateData = SteampipeTemplateData{Marker: utils.Marker}
	awsTemplateData       = AWSTemplateData{Marker: utils.Marker}
)

type AWSTemplateData struct {
	Params      *Options
	AccountList []AWSAccountInfo
	Marker      string
}

type SteampipeTemplateData struct {
	Regions           []string
	AccountList       []AWSAccountInfo
	AllAccountsString string
	RegionsString     string
	Marker            string
}

func Init() {

	awsTemplate = template.Must(template.New("awsTemplate").Parse(awsTemplateString))
	steampipeTemplate = template.Must(template.New("steampipeTemplate").Parse(steampipeTemplateString))

	//steampipeTemplateData.RegionsString = "\"" + strings.Join(regions, "\", \"") + "\""

	awsTemplateData.Params = options

	// fmt.Println(args.Aws.Accounts)
	// if args.Aws.SSOStartURL == "" || args.Aws.SSORegion == "" {
	// 	flag.Usage()
	// 	os.Exit(1)
	// }

	// if regionsInput != "" {
	// 	regions = strings.Split(regionsInput, ",")
	// }

	// if len(params.Accounts) > 0 {
	// 	for _, accountID := range params.Accounts {
	// 		accounts = append(accounts,
	// 			AWSAccountInfo{
	// 				AccountInfo: ssotypes.AccountInfo{
	// 					AccountId: &accountID,
	// 				},
	// 			},
	// 		)
	// 	}
	// }

	accessToken, cfg, err := authenticate()
	if err != nil {
		log.Fatalln(err)
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
			if !slices.Contains(options.Accounts.All, *account.AccountId) {
				continue
			}
			account.NormalizedAccountName = utils.SnakeCase(*account.AccountName)
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

func authenticate() (string, aws.Config, error) {
	// load default aws config
	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(), awsconfig.WithRegion(options.SSORegion))
	if err != nil {
		fmt.Println(err)
	}

	accessToken, err := searchForSsoCachedCredentials(options.SSOStartURL, options.SSORegion)
	if err != nil {
		// create sso oidc client to trigger login flow
		ssooidcClient := ssooidc.NewFromConfig(cfg)
		if err != nil {
			fmt.Println(err)
		}
		// register your client which is triggering the login flow
		register, err := ssooidcClient.RegisterClient(context.TODO(), &ssooidc.RegisterClientInput{
			ClientName: aws.String("github.com/tamu-edu/aiphelper"),
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
			StartUrl:     aws.String(options.SSOStartURL),
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
			Region:      options.SSORegion,
			StartUrl:    options.SSOStartURL,
			ExpiresAt:   exp.UTC(),
		}

		if err := putSsoCachedCredentials(ssoCacheFile); err != nil {
			log.Printf("Error occurred writing the credentials to cache: %s", err)
		}
	} else {
		fmt.Println("Using existing access token in SSO cache")
	}

	return accessToken, cfg, nil
}

func updateAwsConfigFile() {
	var err error = nil
	homeDir, _ := os.UserHomeDir()
	awsConfigFilePath := filepath.Join(homeDir, ".aws/config")

	var awsTemplateBuffer bytes.Buffer

	awsTemplateData.AccountList = accounts
	err = awsTemplate.Execute(&awsTemplateBuffer, awsTemplateData)
	if err != nil {
		log.Fatalln(err)
	}

	err = utils.CreateOrReplaceInFile(awsConfigFilePath, awsTemplateBuffer.String())

	if err != nil {
		log.Fatalln(err)
	}
}

func updateSteampipeAwsConfigFile() {
	var spcTemplateBuffer bytes.Buffer
	var err error = nil

	steampipeTemplateData.AccountList = accounts

	steampipeTemplateData.RegionsString = strings.Join(options.Regions.All, "\", \"")

	for _, account := range accounts {
		steampipeTemplateData.AllAccountsString = steampipeTemplateData.AllAccountsString + "\"aws_" + *account.AccountId + "\", "
	}

	steampipeTemplateData.AllAccountsString = strings.Trim(steampipeTemplateData.AllAccountsString, ", ")

	err = steampipeTemplate.Execute(&spcTemplateBuffer, steampipeTemplateData)
	if err != nil {
		log.Fatalln(err)
	}

	homeDir, _ := os.UserHomeDir()
	spcFilePath := filepath.Join(homeDir, ".steampipe/config/aws.spc")

	err = utils.CreateOrReplaceInFile(spcFilePath, spcTemplateBuffer.String())
	if err != nil {
		log.Fatalln(err)
	}
}

func searchForSsoCachedCredentials(startUrl string, region string) (string, error) {
	homedir, _ := os.UserHomeDir()
	globPattern := filepath.Join(homedir, ".aws/sso/cache", "*.json")
	matches, err := filepath.Glob(globPattern)
	if err != nil {
		log.Fatalf("Failed to match %q: %v", globPattern, err)
	}

	for _, match := range matches {
		file, _ := ioutil.ReadFile(match)
		data := SSOCachedCredential{}
		if err = json.Unmarshal([]byte(file), &data); err != nil {
			log.Printf("Error: %v", err)
		} else {
			if data.StartUrl != startUrl {
				log.Println("Token does not match desired startUrl")
				continue
			}
			if data.Region != region {
				log.Println("Token does not match desired region")
				continue
			}
			if data.ExpiresAt.Before(time.Now()) {
				log.Println("Token has expired")
				continue
			}
			if len(data.AccessToken) == 0 {
				log.Println("Invalid access token")
				continue
			}
			return data.AccessToken, nil
		}
	}
	return "", errors.New("No access token found")
}

func putSsoCachedCredentials(creds SSOCachedCredential) error {
	s := creds.StartUrl
	h := sha1.New()
	h.Write([]byte(s))
	hash := hex.EncodeToString(h.Sum(nil))

	homedir, _ := os.UserHomeDir()
	cacheFile := filepath.Join(homedir, ".aws/sso/cache", fmt.Sprintf("%s.json", hash))

	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(cacheFile), 0755)
		os.Create(cacheFile)
	}

	f, err := os.OpenFile(cacheFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	cacheContents, _ := json.Marshal(creds)

	f.WriteString(string(cacheContents))

	if err := f.Close(); err != nil {
		return err
	}
	return nil
}
