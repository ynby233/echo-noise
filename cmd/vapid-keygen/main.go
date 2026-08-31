package main

import (
	"fmt"
	"log"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Fatal("generate VAPID keys: ", err)
	}
	fmt.Println("WEB_PUSH_VAPID_PUBLIC_KEY=" + publicKey)
	fmt.Println("WEB_PUSH_VAPID_PRIVATE_KEY=" + privateKey)
}
