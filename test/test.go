package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
)

func main() {
	s := "https://aggie-innovation-platform.awsapps.com/start"
	h := sha1.New()
	h.Write([]byte(s))
	sha1_hash := hex.EncodeToString(h.Sum(nil))

	fmt.Println(s, sha1_hash)
}
