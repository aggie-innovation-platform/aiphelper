package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"
)

type SSOCachedCredential struct {
	StartUrl    string    `json:"startUrl"`
	Region      string    `json:"region"`
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
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
